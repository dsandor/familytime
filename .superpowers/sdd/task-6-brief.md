# Task 6 brief — Bedtime implementation plan

## Global Constraints

- **NEVER run any `git` command.** No `git init`, no commits, no pushes — the user's team handles version control. This overrides the usual commit-per-step workflow: where a normal plan says "commit", instead just re-run the verification commands.
- Project root: `/Users/dsandor/Projects/bedtime`. All commands run from there unless a step says otherwise.
- Module name: `bedtime`. Go `1.24`.
- Only external Go dependency allowed: `golang.org/x/crypto` (bcrypt). Everything else stdlib.
- Every UniFi rule this app creates has a description starting with exactly `[bedtime] `. Never modify or delete a gateway rule whose description lacks that prefix.
- Store file permissions 0600; writes are temp-file + rename (atomic).
- Defaults: port `8080`, data file `<os.UserConfigDir()>/bedtime/bedtime.json`.
- After every task: `go build ./... && go vet ./... && go test ./...` must pass.
- `ls` is aliased on this machine — use `/bin/ls` in shell commands.
- The live gateway is at `https://192.168.0.1` with the API key in `.env` (`UNIFI_API_KEY`). **Do not create/modify/delete anything on the gateway except in Task 10 (opt-in E2E), and never touch rules lacking the `[bedtime]`/`[bedtime-e2e]` prefix.** The user's real rule "kids apps" must survive untouched.



## Verified UniFi v2 API facts (probed live 2026-07-02 — treat as ground truth)

- Base: `https://192.168.0.1/proxy/network/v2/api/site/default/trafficrules`, header `X-API-KEY`, self-signed TLS.
- Writes return **200 or 201** interchangeably. No GET-by-id — list and filter. DELETE returns 200.
- `schedule.mode`: `ALWAYS`, `EVERY_DAY`, `EVERY_WEEK` (+ `repeat_on_days: ["sun".."sat"]`), `ONE_TIME_ONLY` (+ single `date: "YYYY-MM-DD"`).
- Time ranges crossing midnight (`21:00`→`07:00`) are **accepted natively** — no rule splitting.
- `target_devices`: `{"client_mac": "aa:bb:…", "type": "CLIENT"}` or `{"type": "ALL_CLIENTS"}`.
- `matching_target`: `DOMAIN` (with `domains: [{domain, ports: [], port_ranges: []}]`), `APP_CATEGORY` (with `app_category_ids` as **integers**), `INTERNET` (full block).
- Captured real payloads: `internal/unifi/testdata/trafficrules_probe.json` (already in the repo — 4 probe rules covering weekly/midnight/ALL_CLIENTS/one-time shapes).
- Official v1 API (`/proxy/network/integration/v1/…`) works for `info`, `sites`, `sites/{id}/clients` (paginated envelope `{offset,limit,count,totalCount,data}`) — used read-only for device inventory.



### Task 6: API — rules, pause, status, settings

**Files:**
- Create: `internal/server/rules_handlers.go`, `internal/server/settings_handlers.go`
- Modify: `internal/server/handlers.go` (`registerAPIRoutes` — drop the status stub, add the new routes)
- Test: `internal/server/rules_handlers_test.go`

**Interfaces:**
- Consumes: Tasks 3–5 (`rules.Translate/ActiveNow`, `profileForID`, `deleteGatewayRules`, `createFamilyRule` introduced here)
- Produces routes:
  - `GET/POST /api/rules`, `PUT/DELETE /api/rules/{id}`
  - `POST /api/pause` `{profileId, duration: "30m"|"1h"|"morning"|"indefinite"}`, `DELETE /api/pause/{profileId}`
  - `GET /api/status` → `[]profileStatus{id, name, emoji, color, deviceCount, paused, pausedUntil, lines: []statusLine{ruleId, name, label, active, until, pause}}`
  - `GET /api/settings`, `PUT /api/settings/pin`, `PUT /api/settings/gateway`, `POST /api/settings/trust-cert`

- [ ] **Step 1: Replace the routes registration**

In `internal/server/handlers.go`, replace the whole `registerAPIRoutes` function with:

```go
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
	s.mux.Handle("GET /api/status", s.auth(s.handleStatus))
	s.mux.Handle("GET /api/settings", s.auth(s.handleSettingsGet))
	s.mux.Handle("PUT /api/settings/pin", s.auth(s.handleSettingsPIN))
	s.mux.Handle("PUT /api/settings/gateway", s.auth(s.handleSettingsGateway))
	s.mux.Handle("POST /api/settings/trust-cert", s.auth(s.handleTrustCert))
}
```

- [ ] **Step 2: Write the failing tests**

Create `internal/server/rules_handlers_test.go`:

```go
package server

import (
	"net/http"
	"testing"
	"time"

	"bedtime/internal/store"
	"bedtime/internal/unifi"
)

// seedProfile stores a profile with one device and returns its id.
func seedProfile(t *testing.T, st *store.Store) string {
	t.Helper()
	err := st.Update(func(d *store.Data) error {
		d.Profiles = append(d.Profiles, store.Profile{
			ID: "p1", Name: "Emma", Emoji: "🦄",
			Devices: []store.Device{{MAC: "aa:aa:aa:aa:aa:01", Name: "iPad"}},
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return "p1"
}

func fixedNow(t *testing.T, srv *Server, stamp string) time.Time {
	t.Helper()
	now, err := time.ParseInLocation("2006-01-02 15:04", stamp, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	srv.now = func() time.Time { return now }
	return now
}

func TestRuleLifecycle(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)

	var created store.FamilyRule
	code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p1","name":"No YouTube","what":{"type":"preset","presetId":"youtube"},
		  "when":{"kind":"recurring","days":["sun","mon","tue","wed","thu"],"start":"20:00","end":"07:00"}}`, &created)
	if code != 200 || created.ID == "" || len(created.UnifiRuleIDs) != 1 {
		t.Fatalf("create = %d, %+v", code, created)
	}
	gw, ok := fake.ruleByDesc(created.ID)
	if !ok {
		t.Fatal("gateway rule not created")
	}
	if gw.Schedule.Mode != unifi.ModeEveryWeek || gw.MatchingTarget != unifi.MatchDomain || !gw.Enabled {
		t.Errorf("gateway rule wrong: %+v", gw)
	}

	// Update: rename + disable.
	code = doJSON(t, c, "PUT", ts.URL+"/api/rules/"+created.ID,
		`{"name":"YouTube off","what":{"type":"preset","presetId":"youtube"},
		  "when":{"kind":"always"},"enabled":false}`, nil)
	if code != 200 {
		t.Fatalf("update = %d", code)
	}
	gw, _ = fake.ruleByDesc(created.ID)
	if gw.Enabled || gw.Schedule.Mode != unifi.ModeAlways {
		t.Errorf("gateway rule not updated: %+v", gw)
	}
	if st.Snapshot().Rules[0].Name != "YouTube off" || st.Snapshot().Rules[0].Enabled {
		t.Errorf("store not updated: %+v", st.Snapshot().Rules)
	}

	// Delete.
	if code := doJSON(t, c, "DELETE", ts.URL+"/api/rules/"+created.ID, "", nil); code != 200 {
		t.Fatalf("delete = %d", code)
	}
	if len(st.Snapshot().Rules) != 0 || len(fake.rules) != 0 {
		t.Error("rule not fully deleted")
	}
}

func TestRuleCreateValidation(t *testing.T) {
	ts, _, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	if code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"nope","name":"x","what":{"type":"everything"},"when":{"kind":"always"}}`, nil); code != 404 {
		t.Errorf("unknown profile = %d, want 404", code)
	}
	// Profile with no devices → Translate error → 400.
	st.Update(func(d *store.Data) error {
		d.Profiles = append(d.Profiles, store.Profile{ID: "p2", Name: "Empty"})
		return nil
	})
	if code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p2","name":"x","what":{"type":"everything"},"when":{"kind":"always"}}`, nil); code != 400 {
		t.Errorf("empty profile = %d, want 400", code)
	}
	if code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"everyone","name":"","what":{"type":"everything"},"when":{"kind":"always"}}`, nil); code != 400 {
		t.Errorf("empty name = %d, want 400", code)
	}
}

func TestRuleCreateGatewayDownPersistsNothing(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	fake.failAll = http.ErrHandlerTimeout
	code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p1","name":"x","what":{"type":"everything"},"when":{"kind":"always"}}`, nil)
	if code != 502 {
		t.Errorf("gateway down = %d, want 502", code)
	}
	if len(st.Snapshot().Rules) != 0 {
		t.Error("nothing may persist when the gateway write failed")
	}
}

func TestRuleUpdateRecreatesVanishedGatewayRule(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	var created store.FamilyRule
	doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p1","name":"x","what":{"type":"everything"},"when":{"kind":"always"}}`, &created)

	fake.mu.Lock()
	fake.rules = nil // someone deleted it in the UniFi app
	fake.mu.Unlock()

	code := doJSON(t, c, "PUT", ts.URL+"/api/rules/"+created.ID,
		`{"name":"x","what":{"type":"everything"},"when":{"kind":"always"},"enabled":true}`, nil)
	if code != 200 {
		t.Fatalf("update = %d", code)
	}
	if len(fake.rules) != 1 {
		t.Fatal("gateway rule not recreated")
	}
	if got := st.Snapshot().Rules[0].UnifiRuleIDs[0]; got != fake.rules[0].ID {
		t.Errorf("store id %q != gateway id %q", got, fake.rules[0].ID)
	}
}

func TestPauseReplaceAndUnpause(t *testing.T) {
	ts, fake, st, srv := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	now := fixedNow(t, srv, "2026-07-03 21:30") // Friday evening

	if code := doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"30m"}`, nil); code != 200 {
		t.Fatalf("pause = %d", code)
	}
	rules := st.Snapshot().Rules
	if len(rules) != 1 || !rules[0].Pause || rules[0].What.Type != store.WhatEverything {
		t.Fatalf("pause rule wrong: %+v", rules)
	}
	until, _ := time.Parse(time.RFC3339, rules[0].When.Until)
	if !until.Equal(now.Add(30 * time.Minute)) {
		t.Errorf("until = %v, want %v", until, now.Add(30*time.Minute))
	}

	// Pausing again replaces (no stacking).
	doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"morning"}`, nil)
	rules = st.Snapshot().Rules
	if len(rules) != 1 {
		t.Fatalf("pause must replace, got %d rules", len(rules))
	}
	until, _ = time.Parse(time.RFC3339, rules[0].When.Until)
	want := time.Date(2026, 7, 4, 7, 0, 0, 0, time.Local) // next morning 07:00
	if !until.Equal(want) {
		t.Errorf("morning until = %v, want %v", until, want)
	}
	if len(fake.rules) != 1 {
		t.Errorf("gateway must hold exactly 1 pause rule, got %d", len(fake.rules))
	}

	// Indefinite pause has no Until.
	doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"indefinite"}`, nil)
	if got := st.Snapshot().Rules[0].When; got.Kind != store.WhenAlways {
		t.Errorf("indefinite = %+v", got)
	}

	if code := doJSON(t, c, "DELETE", ts.URL+"/api/pause/p1", "", nil); code != 200 {
		t.Fatalf("unpause = %d", code)
	}
	if len(st.Snapshot().Rules) != 0 || len(fake.rules) != 0 {
		t.Error("unpause must remove the pause rule everywhere")
	}
	// Unpause with no pause active is fine (idempotent).
	if code := doJSON(t, c, "DELETE", ts.URL+"/api/pause/p1", "", nil); code != 200 {
		t.Errorf("second unpause = %d, want 200", code)
	}
}

func TestStatusReportsActiveWindows(t *testing.T) {
	ts, _, st, srv := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	fixedNow(t, srv, "2026-07-03 21:30") // Friday 21:30

	doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p1","name":"School nights","what":{"type":"preset","presetId":"youtube"},
		  "when":{"kind":"recurring","days":["fri"],"start":"21:00","end":"07:00"}}`, nil)
	doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p1","name":"Weekend mornings","what":{"type":"preset","presetId":"roblox"},
		  "when":{"kind":"recurring","days":["sat"],"start":"08:00","end":"12:00"}}`, nil)
	doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"30m"}`, nil)

	var out []struct {
		ID          string `json:"id"`
		Paused      bool   `json:"paused"`
		PausedUntil string `json:"pausedUntil"`
		Lines       []struct {
			Label  string `json:"label"`
			Active bool   `json:"active"`
			Until  string `json:"until"`
			Pause  bool   `json:"pause"`
		} `json:"lines"`
	}
	if code := getJSON(t, c, ts.URL+"/api/status", &out); code != 200 {
		t.Fatalf("status = %d", code)
	}
	if len(out) != 2 || out[0].ID != store.EveryoneProfileID {
		t.Fatalf("want [everyone, emma], got %+v", out)
	}
	emma := out[1]
	if !emma.Paused || emma.PausedUntil == "" {
		t.Errorf("emma should be paused: %+v", emma)
	}
	var activeCount int
	for _, l := range emma.Lines {
		if l.Active {
			activeCount++
		}
	}
	// Active: YouTube (Fri 21:00–07:00 window) + the pause. Roblox (Sat morning) inactive.
	if activeCount != 2 {
		t.Errorf("want 2 active lines, got %d: %+v", activeCount, emma.Lines)
	}
}

func TestSettingsPINChange(t *testing.T) {
	ts, _, _, _ := newTestServer(t)
	c := doSetup(t, ts)
	if code := doJSON(t, c, "PUT", ts.URL+"/api/settings/pin", `{"currentPin":"9999","newPin":"5678"}`, nil); code != 401 {
		t.Errorf("wrong current pin = %d, want 401", code)
	}
	if code := doJSON(t, c, "PUT", ts.URL+"/api/settings/pin", `{"currentPin":"1234","newPin":"5678"}`, nil); code != 200 {
		t.Errorf("pin change = %d, want 200", code)
	}
	fresh := client(t)
	if code := doJSON(t, fresh, "POST", ts.URL+"/api/login", `{"pin":"5678"}`, nil); code != 200 {
		t.Errorf("login with new pin = %d, want 200", code)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/server/ -v -run 'TestRule|TestPause|TestStatus|TestSettings'`
Expected: FAIL — compile error (`handleRulesList` etc. undefined).

- [ ] **Step 4: Implement rules/pause/status handlers**

Create `internal/server/rules_handlers.go`:

```go
package server

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"bedtime/internal/rules"
	"bedtime/internal/store"
	"bedtime/internal/unifi"
)

type ruleInput struct {
	ProfileID string      `json:"profileId"`
	Name      string      `json:"name"`
	What      store.What  `json:"what"`
	When      store.When  `json:"when"`
	Enabled   *bool       `json:"enabled"`
}

// createFamilyRule translates and applies a family rule gateway-first, so
// the store never references a rule the gateway doesn't have. Returns the
// stored rule and an HTTP status (0 means: call failErr with err).
func (s *Server) createFamilyRule(ctx context.Context, fr store.FamilyRule, p store.Profile) (store.FamilyRule, int, error) {
	tr, err := rules.Translate(fr, p)
	if err != nil {
		return fr, http.StatusBadRequest, err
	}
	created, err := s.api().CreateTrafficRule(ctx, tr)
	if err != nil {
		return fr, 0, err
	}
	fr.UnifiRuleIDs = []string{created.ID}
	err = s.store.Update(func(d *store.Data) error {
		d.Rules = append(d.Rules, fr)
		return nil
	})
	if err != nil {
		// Compensate: don't leave an untracked rule enforcing on the gateway.
		s.deleteGatewayRules(ctx, fr.UnifiRuleIDs)
		return fr, http.StatusInternalServerError, err
	}
	return fr, http.StatusOK, nil
}

func (s *Server) handleRuleCreate(w http.ResponseWriter, r *http.Request) {
	var in ruleInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "Bad request.")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		fail(w, 400, "Give this rule a name.")
		return
	}
	d := s.store.Snapshot()
	p, ok := profileForID(d, in.ProfileID)
	if !ok {
		fail(w, 404, "No such profile.")
		return
	}
	fr := store.FamilyRule{
		ID: store.NewID(), ProfileID: p.ID, Name: in.Name,
		What: in.What, When: in.When,
		Enabled: in.Enabled == nil || *in.Enabled,
	}
	fr, code, err := s.createFamilyRule(r.Context(), fr, p)
	if err != nil {
		if code == 0 {
			failErr(w, err)
		} else {
			fail(w, code, err.Error())
		}
		return
	}
	writeJSON(w, 200, fr)
}

func (s *Server) handleRuleUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in ruleInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "Bad request.")
		return
	}
	d := s.store.Snapshot()
	var existing *store.FamilyRule
	for i := range d.Rules {
		if d.Rules[i].ID == id {
			existing = &d.Rules[i]
			break
		}
	}
	if existing == nil {
		fail(w, 404, "No such rule.")
		return
	}
	p, ok := profileForID(d, existing.ProfileID)
	if !ok {
		fail(w, 500, "Rule's profile is missing.")
		return
	}
	updated := *existing
	updated.Name = strings.TrimSpace(in.Name)
	updated.What, updated.When = in.What, in.When
	if in.Enabled != nil {
		updated.Enabled = *in.Enabled
	}
	if updated.Name == "" {
		fail(w, 400, "Give this rule a name.")
		return
	}
	tr, err := rules.Translate(updated, p)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	// Rewrite the existing gateway rule; if it vanished (deleted in the
	// UniFi app), recreate it.
	if len(updated.UnifiRuleIDs) == 1 {
		tr.ID = updated.UnifiRuleIDs[0]
		err = s.api().UpdateTrafficRule(r.Context(), tr)
	} else {
		err = unifi.ErrNotFound
	}
	if errors.Is(err, unifi.ErrNotFound) {
		tr.ID = ""
		created, cerr := s.api().CreateTrafficRule(r.Context(), tr)
		if cerr != nil {
			failErr(w, cerr)
			return
		}
		updated.UnifiRuleIDs = []string{created.ID}
		err = nil
	}
	if err != nil {
		failErr(w, err)
		return
	}
	serr := s.store.Update(func(d *store.Data) error {
		for i := range d.Rules {
			if d.Rules[i].ID == id {
				d.Rules[i] = updated
			}
		}
		return nil
	})
	if serr != nil {
		fail(w, 500, serr.Error())
		return
	}
	writeJSON(w, 200, updated)
}

func (s *Server) handleRuleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d := s.store.Snapshot()
	var target *store.FamilyRule
	for i := range d.Rules {
		if d.Rules[i].ID == id {
			target = &d.Rules[i]
			break
		}
	}
	if target == nil {
		fail(w, 404, "No such rule.")
		return
	}
	if err := s.deleteGatewayRules(r.Context(), target.UnifiRuleIDs); err != nil {
		failErr(w, err)
		return
	}
	serr := s.store.Update(func(d *store.Data) error {
		for i := range d.Rules {
			if d.Rules[i].ID == id {
				d.Rules = append(d.Rules[:i], d.Rules[i+1:]...)
				break
			}
		}
		return nil
	})
	if serr != nil {
		fail(w, 500, serr.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type ruleView struct {
	store.FamilyRule
	Active bool   `json:"active"`
	Until  string `json:"until,omitempty"`
}

func (s *Server) handleRulesList(w http.ResponseWriter, r *http.Request) {
	d := s.store.Snapshot()
	now := s.now()
	out := []ruleView{}
	for _, fr := range d.Rules {
		v := ruleView{FamilyRule: fr}
		if fr.Enabled {
			active, until, hasUntil := rules.ActiveNow(fr.When, now)
			v.Active = active
			if hasUntil {
				v.Until = until.Format(time.RFC3339)
			}
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
}

// removePauseRules deletes all pause rules for a profile, gateway-first.
func (s *Server) removePauseRules(ctx context.Context, profileID string) error {
	d := s.store.Snapshot()
	var ids []string
	for _, fr := range d.Rules {
		if fr.ProfileID == profileID && fr.Pause {
			ids = append(ids, fr.UnifiRuleIDs...)
		}
	}
	if err := s.deleteGatewayRules(ctx, ids); err != nil {
		return err
	}
	return s.store.Update(func(d *store.Data) error {
		kept := d.Rules[:0]
		for _, fr := range d.Rules {
			if !(fr.ProfileID == profileID && fr.Pause) {
				kept = append(kept, fr)
			}
		}
		d.Rules = kept
		return nil
	})
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ProfileID string `json:"profileId"`
		Duration  string `json:"duration"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "Bad request.")
		return
	}
	d := s.store.Snapshot()
	p, ok := profileForID(d, in.ProfileID)
	if !ok {
		fail(w, 404, "No such profile.")
		return
	}
	now := s.now()
	var when store.When
	switch in.Duration {
	case "30m":
		when = oneTimeUntil(now, now.Add(30*time.Minute))
	case "1h":
		when = oneTimeUntil(now, now.Add(time.Hour))
	case "morning":
		morning := time.Date(now.Year(), now.Month(), now.Day(), 7, 0, 0, 0, now.Location())
		if !now.Before(morning) {
			morning = morning.AddDate(0, 0, 1)
		}
		when = oneTimeUntil(now, morning)
	case "indefinite":
		when = store.When{Kind: store.WhenAlways}
	default:
		fail(w, 400, "Unknown pause duration.")
		return
	}
	// Replace any existing pause — tapping Pause twice must not stack rules.
	if err := s.removePauseRules(r.Context(), p.ID); err != nil {
		failErr(w, err)
		return
	}
	fr := store.FamilyRule{
		ID: store.NewID(), ProfileID: p.ID, Name: "Internet pause",
		What: store.What{Type: store.WhatEverything}, When: when,
		Enabled: true, Pause: true,
	}
	fr, code, err := s.createFamilyRule(r.Context(), fr, p)
	if err != nil {
		if code == 0 {
			failErr(w, err)
		} else {
			fail(w, code, err.Error())
		}
		return
	}
	writeJSON(w, 200, fr)
}

func oneTimeUntil(now, until time.Time) store.When {
	return store.When{
		Kind:  store.WhenOneTime,
		Start: now.Format("15:04"),
		Until: until.Format(time.RFC3339),
	}
}

func (s *Server) handleUnpause(w http.ResponseWriter, r *http.Request) {
	if err := s.removePauseRules(r.Context(), r.PathValue("profileId")); err != nil {
		failErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type statusLine struct {
	RuleID string `json:"ruleId"`
	Name   string `json:"name"`
	Label  string `json:"label"`
	Active bool   `json:"active"`
	Until  string `json:"until,omitempty"`
	Pause  bool   `json:"pause"`
}

type profileStatus struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Emoji       string       `json:"emoji"`
	Color       string       `json:"color"`
	DeviceCount int          `json:"deviceCount"`
	Paused      bool         `json:"paused"`
	PausedUntil string       `json:"pausedUntil,omitempty"`
	Lines       []statusLine `json:"lines"`
}

func whatLabel(wt store.What) string {
	switch wt.Type {
	case store.WhatPreset:
		if p, ok := rules.PresetByID(wt.PresetID); ok {
			return p.Name
		}
		return wt.PresetID
	case store.WhatCategory:
		if c, ok := rules.CategoryByID(wt.CategoryID); ok {
			return c.Name
		}
		return wt.CategoryID
	case store.WhatDomains:
		return strings.Join(wt.Domains, ", ")
	case store.WhatEverything:
		return "All internet"
	}
	return wt.Type
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	d := s.store.Snapshot()
	now := s.now()
	everyone, _ := profileForID(d, store.EveryoneProfileID)
	profiles := append([]store.Profile{everyone}, d.Profiles...)
	sort.SliceStable(profiles[1:], func(i, j int) bool { return profiles[i+1].Name < profiles[j+1].Name })

	out := []profileStatus{}
	for _, p := range profiles {
		ps := profileStatus{ID: p.ID, Name: p.Name, Emoji: p.Emoji, Color: p.Color, DeviceCount: len(p.Devices), Lines: []statusLine{}}
		for _, fr := range d.Rules {
			if fr.ProfileID != p.ID || !fr.Enabled {
				continue
			}
			active, until, hasUntil := rules.ActiveNow(fr.When, now)
			line := statusLine{RuleID: fr.ID, Name: fr.Name, Label: whatLabel(fr.What), Active: active, Pause: fr.Pause}
			if hasUntil {
				line.Until = until.Format(time.RFC3339)
			}
			ps.Lines = append(ps.Lines, line)
			if fr.Pause && active {
				ps.Paused = true
				ps.PausedUntil = line.Until
			}
		}
		out = append(out, ps)
	}
	writeJSON(w, 200, out)
}
```

- [ ] **Step 5: Implement settings handlers**

Create `internal/server/settings_handlers.go`:

```go
package server

import (
	"net/http"

	"bedtime/internal/store"
	"bedtime/internal/unifi"
)

func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	g := s.store.Snapshot().Gateway
	writeJSON(w, 200, map[string]string{
		"host":     g.Host,
		"siteName": g.SiteName,
		"dataPath": s.store.Path(),
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
	if err := s.store.Update(func(d *store.Data) error { d.Auth.PINHash = h; return nil }); err != nil {
		fail(w, 500, err.Error())
		return
	}
	s.guard.reset()
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
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/server/ -v`
Expected: PASS — all server tests so far (21).

- [ ] **Step 7: Full verification**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all pass.

---

