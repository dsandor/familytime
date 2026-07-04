package server

import (
	"net/http"
	"testing"
	"time"

	"familytime/internal/store"
	"familytime/internal/unifi"
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

	if code := doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"15m"}`, nil); code != 200 {
		t.Fatalf("pause = %d", code)
	}
	rules := st.Snapshot().Rules
	if len(rules) != 1 || !rules[0].Pause || rules[0].What.Type != store.WhatEverything {
		t.Fatalf("pause rule wrong: %+v", rules)
	}
	until, _ := time.Parse(time.RFC3339, rules[0].When.Until)
	if !until.Equal(now.Add(15 * time.Minute)) {
		t.Errorf("until = %v, want %v", until, now.Add(15*time.Minute))
	}

	// Pausing again with a different duration replaces (no stacking).
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"30m"}`, nil); code != 200 {
		t.Fatalf("pause = %d", code)
	}
	rules = st.Snapshot().Rules
	if len(rules) != 1 || !rules[0].Pause || rules[0].What.Type != store.WhatEverything {
		t.Fatalf("pause rule wrong: %+v", rules)
	}
	until, _ = time.Parse(time.RFC3339, rules[0].When.Until)
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

func TestPauseDelayedStart(t *testing.T) {
	ts, fake, st, srv := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	now := fixedNow(t, srv, "2026-07-03 20:00")

	// An immediate pause first — the delayed pause must replace it.
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"15m"}`, nil); code != 200 {
		t.Fatalf("pause = %d", code)
	}

	// "You have 30 more minutes": pause in 30m, then for 1h.
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"1h","delay":"30m"}`, nil); code != 200 {
		t.Fatalf("delayed pause = %d", code)
	}
	rs := st.Snapshot().Rules
	if len(rs) != 1 {
		t.Fatalf("delayed pause must replace, got %d rules", len(rs))
	}
	fr := rs[0]
	if fr.When.Start != "20:30" {
		t.Errorf("start = %q, want 20:30", fr.When.Start)
	}
	until, _ := time.Parse(time.RFC3339, fr.When.Until)
	if !until.Equal(now.Add(90 * time.Minute)) {
		t.Errorf("until = %v, want %v", until, now.Add(90*time.Minute))
	}
	gw, ok := fake.ruleByDesc(fr.ID)
	if !ok || gw.Schedule.Mode != unifi.ModeOneTime || gw.Schedule.TimeRangeStart != "20:30" || gw.Schedule.TimeRangeEnd != "21:30" {
		t.Errorf("gateway schedule = %+v", gw.Schedule)
	}

	// The rules list reports it pending: inactive, with startsAt.
	var out []struct {
		ID       string `json:"id"`
		Active   bool   `json:"active"`
		StartsAt string `json:"startsAt"`
	}
	if code := getJSON(t, c, ts.URL+"/api/rules", &out); code != 200 || len(out) != 1 {
		t.Fatalf("rules list = %d, %+v", code, out)
	}
	if out[0].Active {
		t.Error("delayed pause must not be active yet")
	}
	if want := now.Add(30 * time.Minute).Format(time.RFC3339); out[0].StartsAt != want {
		t.Errorf("startsAt = %q, want %q", out[0].StartsAt, want)
	}

	// A different delay value round-trips too.
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"15m","delay":"15m"}`, nil); code != 200 {
		t.Fatalf("15m delayed pause = %d", code)
	}
	fr = st.Snapshot().Rules[0]
	if fr.When.Start != "20:15" {
		t.Errorf("15m delay start = %q, want 20:15", fr.When.Start)
	}
}

func TestPauseDelayedMorningAnchorsToStart(t *testing.T) {
	ts, _, st, srv := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	fixedNow(t, srv, "2026-07-03 23:50")

	// The delay pushes the start past midnight; "morning" is the first
	// 07:00 after the START, not after now.
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"morning","delay":"30m"}`, nil); code != 200 {
		t.Fatalf("delayed pause = %d", code)
	}
	fr := st.Snapshot().Rules[0]
	if fr.When.Start != "00:20" {
		t.Errorf("start = %q, want 00:20", fr.When.Start)
	}
	until, _ := time.Parse(time.RFC3339, fr.When.Until)
	want := time.Date(2026, 7, 4, 7, 0, 0, 0, time.Local)
	if !until.Equal(want) {
		t.Errorf("until = %v, want %v", until, want)
	}

	// Discriminating case: at 06:50 + 30m the start (07:20) is past 07:00,
	// so morning must roll to the NEXT day — anchor-to-now would pick today.
	fixedNow(t, srv, "2026-07-04 06:50")
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"morning","delay":"30m"}`, nil); code != 200 {
		t.Fatalf("delayed pause = %d", code)
	}
	fr = st.Snapshot().Rules[0]
	if fr.When.Start != "07:20" {
		t.Errorf("start = %q, want 07:20", fr.When.Start)
	}
	until, _ = time.Parse(time.RFC3339, fr.When.Until)
	want = time.Date(2026, 7, 5, 7, 0, 0, 0, time.Local)
	if !until.Equal(want) {
		t.Errorf("until = %v, want %v (next-day morning, anchored to start)", until, want)
	}
}

func TestPauseDelayValidation(t *testing.T) {
	ts, _, st, srv := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	fixedNow(t, srv, "2026-07-03 20:00")

	if code := doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"1h","delay":"2h"}`, nil); code != 400 {
		t.Errorf("unknown delay = %d, want 400", code)
	}
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"indefinite","delay":"30m"}`, nil); code != 400 {
		t.Errorf("delayed indefinite = %d, want 400", code)
	}
	if len(st.Snapshot().Rules) != 0 {
		t.Error("failed requests must not persist rules")
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

func TestSettingsPINChangeRotatesSessionSecret(t *testing.T) {
	ts, _, _, _ := newTestServer(t)
	c := doSetup(t, ts) // client that will change the PIN

	other := client(t) // a second, pre-existing browser session
	if code := doJSON(t, other, "POST", ts.URL+"/api/login", `{"pin":"1234"}`, nil); code != 200 {
		t.Fatalf("second client login = %d", code)
	}
	if code := getJSON(t, other, ts.URL+"/api/status", nil); code != 200 {
		t.Fatalf("second client status before pin change = %d, want 200", code)
	}

	if code := doJSON(t, c, "PUT", ts.URL+"/api/settings/pin", `{"currentPin":"1234","newPin":"5678"}`, nil); code != 200 {
		t.Fatalf("pin change = %d, want 200", code)
	}

	if code := getJSON(t, other, ts.URL+"/api/status", nil); code != 401 {
		t.Errorf("other browser's old cookie after pin change = %d, want 401", code)
	}
	if code := getJSON(t, c, ts.URL+"/api/status", nil); code != 200 {
		t.Errorf("changing client should stay authed, got %d", code)
	}
}

func TestPauseGrantKeepsOriginalEnd(t *testing.T) {
	ts, fake, st, srv := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	fixedNow(t, srv, "2026-07-03 21:30")

	doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"morning"}`, nil)
	origUntil := st.Snapshot().Rules[0].When.Until

	// Ten minutes into the pause, grant 30 more minutes.
	fixedNow(t, srv, "2026-07-03 21:40")
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause/p1/delay", `{"delay":"30m"}`, nil); code != 200 {
		t.Fatalf("grant = %d", code)
	}
	rs := st.Snapshot().Rules
	if len(rs) != 1 {
		t.Fatalf("grant must replace, got %d rules", len(rs))
	}
	if rs[0].When.Start != "22:10" {
		t.Errorf("start = %q, want 22:10", rs[0].When.Start)
	}
	if rs[0].When.Until != origUntil {
		t.Errorf("until changed: %q -> %q", origUntil, rs[0].When.Until)
	}
	if len(fake.rules) != 1 {
		t.Errorf("gateway must hold exactly 1 rule, got %d", len(fake.rules))
	}
}

func TestPauseGrantOutlastingPauseRemovesIt(t *testing.T) {
	ts, fake, st, srv := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	fixedNow(t, srv, "2026-07-03 20:00")

	// Active pause ending 20:15; a 30m grant at 20:05 outlasts it.
	doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"15m"}`, nil)
	fixedNow(t, srv, "2026-07-03 20:05")
	var out map[string]bool
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause/p1/delay", `{"delay":"30m"}`, &out); code != 200 || !out["removed"] {
		t.Fatalf("grant = %d, out = %v, want removed:true", code, out)
	}
	if len(st.Snapshot().Rules) != 0 || len(fake.rules) != 0 {
		t.Error("outlasted pause must be fully removed")
	}

	// Same for a pending pause: start 20:20, end 20:35; +30m pushes past the end.
	fixedNow(t, srv, "2026-07-03 20:05")
	doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"15m","delay":"15m"}`, nil)
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause/p1/delay", `{"delay":"30m"}`, &out); code != 200 || !out["removed"] {
		t.Fatalf("pending grant = %d, out = %v, want removed:true", code, out)
	}
	if len(st.Snapshot().Rules) != 0 {
		t.Error("outlasted pending pause must be removed")
	}
}

func TestPauseGrantIndefiniteEndsAtMorning(t *testing.T) {
	ts, _, st, srv := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	fixedNow(t, srv, "2026-07-03 21:30")

	doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"indefinite"}`, nil)
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause/p1/delay", `{"delay":"30m"}`, nil); code != 200 {
		t.Fatalf("grant = %d", code)
	}
	fr := st.Snapshot().Rules[0]
	if fr.When.Kind != store.WhenOneTime || fr.When.Start != "22:00" {
		t.Errorf("when = %+v, want onetime starting 22:00", fr.When)
	}
	until, _ := time.Parse(time.RFC3339, fr.When.Until)
	want := time.Date(2026, 7, 4, 7, 0, 0, 0, time.Local)
	if !until.Equal(want) {
		t.Errorf("until = %v, want %v (first morning after restart)", until, want)
	}
}

func TestPauseGrantDegenerateWindowLeavesPauseIntact(t *testing.T) {
	ts, _, st, srv := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	fixedNow(t, srv, "2026-07-04 06:30")

	// Indefinite pause; a 30m grant lands exactly at 07:00, so the morning
	// re-engage window would start and end in the same clock minute.
	doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"indefinite"}`, nil)
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause/p1/delay", `{"delay":"30m"}`, nil); code != 400 {
		t.Fatalf("degenerate grant = %d, want 400", code)
	}
	rs := st.Snapshot().Rules
	if len(rs) != 1 || rs[0].When.Kind != store.WhenAlways {
		t.Errorf("failed grant must leave the pause untouched: %+v", rs)
	}
}

func TestPauseGrantPendingPushesPromisedStart(t *testing.T) {
	ts, _, st, srv := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	fixedNow(t, srv, "2026-07-03 20:00")

	// Scheduled pause: starts 20:30, ends 21:30.
	doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"1h","delay":"30m"}`, nil)
	origUntil := st.Snapshot().Rules[0].When.Until

	// "+30 min" counts from the promised start, not from now.
	fixedNow(t, srv, "2026-07-03 20:10")
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause/p1/delay", `{"delay":"30m"}`, nil); code != 200 {
		t.Fatalf("grant = %d", code)
	}
	fr := st.Snapshot().Rules[0]
	if fr.When.Start != "21:00" {
		t.Errorf("start = %q, want 21:00 (20:30 + 30m)", fr.When.Start)
	}
	if fr.When.Until != origUntil {
		t.Errorf("until changed: %q -> %q", origUntil, fr.When.Until)
	}
}

func TestPauseGrantValidation(t *testing.T) {
	ts, _, st, srv := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	fixedNow(t, srv, "2026-07-03 20:00")

	if code := doJSON(t, c, "POST", ts.URL+"/api/pause/p1/delay", `{"delay":"30m"}`, nil); code != 404 {
		t.Errorf("grant with nothing paused = %d, want 404", code)
	}
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause/nope/delay", `{"delay":"30m"}`, nil); code != 404 {
		t.Errorf("grant on unknown profile = %d, want 404", code)
	}
	doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"1h"}`, nil)
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause/p1/delay", `{"delay":"2h"}`, nil); code != 400 {
		t.Errorf("unknown delay = %d, want 400", code)
	}
	rs := st.Snapshot().Rules
	if len(rs) != 1 || rs[0].When.Start != "20:00" {
		t.Errorf("failed grant must not touch the pause: %+v", rs)
	}
}
