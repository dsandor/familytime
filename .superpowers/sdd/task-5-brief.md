# Task 5 brief — Bedtime implementation plan

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



### Task 5: API — presets, devices, profiles

**Files:**
- Create: `internal/server/handlers.go`
- Modify: `internal/server/server.go` (delete the temporary `registerAPIRoutes` stub — `handlers.go` now owns it)
- Test: `internal/server/handlers_test.go`

**Interfaces:**
- Consumes: Task 4 server core + test helpers, `rules` catalogs (Task 3)
- Produces:
  - Routes: `GET /api/presets`, `GET /api/devices`, `GET /api/profiles`, `POST /api/profiles`, `PUT /api/profiles/{id}`, `DELETE /api/profiles/{id}`
  - Helpers Task 6 uses: `profileForID(d store.Data, id string) (store.Profile, bool)` (resolves the virtual Everyone profile), `s.deleteGatewayRules(ctx, ids []string) error` (ignores not-found)
  - JSON shape `deviceInfo{mac, name, ip, connected, wireless, profileId}`

- [ ] **Step 1: Write the failing tests**

Create `internal/server/handlers_test.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"bedtime/internal/store"
	"bedtime/internal/unifi"
)

func getJSON(t *testing.T, c *http.Client, url string, out any) int {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func doJSON(t *testing.T, c *http.Client, method, url, body string, out any) int {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func TestPresetsEndpoint(t *testing.T) {
	ts, _, _, _ := newTestServer(t)
	c := doSetup(t, ts)
	var out struct {
		Presets    []struct{ ID, Name string }
		Categories []struct{ ID, Name string }
	}
	if code := getJSON(t, c, ts.URL+"/api/presets", &out); code != 200 {
		t.Fatalf("presets = %d", code)
	}
	found := false
	for _, p := range out.Presets {
		if p.ID == "youtube" {
			found = true
		}
	}
	if !found || len(out.Categories) != 3 {
		t.Errorf("presets=%+v categories=%+v", out.Presets, out.Categories)
	}
}

func TestDevicesMergesLiveAndAssigned(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	fake.clients = []unifi.NetClient{
		{Name: "Emma iPad", MACAddress: "aa:aa:aa:aa:aa:01", IPAddress: "192.168.0.50", Type: "WIRELESS"},
		{Name: "TV", MACAddress: "aa:aa:aa:aa:aa:02", IPAddress: "192.168.0.51", Type: "WIRED"},
	}
	st.Update(func(d *store.Data) error {
		d.Profiles = []store.Profile{{ID: "p1", Name: "Emma", Devices: []store.Device{
			{MAC: "aa:aa:aa:aa:aa:01", Name: "Emma iPad"},
			{MAC: "aa:aa:aa:aa:aa:99", Name: "Old Switch"}, // offline right now
		}}}
		return nil
	})
	var out []struct {
		MAC       string `json:"mac"`
		Name      string `json:"name"`
		Connected bool   `json:"connected"`
		ProfileID string `json:"profileId"`
	}
	if code := getJSON(t, c, ts.URL+"/api/devices", &out); code != 200 {
		t.Fatalf("devices = %d", code)
	}
	if len(out) != 3 {
		t.Fatalf("got %d devices, want 3 (2 live + 1 assigned offline): %+v", len(out), out)
	}
	byMAC := map[string]struct {
		Name      string
		Connected bool
		ProfileID string
	}{}
	for _, d := range out {
		byMAC[d.MAC] = struct {
			Name      string
			Connected bool
			ProfileID string
		}{d.Name, d.Connected, d.ProfileID}
	}
	if d := byMAC["aa:aa:aa:aa:aa:01"]; !d.Connected || d.ProfileID != "p1" {
		t.Errorf("live assigned device wrong: %+v", d)
	}
	if d := byMAC["aa:aa:aa:aa:aa:99"]; d.Connected || d.ProfileID != "p1" || d.Name != "Old Switch" {
		t.Errorf("offline assigned device wrong: %+v", d)
	}
	if d := byMAC["aa:aa:aa:aa:aa:02"]; d.ProfileID != "" {
		t.Errorf("unassigned device should have empty profileId: %+v", d)
	}
}

func TestProfileCRUD(t *testing.T) {
	ts, _, st, _ := newTestServer(t)
	c := doSetup(t, ts)

	var created store.Profile
	code := doJSON(t, c, "POST", ts.URL+"/api/profiles",
		`{"name":"Emma","emoji":"🦄","color":"#b57edc","devices":[{"mac":"AA:BB:CC:DD:EE:01","name":"iPad"}]}`, &created)
	if code != 200 || created.ID == "" || created.Name != "Emma" {
		t.Fatalf("create = %d, %+v", code, created)
	}
	if created.Devices[0].MAC != "aa:bb:cc:dd:ee:01" {
		t.Errorf("MAC not normalized to lowercase: %q", created.Devices[0].MAC)
	}

	var list []store.Profile
	getJSON(t, c, ts.URL+"/api/profiles", &list)
	if len(list) != 1 {
		t.Fatalf("list = %+v", list)
	}

	code = doJSON(t, c, "PUT", ts.URL+"/api/profiles/"+created.ID,
		`{"name":"Emma R","emoji":"🦄","color":"#b57edc","devices":[]}`, nil)
	if code != 200 || st.Snapshot().Profiles[0].Name != "Emma R" {
		t.Errorf("update = %d, store = %+v", code, st.Snapshot().Profiles)
	}

	if code := doJSON(t, c, "DELETE", ts.URL+"/api/profiles/"+created.ID, "", nil); code != 200 {
		t.Errorf("delete = %d", code)
	}
	if len(st.Snapshot().Profiles) != 0 {
		t.Error("profile not removed from store")
	}
}

func TestProfileValidation(t *testing.T) {
	ts, _, _, _ := newTestServer(t)
	c := doSetup(t, ts)
	if code := doJSON(t, c, "POST", ts.URL+"/api/profiles", `{"name":""}`, nil); code != 400 {
		t.Errorf("empty name = %d, want 400", code)
	}
	if code := doJSON(t, c, "PUT", ts.URL+"/api/profiles/everyone", `{"name":"x"}`, nil); code != 400 {
		t.Errorf("editing everyone = %d, want 400", code)
	}
	if code := doJSON(t, c, "DELETE", ts.URL+"/api/profiles/everyone", "", nil); code != 400 {
		t.Errorf("deleting everyone = %d, want 400", code)
	}
	if code := doJSON(t, c, "PUT", ts.URL+"/api/profiles/nope", `{"name":"x"}`, nil); code != 404 {
		t.Errorf("unknown profile = %d, want 404", code)
	}
}

func TestProfileRejectsMACAssignedElsewhere(t *testing.T) {
	ts, _, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	st.Update(func(d *store.Data) error {
		d.Profiles = []store.Profile{{ID: "p1", Name: "Emma", Devices: []store.Device{{MAC: "aa:aa:aa:aa:aa:01", Name: "iPad"}}}}
		return nil
	})
	code := doJSON(t, c, "POST", ts.URL+"/api/profiles",
		`{"name":"Jack","devices":[{"mac":"aa:aa:aa:aa:aa:01","name":"iPad"}]}`, nil)
	if code != 400 {
		t.Errorf("duplicate MAC across profiles = %d, want 400", code)
	}
}

func TestProfileDeleteCascadesItsRules(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	// Seed: profile with one rule that exists on the gateway.
	fake.rules = []unifi.TrafficRule{{ID: "u1", Description: "[bedtime] fr1 No YouTube"}}
	st.Update(func(d *store.Data) error {
		d.Profiles = []store.Profile{{ID: "p1", Name: "Emma", Devices: []store.Device{{MAC: "aa:aa:aa:aa:aa:01"}}}}
		d.Rules = []store.FamilyRule{{ID: "fr1", ProfileID: "p1", Name: "No YouTube", UnifiRuleIDs: []string{"u1"}}}
		return nil
	})
	if code := doJSON(t, c, "DELETE", ts.URL+"/api/profiles/p1", "", nil); code != 200 {
		t.Fatalf("delete = %d", code)
	}
	d := st.Snapshot()
	if len(d.Profiles) != 0 || len(d.Rules) != 0 {
		t.Errorf("store not cascaded: %+v", d)
	}
	if len(fake.rules) != 0 {
		t.Errorf("gateway rule not deleted: %+v", fake.rules)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -v -run 'TestPresets|TestDevices|TestProfile'`
Expected: FAIL — routes return 404 (only the Task 4 stub exists).

- [ ] **Step 3: Implement the handlers**

Delete the `registerAPIRoutes` stub from `internal/server/server.go`, then create `internal/server/handlers.go`:

```go
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"bedtime/internal/rules"
	"bedtime/internal/store"
	"bedtime/internal/unifi"
)

// registerAPIRoutes wires the protected JSON API (auth-gated).
func (s *Server) registerAPIRoutes() {
	s.mux.Handle("GET /api/presets", s.auth(s.handlePresets))
	s.mux.Handle("GET /api/devices", s.auth(s.handleDevices))
	s.mux.Handle("GET /api/profiles", s.auth(s.handleProfilesList))
	s.mux.Handle("POST /api/profiles", s.auth(s.handleProfileCreate))
	s.mux.Handle("PUT /api/profiles/{id}", s.auth(s.handleProfileUpdate))
	s.mux.Handle("DELETE /api/profiles/{id}", s.auth(s.handleProfileDelete))
	// Temporary stub until the rules/status task:
	s.mux.Handle("GET /api/status", s.auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"todo": "task 6"})
	}))
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
	assigned := map[string]string{}  // mac → profile id
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
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
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
	var updated store.Profile
	found := false
	err := s.store.Update(func(d *store.Data) error {
		for i := range d.Profiles {
			if d.Profiles[i].ID != id {
				continue
			}
			found = true
			if err := validateProfileInput(*d, &in, id); err != nil {
				return err
			}
			d.Profiles[i].Name, d.Profiles[i].Emoji, d.Profiles[i].Color, d.Profiles[i].Devices = in.Name, in.Emoji, in.Color, in.Devices
			updated = d.Profiles[i]
			return nil
		}
		return nil
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if !found {
		fail(w, 404, "No such profile.")
		return
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/ -v`
Expected: PASS — all Task 4 + Task 5 tests (14).

- [ ] **Step 5: Full verification**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all pass.

---

