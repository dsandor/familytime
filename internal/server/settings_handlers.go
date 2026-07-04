package server

import (
	"fmt"
	"net/http"

	"familytime/internal/store"
	"familytime/internal/unifi"
)

func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	g := s.store.Snapshot().Gateway
	enrollURL := ""
	if ip := localIP(); ip != "" {
		enrollURL = fmt.Sprintf("http://%s:%d/enroll", ip, s.advertisedPort)
	}
	writeJSON(w, 200, map[string]string{
		"host":      g.Host,
		"siteName":  g.SiteName,
		"dataPath":  s.store.Path(),
		"enrollUrl": enrollURL,
	})
}

func (s *Server) handleSettingsPIN(w http.ResponseWriter, r *http.Request) {
	var in struct{ CurrentPin, NewPin string }
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "Bad request.")
		return
	}
	if !checkPIN(s.store.Snapshot().Auth.PINHash, in.CurrentPin) {
		fail(w, 401, "Current PIN is wrong.")
		return
	}
	h, err := hashPIN(in.NewPin)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	// Rotating the session secret invalidates every existing session cookie
	// — including this caller's — so a fresh one is issued below to keep the
	// parent who just changed the PIN logged in.
	err = s.store.Update(func(d *store.Data) error {
		d.Auth.PINHash = h
		d.Auth.SessionSecret = newSecret()
		return nil
	})
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	s.guard.reset()
	s.issueSession(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// validateAndStoreGateway checks connectivity with the given credentials and
// persists them. Shared by gateway update and trust-cert.
func (s *Server) validateAndStoreGateway(w http.ResponseWriter, r *http.Request, host, apiKey string) {
	fp, err := unifi.FetchCertFingerprint(r.Context(), host)
	if err != nil {
		failErr(w, err)
		return
	}
	api := s.newAPI(host, apiKey, fp)
	if _, err := api.Version(r.Context()); err != nil {
		failErr(w, err)
		return
	}
	site, err := api.FirstSite(r.Context())
	if err != nil {
		failErr(w, err)
		return
	}
	err = s.store.Update(func(d *store.Data) error {
		d.Gateway = store.Gateway{Host: host, APIKey: apiKey, SiteID: site.ID, SiteName: site.InternalReference, CertFingerprint: fp}
		return nil
	})
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleSettingsGateway(w http.ResponseWriter, r *http.Request) {
	var in struct{ Host, APIKey string }
	if err := readJSON(r, &in); err != nil || in.Host == "" || in.APIKey == "" {
		fail(w, 400, "Gateway address and API key are required.")
		return
	}
	s.validateAndStoreGateway(w, r, in.Host, in.APIKey)
}

// handleTrustCert re-pins the gateway certificate after a legitimate change
// (e.g. a firmware update regenerated it).
func (s *Server) handleTrustCert(w http.ResponseWriter, r *http.Request) {
	g := s.store.Snapshot().Gateway
	s.validateAndStoreGateway(w, r, g.Host, g.APIKey)
}
