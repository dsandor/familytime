// Package server is Family Time's HTTP layer: a JSON API consumed by the
// embedded web UI, PIN-based sessions, and the gateway janitor.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"time"

	"familytime/internal/store"
	"familytime/internal/unifi"
)

// UnifiAPI is the slice of unifi.Client the server consumes; tests inject
// an in-memory fake.
type UnifiAPI interface {
	Version(ctx context.Context) (string, error)
	FirstSite(ctx context.Context) (unifi.Site, error)
	ListClients(ctx context.Context, siteID string) ([]unifi.NetClient, error)
	ListTrafficRules(ctx context.Context) ([]unifi.TrafficRule, error)
	CreateTrafficRule(ctx context.Context, r unifi.TrafficRule) (unifi.TrafficRule, error)
	UpdateTrafficRule(ctx context.Context, r unifi.TrafficRule) error
	DeleteTrafficRule(ctx context.Context, id string) error
	// RenameClient pushes a device's Family Time name to UniFi as its client
	// alias, so both UIs agree. Callers treat failures as best-effort — see
	// handleEnroll / handleProfileUpdate.
	RenameClient(ctx context.Context, mac, name string) error
}

type APIFactory func(host, apiKey, certFingerprint string) UnifiAPI

type Server struct {
	store  *store.Store
	newAPI APIFactory
	mux    *http.ServeMux
	guard  *loginGuard
	now    func() time.Time
	// detectGateway guesses the local gateway address; overridable in tests.
	// The env-var API key fallback only applies when the requested host
	// matches this, so a stray/foreign host can never silently borrow the
	// server's key.
	detectGateway func() string
	// suspects holds ids the janitor saw as inconsistent on its previous
	// pass; action is only taken on the second consecutive sighting, so an
	// in-flight rule write can never be mistaken for an orphan.
	suspects map[string]bool
	// advertisedPort is the port shown in the enroll URL on the Settings
	// screen. The LAN IP is auto-detected (see localIP), but the listening
	// port isn't observable from inside net/http, so main wires it in via
	// SetAdvertisedPort. Defaults to 8080, matching main's --port default.
	advertisedPort int
}

func New(st *store.Store, newAPI APIFactory, static fs.FS) *Server {
	s := &Server{store: st, newAPI: newAPI, guard: &loginGuard{}, now: time.Now, detectGateway: guessGateway, suspects: map[string]bool{}, advertisedPort: 8080}
	s.mux = http.NewServeMux()
	s.routes(static)
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

// SetAdvertisedPort sets the port used to build the enroll URL shown in
// Settings (see handleSettingsGet). Callers that don't care about a custom
// port (e.g. most tests) never need to call this.
func (s *Server) SetAdvertisedPort(port int) {
	s.advertisedPort = port
}

// api builds a gateway client from the stored connection settings.
func (s *Server) api() UnifiAPI {
	g := s.store.Snapshot().Gateway
	return s.newAPI(g.Host, g.APIKey, g.CertFingerprint)
}

func (s *Server) routes(static fs.FS) {
	s.mux.Handle("/", http.FileServerFS(static))
	// The enroll page is visited directly by a device's own browser (a QR
	// code or typed address), not through the SPA's client-side router, so
	// it needs its own server-side route to the same embedded index.html.
	s.mux.HandleFunc("GET /enroll", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, static, "index.html")
	})
	s.mux.HandleFunc("GET /api/state", s.handleState)
	s.mux.HandleFunc("POST /api/setup", s.handleSetup)
	s.mux.HandleFunc("POST /api/test-connection", s.handleTestConnection)
	s.mux.HandleFunc("POST /api/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/logout", s.handleLogout)
	// Enrollment is intentionally unauthenticated: the server resolves the
	// device's identity itself from the request's source IP against the
	// gateway's live client list, so a visitor can only ever affect the
	// device making the request — there's no client-supplied identity to
	// abuse. See handleEnrollWhoami / handleEnroll.
	s.mux.HandleFunc("GET /api/enroll/whoami", s.handleEnrollWhoami)
	s.mux.HandleFunc("POST /api/enroll", s.handleEnroll)
	// Protected routes are registered by registerAPIRoutes in handlers.go.
	s.registerAPIRoutes()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

type errBody struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, errBody{Error: msg})
}

// failErr maps gateway errors to friendly, UI-actionable responses.
func failErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, unifi.ErrUnauthorized):
		writeJSON(w, http.StatusBadGateway, errBody{Error: "The gateway rejected the API key.", Code: "unauthorized"})
	case errors.Is(err, unifi.ErrCertChanged):
		writeJSON(w, http.StatusBadGateway, errBody{Error: "Your gateway's security certificate changed. If you recently updated UniFi, re-trust it in Settings.", Code: "cert_changed"})
	case errors.Is(err, unifi.ErrNotFound):
		writeJSON(w, http.StatusBadGateway, errBody{Error: "The gateway didn't recognize that rule (it may have been removed in the UniFi app).", Code: "not_found"})
	default:
		log.Printf("familytime: gateway error: %v", err)
		writeJSON(w, http.StatusBadGateway, errBody{Error: "Can't reach your UniFi gateway right now.", Code: "unreachable"})
	}
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	return dec.Decode(v)
}
