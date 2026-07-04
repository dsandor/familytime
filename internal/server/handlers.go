package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"familytime/internal/rules"
	"familytime/internal/store"
	"familytime/internal/unifi"
)

// registerAPIRoutes wires the protected JSON API (auth-gated).
func (s *Server) registerAPIRoutes() {
	s.mux.Handle("GET /api/presets", s.auth(s.handlePresets))
	s.mux.Handle("GET /api/devices", s.auth(s.handleDevices))
	s.mux.Handle("GET /api/profiles", s.auth(s.handleProfilesList))
	s.mux.Handle("POST /api/profiles", s.auth(s.handleProfileCreate))
	s.mux.Handle("PUT /api/profiles/{id}", s.auth(s.handleProfileUpdate))
	s.mux.Handle("DELETE /api/profiles/{id}", s.auth(s.handleProfileDelete))
	s.mux.Handle("GET /api/rules", s.auth(s.handleRulesList))
	s.mux.Handle("POST /api/rules", s.auth(s.handleRuleCreate))
	s.mux.Handle("PUT /api/rules/{id}", s.auth(s.handleRuleUpdate))
	s.mux.Handle("DELETE /api/rules/{id}", s.auth(s.handleRuleDelete))
	s.mux.Handle("POST /api/pause", s.auth(s.handlePause))
	s.mux.Handle("DELETE /api/pause/{profileId}", s.auth(s.handleUnpause))
	s.mux.Handle("POST /api/pause/{profileId}/delay", s.auth(s.handlePauseDelay))
	s.mux.Handle("GET /api/status", s.auth(s.handleStatus))
	s.mux.Handle("GET /api/settings", s.auth(s.handleSettingsGet))
	s.mux.Handle("PUT /api/settings/pin", s.auth(s.handleSettingsPIN))
	s.mux.Handle("PUT /api/settings/gateway", s.auth(s.handleSettingsGateway))
	s.mux.Handle("POST /api/settings/trust-cert", s.auth(s.handleTrustCert))
}

func (s *Server) handlePresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"presets":    rules.Presets(),
		"categories": rules.Categories(),
	})
}

// profileForID resolves a profile id, including the virtual Everyone
// profile (which is never stored).
func profileForID(d store.Data, id string) (store.Profile, bool) {
	if id == store.EveryoneProfileID {
		return store.Profile{ID: store.EveryoneProfileID, Name: "Everyone", Emoji: "🌍"}, true
	}
	for _, p := range d.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return store.Profile{}, false
}

type deviceInfo struct {
	MAC       string `json:"mac"`
	Name      string `json:"name"`
	IP        string `json:"ip,omitempty"`
	Connected bool   `json:"connected"`
	Wireless  bool   `json:"wireless"`
	ProfileID string `json:"profileId,omitempty"`
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	d := s.store.Snapshot()
	live, err := s.api().ListClients(r.Context(), d.Gateway.SiteID)
	if err != nil {
		failErr(w, err)
		return
	}
	assigned := map[string]string{}     // mac → profile id
	assignedName := map[string]string{} // mac → snapshot name
	for _, p := range d.Profiles {
		for _, dev := range p.Devices {
			assigned[dev.MAC] = p.ID
			assignedName[dev.MAC] = dev.Name
		}
	}
	seen := map[string]bool{}
	var out []deviceInfo
	for _, c := range live {
		mac := strings.ToLower(c.MACAddress)
		seen[mac] = true
		name := c.Name
		if name == "" {
			name = mac
		}
		out = append(out, deviceInfo{
			MAC: mac, Name: name, IP: c.IPAddress,
			Connected: true, Wireless: c.Type == "WIRELESS",
			ProfileID: assigned[mac],
		})
	}
	for mac, pid := range assigned {
		if !seen[mac] {
			out = append(out, deviceInfo{MAC: mac, Name: assignedName[mac], Connected: false, ProfileID: pid})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].MAC < out[j].MAC
	})
	writeJSON(w, 200, out)
}

func (s *Server) handleProfilesList(w http.ResponseWriter, r *http.Request) {
	d := s.store.Snapshot()
	if d.Profiles == nil {
		d.Profiles = []store.Profile{}
	}
	writeJSON(w, 200, d.Profiles)
}

type profileInput struct {
	Name    string         `json:"name"`
	Emoji   string         `json:"emoji"`
	Color   string         `json:"color"`
	Devices []store.Device `json:"devices"`
}

// validateProfileInput normalizes MACs and rejects devices already assigned
// to a different profile (selfID exempts the profile being edited).
func validateProfileInput(d store.Data, in *profileInput, selfID string) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return fmt.Errorf("give this profile a name")
	}
	seen := map[string]bool{}
	for i := range in.Devices {
		in.Devices[i].MAC = strings.ToLower(strings.TrimSpace(in.Devices[i].MAC))
		mac := in.Devices[i].MAC
		if mac == "" {
			return fmt.Errorf("device with empty MAC")
		}
		if seen[mac] {
			return fmt.Errorf("device %s listed twice", mac)
		}
		seen[mac] = true
		for _, p := range d.Profiles {
			if p.ID == selfID {
				continue
			}
			for _, dev := range p.Devices {
				if dev.MAC == mac {
					return fmt.Errorf("that device already belongs to %s", p.Name)
				}
			}
		}
	}
	if in.Devices == nil {
		in.Devices = []store.Device{}
	}
	return nil
}

func (s *Server) handleProfileCreate(w http.ResponseWriter, r *http.Request) {
	var in profileInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "Bad request.")
		return
	}
	p := store.Profile{ID: store.NewID()}
	err := s.store.Update(func(d *store.Data) error {
		if err := validateProfileInput(*d, &in, p.ID); err != nil {
			return err
		}
		p.Name, p.Emoji, p.Color, p.Devices = in.Name, in.Emoji, in.Color, in.Devices
		d.Profiles = append(d.Profiles, p)
		return nil
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, p)
}

// persistProfilesAndRuleIDs writes new name/emoji/color/devices for one or
// more profiles (keyed by id) together with any rule ids recreated on the
// gateway so far, in a single store update. Enrollment can touch two
// profiles at once (a device's old group and its new one); profile editing
// only ever touches one, via persistProfileAndRuleIDs below.
func (s *Server) persistProfilesAndRuleIDs(updates map[string]store.Profile, recreated map[string][]string) error {
	return s.store.Update(func(d *store.Data) error {
		for i := range d.Profiles {
			if updated, ok := updates[d.Profiles[i].ID]; ok {
				d.Profiles[i] = updated
			}
		}
		for i := range d.Rules {
			if ids, ok := recreated[d.Rules[i].ID]; ok {
				d.Rules[i].UnifiRuleIDs = ids
			}
		}
		return nil
	})
}

// persistProfileAndRuleIDs writes the profile's new name/emoji/color/devices
// together with any rule ids recreated on the gateway so far, in a single
// store update.
func (s *Server) persistProfileAndRuleIDs(id string, updated store.Profile, recreated map[string][]string) error {
	return s.persistProfilesAndRuleIDs(map[string]store.Profile{id: updated}, recreated)
}

// persistPartialProfileUpdate persists the profile's new intent (and any
// rule ids recreated so far) when the gateway-first loop in
// handleProfileUpdate stops partway through. Without this, a mid-loop
// failure would leave rules 1..N-1 already enforcing the new devices on the
// gateway while the store/UI kept showing the old ones — a silent,
// permanent divergence the janitor can't detect (it only reconciles rule-id
// existence, never target-device content). Persisting the intent here means
// a later re-save, or any edit to one of this profile's rules, re-translates
// from the stored profile and converges the rest of the rules (gateway PUTs
// are idempotent, so retrying is safe).
func (s *Server) persistPartialProfileUpdate(id string, updated store.Profile, recreated map[string][]string) {
	if err := s.persistProfileAndRuleIDs(id, updated, recreated); err != nil {
		log.Printf("familytime: failed to persist profile intent after partial gateway failure: %v", err)
	}
}

// persistPartialEnroll is persistPartialProfileUpdate's multi-profile
// counterpart, for enrollment moves that can touch two profiles (the
// device's old group and its new one) in a single request. Same rationale:
// persist the new intent even when the gateway-first retarget loop stops
// partway through, rather than leave the store silently stale.
func (s *Server) persistPartialEnroll(updates map[string]store.Profile, recreated map[string][]string) {
	if err := s.persistProfilesAndRuleIDs(updates, recreated); err != nil {
		log.Printf("familytime: failed to persist enroll intent after partial gateway failure: %v", err)
	}
}

// failPartialProfileUpdate reports a gateway error that stopped
// handleProfileUpdate's retarget loop partway through, after the caller has
// already persisted the new intent via persistPartialProfileUpdate. It keeps
// failErr's status/code mapping (so the UI's existing gateway-error handling
// still applies) but swaps in a message that tells the parent their change
// was saved and a re-save will finish syncing the gateway.
func failPartialProfileUpdate(w http.ResponseWriter, err error) {
	code := "unreachable"
	switch {
	case errors.Is(err, unifi.ErrUnauthorized):
		code = "unauthorized"
	case errors.Is(err, unifi.ErrCertChanged):
		code = "cert_changed"
	case errors.Is(err, unifi.ErrNotFound):
		code = "not_found"
	default:
		log.Printf("familytime: gateway error: %v", err)
	}
	writeJSON(w, http.StatusBadGateway, errBody{
		Error: "Saved your changes, but some of this profile's rules couldn't be updated on the gateway — check your connection and save this profile again.",
		Code:  code,
	})
}

// retargetTranslateError marks a rules.Translate failure encountered inside
// retargetProfileRules, so callers can tell a validation problem (e.g. a
// profile that would end up with zero devices while it still has rules)
// apart from a real gateway failure. Reported as a plain 400 with the
// original message — see respondRetargetError.
type retargetTranslateError struct{ err error }

func (e *retargetTranslateError) Error() string { return e.err.Error() }
func (e *retargetTranslateError) Unwrap() error { return e.err }

// retargetProfileRules re-targets every one of profileRules' gateway rules
// to match updated's devices, gateway-first: each rule is updated in place,
// or recreated if its gateway rule vanished behind our back (e.g. deleted
// in the UniFi app), before the next one is attempted. It returns the ids
// of any rules it had to recreate (family rule id -> new gateway rule
// ids) — reflecting everything it got done before stopping, even when it
// returns an error, so callers can persist that partial progress (see
// persistPartialProfileUpdate / persistPartialEnroll) instead of silently
// discarding gateway state that already changed.
//
// Shared by profile editing (handleProfileUpdate) and device enrollment
// (handleEnroll), which can retarget up to two profiles — the device's old
// group and its new one — in a single request.
func (s *Server) retargetProfileRules(ctx context.Context, updated store.Profile, profileRules []store.FamilyRule) (map[string][]string, error) {
	recreated := map[string][]string{}
	for _, fr := range profileRules {
		tr, err := rules.Translate(fr, updated)
		if err != nil {
			return recreated, &retargetTranslateError{err}
		}
		if len(fr.UnifiRuleIDs) == 1 {
			tr.ID = fr.UnifiRuleIDs[0]
			err = s.api().UpdateTrafficRule(ctx, tr)
		} else {
			err = unifi.ErrNotFound
		}
		if errors.Is(err, unifi.ErrNotFound) {
			// Defensive: clear any stale gateway ids so the recreate can't
			// leave untracked rules enforcing (deleteGatewayRules tolerates
			// not-found).
			if derr := s.deleteGatewayRules(ctx, fr.UnifiRuleIDs); derr != nil {
				return recreated, derr
			}
			tr.ID = ""
			created, cerr := s.api().CreateTrafficRule(ctx, tr)
			if cerr != nil {
				return recreated, cerr
			}
			recreated[fr.ID] = []string{created.ID}
			err = nil
		}
		if err != nil {
			return recreated, err
		}
	}
	return recreated, nil
}

// respondRetargetError reports a retargetProfileRules failure. Callers must
// persist the affected profile(s)' new intent (via
// persistPartialProfileUpdate or persistPartialEnroll) before calling this
// — see retargetProfileRules. Translate/validation failures are reported as
// a plain 400 with the original message; gateway failures get
// failPartialProfileUpdate's friendly "saved, but sync failed" message.
func respondRetargetError(w http.ResponseWriter, err error) {
	var terr *retargetTranslateError
	if errors.As(err, &terr) {
		fail(w, 400, terr.Error())
		return
	}
	failPartialProfileUpdate(w, err)
}

func (s *Server) handleProfileUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == store.EveryoneProfileID {
		fail(w, 400, "The Everyone profile can't be edited.")
		return
	}
	var in profileInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "Bad request.")
		return
	}
	d := s.store.Snapshot()
	var existing *store.Profile
	for i := range d.Profiles {
		if d.Profiles[i].ID == id {
			existing = &d.Profiles[i]
			break
		}
	}
	if existing == nil {
		fail(w, 404, "No such profile.")
		return
	}
	if err := validateProfileInput(d, &in, id); err != nil {
		fail(w, 400, err.Error())
		return
	}
	updated := *existing
	updated.Name, updated.Emoji, updated.Color, updated.Devices = in.Name, in.Emoji, in.Color, in.Devices

	var profileRules []store.FamilyRule
	for _, fr := range d.Rules {
		if fr.ProfileID == id {
			profileRules = append(profileRules, fr)
		}
	}
	if len(updated.Devices) == 0 && len(profileRules) > 0 {
		fail(w, 400, "This profile still has rules — keep at least one device, or delete the rules first.")
		return
	}

	// Gateway first: re-target every one of this profile's rules before
	// reporting success, so a fully successful request never claims a
	// gateway state that isn't real. If the loop stops partway through,
	// each failure path persists the profile's new intent (and any ids
	// recreated so far) anyway — see persistPartialProfileUpdate — because
	// the alternative (silently keeping the old devices in the store while
	// rules 1..N-1 already enforce the new ones on the gateway) is a worse,
	// permanent divergence that nothing else can detect or repair.
	recreated, err := s.retargetProfileRules(r.Context(), updated, profileRules)
	if err != nil {
		s.persistPartialProfileUpdate(id, updated, recreated)
		respondRetargetError(w, err)
		return
	}

	if err := s.persistProfileAndRuleIDs(id, updated, recreated); err != nil {
		fail(w, 500, err.Error())
		return
	}
	// Best-effort: push renames to UniFi as client aliases so both UIs
	// agree. Only devices present before and after with a changed name
	// count — newly added or removed devices are never touched here.
	oldNames := map[string]string{}
	for _, dev := range existing.Devices {
		oldNames[dev.MAC] = dev.Name
	}
	for _, dev := range updated.Devices {
		if oldName, ok := oldNames[dev.MAC]; ok && oldName != dev.Name {
			if err := s.api().RenameClient(r.Context(), dev.MAC, dev.Name); err != nil {
				log.Printf("familytime: unifi rename failed for %s: %v", dev.MAC, err)
			}
		}
	}
	writeJSON(w, 200, updated)
}

// deleteGatewayRules removes gateway rules by id, tolerating rules that
// were already deleted behind our back.
func (s *Server) deleteGatewayRules(ctx context.Context, ids []string) error {
	api := s.api()
	for _, id := range ids {
		if err := api.DeleteTrafficRule(ctx, id); err != nil && !errors.Is(err, unifi.ErrNotFound) {
			return err
		}
	}
	return nil
}

func (s *Server) handleProfileDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == store.EveryoneProfileID {
		fail(w, 400, "The Everyone profile can't be deleted.")
		return
	}
	d := s.store.Snapshot()
	if _, ok := profileForID(d, id); !ok {
		fail(w, 404, "No such profile.")
		return
	}
	// Gateway first: remove this profile's rules so nothing keeps enforcing.
	var gatewayIDs []string
	for _, fr := range d.Rules {
		if fr.ProfileID == id {
			gatewayIDs = append(gatewayIDs, fr.UnifiRuleIDs...)
		}
	}
	if err := s.deleteGatewayRules(r.Context(), gatewayIDs); err != nil {
		failErr(w, err)
		return
	}
	err := s.store.Update(func(d *store.Data) error {
		kept := d.Rules[:0]
		for _, fr := range d.Rules {
			if fr.ProfileID != id {
				kept = append(kept, fr)
			}
		}
		d.Rules = kept
		for i := range d.Profiles {
			if d.Profiles[i].ID == id {
				d.Profiles = append(d.Profiles[:i], d.Profiles[i+1:]...)
				break
			}
		}
		return nil
	})
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
