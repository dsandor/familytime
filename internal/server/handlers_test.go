package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	"familytime/internal/store"
	"familytime/internal/unifi"
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

func TestProfileUpdateRetargetsGatewayRules(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st) // p1, one device: aa:aa:aa:aa:aa:01
	var created store.FamilyRule
	code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p1","name":"No YouTube","what":{"type":"preset","presetId":"youtube"},"when":{"kind":"always"}}`, &created)
	if code != 200 {
		t.Fatalf("create rule = %d", code)
	}

	code = doJSON(t, c, "PUT", ts.URL+"/api/profiles/p1",
		`{"name":"Emma","emoji":"🦄","devices":[{"mac":"aa:aa:aa:aa:aa:01","name":"iPad"},{"mac":"aa:aa:aa:aa:aa:02","name":"Switch"}]}`, nil)
	if code != 200 {
		t.Fatalf("profile update = %d", code)
	}
	gw, ok := fake.ruleByDesc(created.ID)
	if !ok {
		t.Fatal("gateway rule missing")
	}
	if len(gw.TargetDevices) != 2 {
		t.Errorf("gateway rule target devices = %+v, want 2", gw.TargetDevices)
	}
	if got := st.Snapshot().Profiles[0].Devices; len(got) != 2 {
		t.Errorf("store profile devices = %+v, want 2", got)
	}
}

func TestProfileUpdateRejectsZeroDevicesWithRules(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	var created store.FamilyRule
	code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p1","name":"No YouTube","what":{"type":"preset","presetId":"youtube"},"when":{"kind":"always"}}`, &created)
	if code != 200 {
		t.Fatalf("create rule = %d", code)
	}
	before, _ := fake.ruleByDesc(created.ID)

	code = doJSON(t, c, "PUT", ts.URL+"/api/profiles/p1", `{"name":"Emma","devices":[]}`, nil)
	if code != 400 {
		t.Errorf("zero devices with rules = %d, want 400", code)
	}
	after, _ := fake.ruleByDesc(created.ID)
	if len(after.TargetDevices) != len(before.TargetDevices) {
		t.Errorf("gateway rule changed: before=%+v after=%+v", before, after)
	}
	if got := st.Snapshot().Profiles[0].Devices; len(got) != 1 {
		t.Errorf("store profile devices should be unchanged, got %+v", got)
	}
}

func TestProfileUpdateRecreatesVanishedGatewayRule(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	var created store.FamilyRule
	code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p1","name":"No YouTube","what":{"type":"preset","presetId":"youtube"},"when":{"kind":"always"}}`, &created)
	if code != 200 {
		t.Fatalf("create rule = %d", code)
	}

	fake.mu.Lock()
	fake.rules = nil // someone deleted it in the UniFi app
	fake.mu.Unlock()

	code = doJSON(t, c, "PUT", ts.URL+"/api/profiles/p1",
		`{"name":"Emma","emoji":"🦄","devices":[{"mac":"aa:aa:aa:aa:aa:01","name":"iPad"}]}`, nil)
	if code != 200 {
		t.Fatalf("profile update = %d", code)
	}
	if len(fake.rules) != 1 {
		t.Fatal("gateway rule not recreated")
	}
	got := st.Snapshot().Rules[0].UnifiRuleIDs[0]
	if got != fake.rules[0].ID {
		t.Errorf("store id %q != gateway id %q", got, fake.rules[0].ID)
	}
}

// TestProfileUpdatePersistsIntentOnPartialGatewayFailure covers the
// gateway-first retarget loop stopping partway through: rule 1 succeeds on
// the gateway, rule 2's UpdateTrafficRule call fails. The handler must still
// persist the new device list (and any recreated rule ids) to the store —
// leaving the store on the old devices while the gateway already enforces
// the new ones on rule 1 would be a silent, permanent divergence. It should
// report an error so the UI can tell the parent to retry, and a later
// re-save (once the gateway call stops failing) must converge rule 2 too.
func TestProfileUpdatePersistsIntentOnPartialGatewayFailure(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	st.Update(func(d *store.Data) error {
		d.Profiles = []store.Profile{{
			ID: "p1", Name: "Emma", Emoji: "🦄",
			Devices: []store.Device{{MAC: "aa:aa:aa:aa:aa:01", Name: "iPad"}},
		}}
		return nil
	})
	var rule1, rule2 store.FamilyRule
	if code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p1","name":"No YouTube","what":{"type":"preset","presetId":"youtube"},"when":{"kind":"always"}}`, &rule1); code != 200 {
		t.Fatalf("create rule1 = %d", code)
	}
	if code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p1","name":"No TikTok","what":{"type":"preset","presetId":"tiktok"},"when":{"kind":"always"}}`, &rule2); code != 200 {
		t.Fatalf("create rule2 = %d", code)
	}
	gw2, ok := fake.ruleByDesc(rule2.ID)
	if !ok {
		t.Fatal("gateway rule2 missing")
	}

	// Make the second rule's gateway update fail.
	fake.mu.Lock()
	fake.failUpdateID = gw2.ID
	fake.mu.Unlock()

	var errOut errBody
	code := doJSON(t, c, "PUT", ts.URL+"/api/profiles/p1",
		`{"name":"Emma","emoji":"🦄","devices":[{"mac":"aa:aa:aa:aa:aa:01","name":"iPad"},{"mac":"aa:aa:aa:aa:aa:02","name":"Switch"}]}`, &errOut)
	if code < 500 || code >= 600 {
		t.Fatalf("partial gateway failure = %d, want 5xx; body=%+v", code, errOut)
	}
	if errOut.Error == "" {
		t.Error("expected a friendly error message in the response body")
	}

	// (b) store holds the NEW device list despite the mid-loop failure.
	got := st.Snapshot().Profiles[0].Devices
	if len(got) != 2 {
		t.Fatalf("store profile devices = %+v, want 2 (new list persisted)", got)
	}

	// (c) gateway rule 1 has the new targets, rule 2 still has the old ones.
	gw1, ok := fake.ruleByDesc(rule1.ID)
	if !ok {
		t.Fatal("gateway rule1 missing")
	}
	if len(gw1.TargetDevices) != 2 {
		t.Errorf("gateway rule1 target devices = %+v, want 2 (retargeted)", gw1.TargetDevices)
	}
	gw2After, ok := fake.ruleByDesc(rule2.ID)
	if !ok {
		t.Fatal("gateway rule2 missing")
	}
	if len(gw2After.TargetDevices) != 1 {
		t.Errorf("gateway rule2 target devices = %+v, want 1 (unchanged, update failed)", gw2After.TargetDevices)
	}

	// (d) clearing the failure and re-saving the same profile converges
	// rule 2 to the new targets with a 200.
	fake.mu.Lock()
	fake.failUpdateID = ""
	fake.mu.Unlock()

	code = doJSON(t, c, "PUT", ts.URL+"/api/profiles/p1",
		`{"name":"Emma","emoji":"🦄","devices":[{"mac":"aa:aa:aa:aa:aa:01","name":"iPad"},{"mac":"aa:aa:aa:aa:aa:02","name":"Switch"}]}`, nil)
	if code != 200 {
		t.Fatalf("retry profile update = %d, want 200", code)
	}
	gw2Final, ok := fake.ruleByDesc(rule2.ID)
	if !ok {
		t.Fatal("gateway rule2 missing after retry")
	}
	if len(gw2Final.TargetDevices) != 2 {
		t.Errorf("gateway rule2 target devices after retry = %+v, want 2 (converged)", gw2Final.TargetDevices)
	}
}

// TestProfileUpdatePushesRenameForChangedMember verifies that saving a
// profile pushes exactly one UniFi rename — for the one member whose name
// changed — and none for a newly added device.
func TestProfileUpdatePushesRenameForChangedMember(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	st.Update(func(d *store.Data) error {
		d.Profiles = []store.Profile{{
			ID: "p1", Name: "Emma", Emoji: "🦄",
			Devices: []store.Device{{MAC: "aa:aa:aa:aa:aa:01", Name: "iPad"}},
		}}
		return nil
	})

	code := doJSON(t, c, "PUT", ts.URL+"/api/profiles/p1",
		`{"name":"Emma","emoji":"🦄","devices":[{"mac":"aa:aa:aa:aa:aa:01","name":"Emma's iPad"},{"mac":"aa:aa:aa:aa:aa:02","name":"New Switch"}]}`, nil)
	if code != 200 {
		t.Fatalf("profile update = %d", code)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.renames) != 1 {
		t.Fatalf("renames = %+v, want exactly 1 (the renamed member only)", fake.renames)
	}
	if fake.renames[0].MAC != "aa:aa:aa:aa:aa:01" || fake.renames[0].Name != "Emma's iPad" {
		t.Errorf("rename = %+v, want {aa:aa:aa:aa:aa:01 Emma's iPad}", fake.renames[0])
	}
}

// TestProfileUpdateNoChangePushesNoRename verifies that re-saving a profile
// with identical device names pushes zero renames.
func TestProfileUpdateNoChangePushesNoRename(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st) // p1, device aa:aa:aa:aa:aa:01 named "iPad"

	code := doJSON(t, c, "PUT", ts.URL+"/api/profiles/p1",
		`{"name":"Emma","emoji":"🦄","devices":[{"mac":"aa:aa:aa:aa:aa:01","name":"iPad"}]}`, nil)
	if code != 200 {
		t.Fatalf("profile update = %d", code)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.renames) != 0 {
		t.Errorf("renames = %+v, want none for an unchanged name", fake.renames)
	}
}

// TestProfileUpdateRenameFailureIsBestEffort verifies that a UniFi rename
// failure never fails the profile save — the save still returns 200 and the
// store is updated, with the failure only logged.
func TestProfileUpdateRenameFailureIsBestEffort(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	st.Update(func(d *store.Data) error {
		d.Profiles = []store.Profile{{
			ID: "p1", Name: "Emma", Emoji: "🦄",
			Devices: []store.Device{{MAC: "aa:aa:aa:aa:aa:01", Name: "iPad"}},
		}}
		return nil
	})
	fake.mu.Lock()
	fake.failRename = errors.New("simulated rename failure")
	fake.mu.Unlock()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	code := doJSON(t, c, "PUT", ts.URL+"/api/profiles/p1",
		`{"name":"Emma","emoji":"🦄","devices":[{"mac":"aa:aa:aa:aa:aa:01","name":"Emma's iPad"}]}`, nil)
	if code != 200 {
		t.Fatalf("profile update = %d, want 200 despite rename failure", code)
	}
	if got := st.Snapshot().Profiles[0].Devices[0].Name; got != "Emma's iPad" {
		t.Errorf("store name = %q, want the save to persist despite the rename failure", got)
	}
	if !strings.Contains(buf.String(), "unifi rename failed") || !strings.Contains(buf.String(), "simulated rename failure") {
		t.Errorf("expected the rename failure to be logged, got %q", buf.String())
	}
}

func TestDevicesSortsByNameThenMAC(t *testing.T) {
	ts, fake, _, _ := newTestServer(t)
	c := doSetup(t, ts)
	fake.clients = []unifi.NetClient{
		{Name: "Kid Tablet", MACAddress: "bb:bb:bb:bb:bb:02", Type: "WIRED"},
		{Name: "Kid Tablet", MACAddress: "aa:aa:aa:aa:aa:01", Type: "WIRED"},
	}
	var out []struct {
		MAC  string `json:"mac"`
		Name string `json:"name"`
	}
	if code := getJSON(t, c, ts.URL+"/api/devices", &out); code != 200 {
		t.Fatalf("devices = %d", code)
	}
	if len(out) != 2 || out[0].MAC != "aa:aa:aa:aa:aa:01" || out[1].MAC != "bb:bb:bb:bb:bb:02" {
		t.Errorf("equal-name devices not tie-broken by MAC: %+v", out)
	}
}

func TestProfileDeleteCascadesItsRules(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	// Seed: profile with one rule that exists on the gateway.
	fake.rules = []unifi.TrafficRule{{ID: "u1", Description: "[family-time] fr1 No YouTube"}}
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
