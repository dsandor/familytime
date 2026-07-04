package server

import (
	"bytes"
	"errors"
	"log"
	"os"
	"strings"
	"testing"

	"familytime/internal/store"
	"familytime/internal/unifi"
)

// httptest requests always arrive with RemoteAddr "127.0.0.1:<port>", so
// every enroll test identifies "this device" by giving a fake client
// IPAddress of "127.0.0.1".
const enrollTestIP = "127.0.0.1"

func TestEnrollWhoamiFound(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	doSetup(t, ts) // configures the store; enroll itself needs no session
	seedProfile(t, st)
	fake.clients = []unifi.NetClient{
		{Name: "iPhone 72:68", MACAddress: "AA:BB:CC:DD:EE:99", IPAddress: enrollTestIP, Type: "WIRELESS"},
	}

	c := client(t) // fresh, unauthenticated client
	var out struct {
		Found             bool
		MAC               string
		Name              string
		IP                string
		CurrentProfileID  string `json:"currentProfileId"`
		CurrentDeviceName string `json:"currentDeviceName"`
		Groups            []struct{ ID, Name, Emoji, Color string }
	}
	if code := getJSON(t, c, ts.URL+"/api/enroll/whoami", &out); code != 200 {
		t.Fatalf("whoami = %d", code)
	}
	if !out.Found || out.MAC != "aa:bb:cc:dd:ee:99" || out.Name != "iPhone 72:68" || out.IP != enrollTestIP {
		t.Fatalf("unexpected whoami response: %+v", out)
	}
	if out.CurrentProfileID != "" || out.CurrentDeviceName != "" {
		t.Errorf("device isn't assigned yet, want empty current fields: %+v", out)
	}
	if len(out.Groups) != 1 || out.Groups[0].ID != "p1" || out.Groups[0].Name != "Emma" {
		t.Errorf("groups = %+v, want the seeded profile", out.Groups)
	}
}

func TestEnrollWhoamiFoundAlreadyAssigned(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	doSetup(t, ts)
	seedProfile(t, st) // p1, device aa:aa:aa:aa:aa:01 named "iPad"
	fake.clients = []unifi.NetClient{
		{Name: "iPhone 72:68", MACAddress: "aa:aa:aa:aa:aa:01", IPAddress: enrollTestIP, Type: "WIRELESS"},
	}

	c := client(t)
	var out struct {
		Found             bool
		CurrentProfileID  string `json:"currentProfileId"`
		CurrentDeviceName string `json:"currentDeviceName"`
	}
	if code := getJSON(t, c, ts.URL+"/api/enroll/whoami", &out); code != 200 {
		t.Fatalf("whoami = %d", code)
	}
	if !out.Found || out.CurrentProfileID != "p1" || out.CurrentDeviceName != "iPad" {
		t.Errorf("expected the existing assignment to be reported: %+v", out)
	}
}

func TestEnrollWhoamiNotFound(t *testing.T) {
	ts, _, st, _ := newTestServer(t)
	doSetup(t, ts)
	seedProfile(t, st)
	// No fake clients at all — the requester's IP won't match anything.

	c := client(t)
	var out struct {
		Found  bool
		Groups []struct{ ID string }
	}
	code := getJSON(t, c, ts.URL+"/api/enroll/whoami", &out)
	if code != 200 {
		t.Fatalf("whoami not-found = %d, want 200", code)
	}
	if out.Found {
		t.Error("expected found=false")
	}
	if len(out.Groups) != 1 {
		t.Errorf("groups should still be returned when not found: %+v", out.Groups)
	}
}

func TestEnrollWhoamiUnconfigured(t *testing.T) {
	ts, _, _, _ := newTestServer(t)
	c := client(t)
	var out errBody
	code := getJSON(t, c, ts.URL+"/api/enroll/whoami", &out)
	if code != 409 {
		t.Fatalf("whoami unconfigured = %d, want 409", code)
	}
	if out.Error == "" {
		t.Error("expected a friendly message")
	}
}

func TestEnrollAssignsToProfile(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	doSetup(t, ts)
	seedProfile(t, st) // p1
	fake.clients = []unifi.NetClient{
		{Name: "iPhone 72:68", MACAddress: "AA:BB:CC:DD:EE:99", IPAddress: enrollTestIP, Type: "WIRELESS"},
	}

	c := client(t) // unauthenticated — enrollment needs no session
	var out struct {
		OK         bool `json:"ok"`
		GroupName  string
		DeviceName string
	}
	code := doJSON(t, c, "POST", ts.URL+"/api/enroll", `{"name":"Ava's iPhone","profileId":"p1"}`, &out)
	if code != 200 {
		t.Fatalf("enroll = %d", code)
	}
	if !out.OK || out.GroupName != "Emma" || out.DeviceName != "Ava's iPhone" {
		t.Errorf("unexpected enroll response: %+v", out)
	}
	profs := st.Snapshot().Profiles
	if len(profs) != 1 || len(profs[0].Devices) != 2 {
		t.Fatalf("store profiles = %+v", profs)
	}
	var got store.Device
	for _, dev := range profs[0].Devices {
		if dev.MAC == "aa:bb:cc:dd:ee:99" {
			got = dev
		}
	}
	if got.Name != "Ava's iPhone" {
		t.Errorf("device not persisted with the given name: %+v", got)
	}
}

func TestEnrollDefaultsNameToGatewayName(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	doSetup(t, ts)
	seedProfile(t, st)
	fake.clients = []unifi.NetClient{
		{Name: "iPhone 72:68", MACAddress: "AA:BB:CC:DD:EE:99", IPAddress: enrollTestIP, Type: "WIRELESS"},
	}

	c := client(t)
	var out struct{ DeviceName string }
	code := doJSON(t, c, "POST", ts.URL+"/api/enroll", `{"name":"","profileId":"p1"}`, &out)
	if code != 200 {
		t.Fatalf("enroll = %d", code)
	}
	if out.DeviceName != "iPhone 72:68" {
		t.Errorf("deviceName = %q, want the gateway's name", out.DeviceName)
	}
}

func TestEnrollMovesBetweenProfilesRetargetsBoth(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts) // authed client, needed for the rule-creation calls below
	err := st.Update(func(d *store.Data) error {
		d.Profiles = []store.Profile{
			{ID: "src", Name: "Old Group", Devices: []store.Device{
				{MAC: "aa:bb:cc:dd:ee:99", Name: "iPhone"},
				{MAC: "aa:bb:cc:dd:ee:01", Name: "Other device"}, // keeps src non-empty after the move
			}},
			{ID: "dst", Name: "New Group", Devices: []store.Device{
				{MAC: "aa:bb:cc:dd:ee:02", Name: "Existing device"},
			}},
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var srcRule, dstRule store.FamilyRule
	if code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"src","name":"No YouTube","what":{"type":"preset","presetId":"youtube"},"when":{"kind":"always"}}`, &srcRule); code != 200 {
		t.Fatalf("create src rule = %d", code)
	}
	if code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"dst","name":"No TikTok","what":{"type":"preset","presetId":"tiktok"},"when":{"kind":"always"}}`, &dstRule); code != 200 {
		t.Fatalf("create dst rule = %d", code)
	}

	fake.clients = []unifi.NetClient{
		{Name: "iPhone 72:68", MACAddress: "AA:BB:CC:DD:EE:99", IPAddress: enrollTestIP, Type: "WIRELESS"},
	}

	anon := client(t) // enrollment itself needs no session
	var out struct {
		OK        bool `json:"ok"`
		GroupName string
	}
	code := doJSON(t, anon, "POST", ts.URL+"/api/enroll", `{"name":"Ava's iPhone","profileId":"dst"}`, &out)
	if code != 200 {
		t.Fatalf("enroll = %d", code)
	}
	if out.GroupName != "New Group" {
		t.Errorf("groupName = %q", out.GroupName)
	}

	profs := st.Snapshot().Profiles
	var src, dst store.Profile
	for _, p := range profs {
		if p.ID == "src" {
			src = p
		}
		if p.ID == "dst" {
			dst = p
		}
	}
	for _, dev := range src.Devices {
		if dev.MAC == "aa:bb:cc:dd:ee:99" {
			t.Errorf("device still present in source profile: %+v", src.Devices)
		}
	}
	foundInDst := false
	for _, dev := range dst.Devices {
		if dev.MAC == "aa:bb:cc:dd:ee:99" {
			foundInDst = true
		}
	}
	if !foundInDst {
		t.Errorf("device not added to destination profile: %+v", dst.Devices)
	}

	gwSrc, ok := fake.ruleByDesc(srcRule.ID)
	if !ok {
		t.Fatal("source gateway rule missing")
	}
	for _, td := range gwSrc.TargetDevices {
		if td.ClientMAC == "aa:bb:cc:dd:ee:99" {
			t.Errorf("source gateway rule still targets the moved device: %+v", gwSrc.TargetDevices)
		}
	}
	gwDst, ok := fake.ruleByDesc(dstRule.ID)
	if !ok {
		t.Fatal("destination gateway rule missing")
	}
	gotInDst := false
	for _, td := range gwDst.TargetDevices {
		if td.ClientMAC == "aa:bb:cc:dd:ee:99" {
			gotInDst = true
		}
	}
	if !gotInDst {
		t.Errorf("destination gateway rule doesn't target the moved device: %+v", gwDst.TargetDevices)
	}
}

func TestEnrollUnknownProfileID400(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	doSetup(t, ts)
	seedProfile(t, st)
	fake.clients = []unifi.NetClient{
		{Name: "iPhone 72:68", MACAddress: "AA:BB:CC:DD:EE:99", IPAddress: enrollTestIP},
	}
	c := client(t)
	code := doJSON(t, c, "POST", ts.URL+"/api/enroll", `{"name":"x","profileId":"nope"}`, nil)
	if code != 400 {
		t.Errorf("unknown profileId = %d, want 400", code)
	}
}

func TestEnrollRejectsEveryone(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	doSetup(t, ts)
	seedProfile(t, st)
	fake.clients = []unifi.NetClient{
		{Name: "iPhone 72:68", MACAddress: "AA:BB:CC:DD:EE:99", IPAddress: enrollTestIP},
	}
	c := client(t)
	code := doJSON(t, c, "POST", ts.URL+"/api/enroll", `{"name":"x","profileId":"everyone"}`, nil)
	if code != 400 {
		t.Errorf("profileId=everyone = %d, want 400", code)
	}
}

func TestEnrollDeviceNotOnNetwork404(t *testing.T) {
	ts, _, st, _ := newTestServer(t)
	doSetup(t, ts)
	seedProfile(t, st)
	// No fake client matches the requester's IP.
	c := client(t)
	var out errBody
	code := doJSON(t, c, "POST", ts.URL+"/api/enroll", `{"name":"x","profileId":"p1"}`, &out)
	if code != 404 {
		t.Fatalf("device not found = %d, want 404", code)
	}
	if out.Error == "" {
		t.Error("expected a friendly message")
	}
}

func TestEnrollUnconfigured(t *testing.T) {
	ts, _, _, _ := newTestServer(t)
	c := client(t)
	code := doJSON(t, c, "POST", ts.URL+"/api/enroll", `{"name":"x","profileId":"p1"}`, nil)
	if code != 409 {
		t.Errorf("enroll unconfigured = %d, want 409", code)
	}
}

// TestEnrollGatewayFailurePersistsIntent mirrors
// TestProfileUpdatePersistsIntentOnPartialGatewayFailure: moving a device
// between two profiles that both have rules, with the destination's
// gateway update failing partway through the retarget loop. The store must
// still end up holding the new intent (device moved) even though the
// gateway only partially converged — see persistPartialEnroll.
func TestEnrollGatewayFailurePersistsIntent(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	err := st.Update(func(d *store.Data) error {
		d.Profiles = []store.Profile{
			{ID: "src", Name: "Old Group", Devices: []store.Device{
				{MAC: "aa:bb:cc:dd:ee:99", Name: "iPhone"},
				{MAC: "aa:bb:cc:dd:ee:01", Name: "Other device"}, // keeps src non-empty after the move
			}},
			{ID: "dst", Name: "New Group", Devices: []store.Device{{MAC: "aa:bb:cc:dd:ee:02", Name: "Existing"}}},
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var srcRule, dstRule store.FamilyRule
	doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"src","name":"No YouTube","what":{"type":"preset","presetId":"youtube"},"when":{"kind":"always"}}`, &srcRule)
	doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"dst","name":"No TikTok","what":{"type":"preset","presetId":"tiktok"},"when":{"kind":"always"}}`, &dstRule)
	gwDst, ok := fake.ruleByDesc(dstRule.ID)
	if !ok {
		t.Fatal("dst gateway rule missing")
	}

	fake.clients = []unifi.NetClient{
		{Name: "iPhone", MACAddress: "AA:BB:CC:DD:EE:99", IPAddress: enrollTestIP},
	}
	fake.mu.Lock()
	fake.failUpdateID = gwDst.ID // dst is processed second (source-first order)
	fake.mu.Unlock()

	anon := client(t)
	var errOut errBody
	code := doJSON(t, anon, "POST", ts.URL+"/api/enroll", `{"name":"Ava's iPhone","profileId":"dst"}`, &errOut)
	if code < 500 || code >= 600 {
		t.Fatalf("partial gateway failure = %d, want 5xx; body=%+v", code, errOut)
	}
	if errOut.Error == "" {
		t.Error("expected a friendly error message")
	}

	profs := st.Snapshot().Profiles
	var src, dst store.Profile
	for _, p := range profs {
		if p.ID == "src" {
			src = p
		}
		if p.ID == "dst" {
			dst = p
		}
	}
	if len(src.Devices) != 1 || src.Devices[0].MAC != "aa:bb:cc:dd:ee:01" {
		t.Errorf("source profile should have lost the moved device despite the gateway failure: %+v", src.Devices)
	}
	foundInDst := false
	for _, dev := range dst.Devices {
		if dev.MAC == "aa:bb:cc:dd:ee:99" {
			foundInDst = true
		}
	}
	if !foundInDst {
		t.Errorf("destination profile should already hold the new intent: %+v", dst.Devices)
	}

	// The source's rule (processed first) converged; the destination's
	// (processed second, and failing) did not.
	gwSrc, _ := fake.ruleByDesc(srcRule.ID)
	if len(gwSrc.TargetDevices) != 1 {
		t.Errorf("source gateway rule should have been retargeted to the one remaining device: %+v", gwSrc.TargetDevices)
	}
	gwDstAfter, _ := fake.ruleByDesc(dstRule.ID)
	if len(gwDstAfter.TargetDevices) != 1 {
		t.Errorf("destination gateway rule should be unchanged (update failed): %+v", gwDstAfter.TargetDevices)
	}

	// Clearing the failure and re-saving the destination profile (the
	// existing profile-update path) converges the rest, exactly like the
	// profile-edit partial-failure test.
	fake.mu.Lock()
	fake.failUpdateID = ""
	fake.mu.Unlock()
	code = doJSON(t, c, "PUT", ts.URL+"/api/profiles/dst",
		`{"name":"New Group","devices":[{"mac":"aa:bb:cc:dd:ee:02","name":"Existing"},{"mac":"aa:bb:cc:dd:ee:99","name":"Ava's iPhone"}]}`, nil)
	if code != 200 {
		t.Fatalf("retry profile update = %d, want 200", code)
	}
	gwDstFinal, _ := fake.ruleByDesc(dstRule.ID)
	if len(gwDstFinal.TargetDevices) != 2 {
		t.Errorf("destination gateway rule after retry = %+v, want 2 (converged)", gwDstFinal.TargetDevices)
	}
}

// TestEnrollPushesRenameToUnifi verifies that a successful enrollment pushes
// exactly one UniFi client rename, with the final (possibly
// gateway-name-defaulted) device name.
func TestEnrollPushesRenameToUnifi(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	doSetup(t, ts)
	seedProfile(t, st)
	fake.clients = []unifi.NetClient{
		{Name: "iPhone 72:68", MACAddress: "AA:BB:CC:DD:EE:99", IPAddress: enrollTestIP, Type: "WIRELESS"},
	}

	c := client(t)
	code := doJSON(t, c, "POST", ts.URL+"/api/enroll", `{"name":"Ava's iPhone","profileId":"p1"}`, nil)
	if code != 200 {
		t.Fatalf("enroll = %d", code)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.renames) != 1 {
		t.Fatalf("renames = %+v, want exactly 1", fake.renames)
	}
	if fake.renames[0].MAC != "aa:bb:cc:dd:ee:99" || fake.renames[0].Name != "Ava's iPhone" {
		t.Errorf("rename = %+v, want {aa:bb:cc:dd:ee:99 Ava's iPhone}", fake.renames[0])
	}
}

// TestEnrollRenameFailureIsBestEffort verifies that a UniFi rename failure
// during enrollment never fails the enrollment itself: the request still
// returns 200 and the store is updated, with the failure only logged.
func TestEnrollRenameFailureIsBestEffort(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	doSetup(t, ts)
	seedProfile(t, st)
	fake.clients = []unifi.NetClient{
		{Name: "iPhone 72:68", MACAddress: "AA:BB:CC:DD:EE:99", IPAddress: enrollTestIP, Type: "WIRELESS"},
	}
	fake.mu.Lock()
	fake.failRename = errors.New("simulated rename failure")
	fake.mu.Unlock()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	c := client(t)
	var out struct {
		OK         bool `json:"ok"`
		DeviceName string
	}
	code := doJSON(t, c, "POST", ts.URL+"/api/enroll", `{"name":"Ava's iPhone","profileId":"p1"}`, &out)
	if code != 200 {
		t.Fatalf("enroll = %d, want 200 despite rename failure", code)
	}
	if !out.OK || out.DeviceName != "Ava's iPhone" {
		t.Errorf("unexpected enroll response: %+v", out)
	}

	profs := st.Snapshot().Profiles
	found := false
	for _, dev := range profs[0].Devices {
		if dev.MAC == "aa:bb:cc:dd:ee:99" && dev.Name == "Ava's iPhone" {
			found = true
		}
	}
	if !found {
		t.Errorf("device not enrolled into the store despite the rename failure: %+v", profs)
	}
	if !strings.Contains(buf.String(), "unifi rename failed") || !strings.Contains(buf.String(), "simulated rename failure") {
		t.Errorf("expected the rename failure to be logged, got %q", buf.String())
	}
}

func TestSettingsIncludesEnrollURL(t *testing.T) {
	ts, _, _, srv := newTestServer(t)
	c := doSetup(t, ts)
	srv.SetAdvertisedPort(9191)

	var out struct{ EnrollUrl string }
	if code := getJSON(t, c, ts.URL+"/api/settings", &out); code != 200 {
		t.Fatalf("settings = %d", code)
	}
	if !strings.HasPrefix(out.EnrollUrl, "http://") || !strings.HasSuffix(out.EnrollUrl, ":9191/enroll") {
		t.Errorf("enrollUrl = %q, want http://<lan-ip>:9191/enroll", out.EnrollUrl)
	}
}

func TestEnrollZeroDevicesWithRules400(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts) // authed client, needed for rule creation
	// Setup: source profile with ONLY the enrolling device
	err := st.Update(func(d *store.Data) error {
		d.Profiles = []store.Profile{
			{ID: "src", Name: "Source Only", Devices: []store.Device{
				{MAC: "aa:bb:cc:dd:ee:99", Name: "iPhone"},
			}},
			{ID: "dst", Name: "Destination", Devices: []store.Device{
				{MAC: "aa:aa:aa:aa:aa:01", Name: "Other device"},
			}},
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a rule on the source profile so the zero-devices guard applies
	var srcRule store.FamilyRule
	if code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"src","name":"No YouTube","what":{"type":"preset","presetId":"youtube"},"when":{"kind":"always"}}`, &srcRule); code != 200 {
		t.Fatalf("create src rule = %d", code)
	}
	gwSrcBefore, ok := fake.ruleByDesc(srcRule.ID)
	if !ok {
		t.Fatal("source gateway rule missing after creation")
	}

	// The fake client matching enrollTestIP (127.0.0.1) has MAC AA:BB:CC:DD:EE:99
	fake.clients = []unifi.NetClient{
		{Name: "iPhone 72:68", MACAddress: "AA:BB:CC:DD:EE:99", IPAddress: enrollTestIP, Type: "WIRELESS"},
	}

	// Try to move the device from src (only device) to dst
	anon := client(t)
	var errOut errBody
	code := doJSON(t, anon, "POST", ts.URL+"/api/enroll", `{"name":"Ava's iPhone","profileId":"dst"}`, &errOut)
	if code != 400 {
		t.Fatalf("enroll zero-devices = %d, want 400", code)
	}
	if !strings.Contains(errOut.Error, "keep at least one device") && !strings.Contains(errOut.Error, "delete the rules first") {
		t.Errorf("error message doesn't mention the guard: %q", errOut.Error)
	}

	// Verify source profile still has the device
	profs := st.Snapshot().Profiles
	var srcAfter store.Profile
	for _, p := range profs {
		if p.ID == "src" {
			srcAfter = p
		}
	}
	var foundInSrc bool
	for _, dev := range srcAfter.Devices {
		if dev.MAC == "aa:bb:cc:dd:ee:99" {
			foundInSrc = true
		}
	}
	if !foundInSrc {
		t.Errorf("device not kept in source profile after failed move: %+v", srcAfter.Devices)
	}

	// Verify target profile unchanged
	var dstAfter store.Profile
	for _, p := range profs {
		if p.ID == "dst" {
			dstAfter = p
		}
	}
	if len(dstAfter.Devices) != 1 || dstAfter.Devices[0].MAC != "aa:aa:aa:aa:aa:01" {
		t.Errorf("target profile should be unchanged: %+v", dstAfter.Devices)
	}

	// Verify gateway rule unchanged
	gwSrcAfter, _ := fake.ruleByDesc(srcRule.ID)
	if len(gwSrcAfter.TargetDevices) != len(gwSrcBefore.TargetDevices) {
		t.Errorf("gateway rule TargetDevices changed: before %d, after %d", len(gwSrcBefore.TargetDevices), len(gwSrcAfter.TargetDevices))
	}
}

func TestEnrollIdempotentSameGroupRename(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	// Setup: device already in a profile under name "Old Name"
	err := st.Update(func(d *store.Data) error {
		d.Profiles = []store.Profile{
			{ID: "p1", Name: "Kids Group", Devices: []store.Device{
				{MAC: "aa:bb:cc:dd:ee:99", Name: "Old Name"},
				{MAC: "aa:cc:cc:cc:cc:01", Name: "Other device"},
			}},
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a rule on the profile so we can verify it still targets the device exactly once
	var rule store.FamilyRule
	if code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p1","name":"No YouTube","what":{"type":"preset","presetId":"youtube"},"when":{"kind":"always"}}`, &rule); code != 200 {
		t.Fatalf("create rule = %d", code)
	}

	// The fake client matching enrollTestIP has MAC AA:BB:CC:DD:EE:99
	fake.clients = []unifi.NetClient{
		{Name: "iPhone 72:68", MACAddress: "AA:BB:CC:DD:EE:99", IPAddress: enrollTestIP, Type: "WIRELESS"},
	}

	// POST enroll with the SAME profileId but different name "New Name"
	anon := client(t)
	var out struct {
		OK         bool `json:"ok"`
		GroupName  string
		DeviceName string
	}
	code := doJSON(t, anon, "POST", ts.URL+"/api/enroll", `{"name":"New Name","profileId":"p1"}`, &out)
	if code != 200 {
		t.Fatalf("enroll = %d, want 200", code)
	}
	if !out.OK || out.GroupName != "Kids Group" || out.DeviceName != "New Name" {
		t.Errorf("unexpected enroll response: %+v", out)
	}

	// Verify device in profile exactly once with the new name
	profs := st.Snapshot().Profiles
	if len(profs) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profs))
	}
	if len(profs[0].Devices) != 2 {
		t.Fatalf("expected 2 devices in profile, got %d: %+v", len(profs[0].Devices), profs[0].Devices)
	}
	var foundCount int
	var foundDev store.Device
	for _, dev := range profs[0].Devices {
		if dev.MAC == "aa:bb:cc:dd:ee:99" {
			foundCount++
			foundDev = dev
		}
	}
	if foundCount != 1 {
		t.Errorf("device should appear exactly once, found %d times", foundCount)
	}
	if foundDev.Name != "New Name" {
		t.Errorf("device name should be 'New Name', got %q", foundDev.Name)
	}

	// Verify gateway rule still targets the MAC exactly once
	gwRule, ok := fake.ruleByDesc(rule.ID)
	if !ok {
		t.Fatal("gateway rule missing")
	}
	var targetCount int
	for _, td := range gwRule.TargetDevices {
		if td.ClientMAC == "aa:bb:cc:dd:ee:99" {
			targetCount++
		}
	}
	if targetCount != 1 {
		t.Errorf("gateway rule should target MAC exactly once, found %d times", targetCount)
	}
}
