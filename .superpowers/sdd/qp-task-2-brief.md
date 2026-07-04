# Task 2 brief — Delayed Quick Pause plan (2026-07-03)

## Global Constraints

- **NEVER commit or push to git** (user rule — overrides this plan template's usual commit steps; none are included, and you must not add any).
- All Go commands run from the repo root: `/Users/dsandor/Projects/bedtime`.
- No new dependencies — Go stdlib and the existing Alpine.js only. The web UI has no build step; `go build ./...` embeds `web/static`.
- API wire values are exact: `delay` accepts `"15m" | "30m" | "1h"` (or absent); `duration` values are unchanged (`15m|30m|1h|morning|indefinite`). The pending JSON field is `startsAt` (RFC3339, `omitempty`).
- UI copy is exact: chip row label `Starting`, chips `Now / in 15 min / in 30 min / in 1 hr`, badge `Scheduled`, pending line `Pauses at <time> · until <time>`, button `Cancel`.
- `ls` is aliased on this machine — use `/bin/ls` when you need it.

### Task 2: `internal/server` — `delay` on `POST /api/pause` and `startsAt` in the rules list

**Files:**
- Modify: `internal/server/rules_handlers.go` — `handlePause` (lines 250–306), `ruleView` + `handleRulesList` (lines 202–224)
- Test: `internal/server/rules_handlers_test.go` (append after `TestPauseReplaceAndUnpause`, line 206)

**Interfaces:**
- Consumes: `rules.OneTimeStart(w store.When) (time.Time, bool)` from Task 1; existing `oneTimeUntil(now, until time.Time) store.When`, `removePauseRules`, `createFamilyRule`, test helpers `newTestServer(t)`, `doSetup`, `seedProfile`, `fixedNow`, `doJSON`, `getJSON`, `fake.ruleByDesc`.
- Produces: `POST /api/pause` accepts optional `"delay": "15m"|"30m"|"1h"`; unknown delay → 400, `delay` + `"indefinite"` → 400. `GET /api/rules` items gain `"startsAt"` (RFC3339, omitted unless a one-time rule hasn't started yet). Task 3's frontend relies on both.

- [ ] **Step 1: Write the failing tests**

Append to `internal/server/rules_handlers_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/server/ -run 'TestPauseDelayed|TestPauseDelayValidation' -v`
Expected: FAIL — `TestPauseDelayedStart` fails on `start = "20:00", want 20:30` (the unknown `delay` field is silently ignored by `readJSON` today) and on empty `startsAt`; `TestPauseDelayValidation` fails because both requests return 200.

- [ ] **Step 3: Implement the handler changes**

In `internal/server/rules_handlers.go`, replace the input struct and the schedule computation in `handlePause` (lines 251–285) with:

```go
	var in struct {
		ProfileID string `json:"profileId"`
		Duration  string `json:"duration"`
		Delay     string `json:"delay"`
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
	// "You have 30 more minutes": an optional delay shifts the whole window
	// forward — the gateway enforces the future start natively.
	start := now
	switch in.Delay {
	case "":
	case "15m":
		start = now.Add(15 * time.Minute)
	case "30m":
		start = now.Add(30 * time.Minute)
	case "1h":
		start = now.Add(time.Hour)
	default:
		fail(w, 400, "Unknown pause delay.")
		return
	}
	var when store.When
	switch in.Duration {
	case "15m":
		when = oneTimeUntil(start, start.Add(15*time.Minute))
	case "30m":
		when = oneTimeUntil(start, start.Add(30*time.Minute))
	case "1h":
		when = oneTimeUntil(start, start.Add(time.Hour))
	case "morning":
		morning := time.Date(start.Year(), start.Month(), start.Day(), 7, 0, 0, 0, start.Location())
		if !start.Before(morning) {
			morning = morning.AddDate(0, 0, 1)
		}
		when = oneTimeUntil(start, morning)
	case "indefinite":
		if in.Delay != "" {
			fail(w, 400, `"Until I resume" starts right away — pick a timed duration to schedule ahead.`)
			return
		}
		when = store.When{Kind: store.WhenAlways}
	default:
		fail(w, 400, "Unknown pause duration.")
		return
	}
```

Everything from the `// Replace any existing pause` comment down is unchanged.

Then extend the rules list. Replace `ruleView` (lines 202–206):

```go
type ruleView struct {
	store.FamilyRule
	Active   bool   `json:"active"`
	Until    string `json:"until,omitempty"`
	StartsAt string `json:"startsAt,omitempty"` // one-time rule scheduled but not yet started
}
```

And in `handleRulesList`, extend the `if fr.Enabled` block (lines 214–220):

```go
		if fr.Enabled {
			active, until, hasUntil := rules.ActiveNow(fr.When, now)
			v.Active = active
			if hasUntil {
				v.Until = until.Format(time.RFC3339)
			}
			if start, ok := rules.OneTimeStart(fr.When); ok && now.Before(start) {
				v.StartsAt = start.Format(time.RFC3339)
			}
		}
```

`handleStatus` needs no edit — it inherits the corrected `Active` semantics through `rules.ActiveNow`.

- [ ] **Step 4: Run the package tests to verify they pass**

Run: `go test ./internal/server/ -v`
Expected: PASS — including the pre-existing `TestPauseReplaceAndUnpause` and `TestStatusReportsActiveWindows` (immediate pauses store `Start = now`'s clock minute, which is never in the future, so they stay active-on-create).
