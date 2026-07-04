# Task 1 brief — Delayed Quick Pause plan (2026-07-03)

## Global Constraints

- **NEVER commit or push to git** (user rule — overrides this plan template's usual commit steps; none are included, and you must not add any).
- All Go commands run from the repo root: `/Users/dsandor/Projects/bedtime`.
- No new dependencies — Go stdlib and the existing Alpine.js only. The web UI has no build step; `go build ./...` embeds `web/static`.
- API wire values are exact: `delay` accepts `"15m" | "30m" | "1h"` (or absent); `duration` values are unchanged (`15m|30m|1h|morning|indefinite`). The pending JSON field is `startsAt` (RFC3339, `omitempty`).
- UI copy is exact: chip row label `Starting`, chips `Now / in 15 min / in 30 min / in 1 hr`, badge `Scheduled`, pending line `Pauses at <time> · until <time>`, button `Cancel`.
- `ls` is aliased on this machine — use `/bin/ls` when you need it.

### Task 1: `internal/rules` — `OneTimeStart` helper and start-gated `ActiveNow`

`ActiveNow` (`internal/rules/translate.go:228`) currently treats a one-time rule as active from the moment it exists (`now.Before(until)`), ignoring `When.Start`. A future-dated pause would render as "Paused" immediately. Add a `OneTimeStart` helper that derives the window's opening moment, and gate `ActiveNow` on it.

**Files:**
- Modify: `internal/rules/translate.go` (one-time case at lines 232–237; new function after `ActiveNow`)
- Test: `internal/rules/translate_test.go` (append after `TestActiveNow`, line 308)

**Interfaces:**
- Consumes: existing `parseHM(s string) (int, error)` (`translate.go:208`), `store.When` fields `Kind`, `Start` ("15:04" clock string), `Until` (RFC3339).
- Produces: `OneTimeStart(w store.When) (time.Time, bool)` — the moment a one-time window opens; `ok=false` when the rule has no parseable start (legacy rules stored only `Until`). `ActiveNow` keeps its signature `(w store.When, now time.Time) (bool, time.Time, bool)` but now returns `active=false` (with `until` and `hasUntil=true` unchanged) before the window opens. Task 2 calls `OneTimeStart` from the server package.

- [ ] **Step 1: Write the failing tests**

Append to `internal/rules/translate_test.go` (existing helpers: `mustParse(t, "2006-01-02 15:04")` parses in `time.Local`; 2026-07-03 is a Friday):

```go
func TestOneTimeStart(t *testing.T) {
	// Overnight window 23:45 → 00:45: start anchors to the day BEFORE Until.
	w := store.When{Kind: store.WhenOneTime, Start: "23:45",
		Until: mustParse(t, "2026-07-04 00:45").Format(time.RFC3339)}
	start, ok := OneTimeStart(w)
	if !ok || !start.Equal(mustParse(t, "2026-07-03 23:45")) {
		t.Errorf("overnight start = %v ok=%v, want 2026-07-03 23:45", start, ok)
	}

	// Same-day window 20:30 → 21:30: start anchors to Until's own day.
	w = store.When{Kind: store.WhenOneTime, Start: "20:30",
		Until: mustParse(t, "2026-07-03 21:30").Format(time.RFC3339)}
	start, ok = OneTimeStart(w)
	if !ok || !start.Equal(mustParse(t, "2026-07-03 20:30")) {
		t.Errorf("same-day start = %v ok=%v, want 2026-07-03 20:30", start, ok)
	}

	// Legacy rule without a start clock → no start.
	if _, ok := OneTimeStart(store.When{Kind: store.WhenOneTime, Until: w.Until}); ok {
		t.Error("missing start clock must return ok=false")
	}
	// Non-one-time kinds have no one-time start.
	if _, ok := OneTimeStart(store.When{Kind: store.WhenAlways}); ok {
		t.Error("non-onetime kind must return ok=false")
	}
}

func TestActiveNowOneTimeStartGating(t *testing.T) {
	// Scheduled window 22:00 → 23:00.
	ot := store.When{Kind: store.WhenOneTime, Start: "22:00",
		Until: mustParse(t, "2026-07-03 23:00").Format(time.RFC3339)}

	// Before the start: pending — inactive, but until is still reported.
	active, until, hasUntil := ActiveNow(ot, mustParse(t, "2026-07-03 21:30"))
	if active || !hasUntil || !until.Equal(mustParse(t, "2026-07-03 23:00")) {
		t.Errorf("pending at 21:30: active=%v until=%v hasUntil=%v", active, until, hasUntil)
	}
	if active, _, _ := ActiveNow(ot, mustParse(t, "2026-07-03 22:30")); !active {
		t.Error("must be active at 22:30 (inside the window)")
	}
	if active, _, _ := ActiveNow(ot, mustParse(t, "2026-07-03 23:01")); active {
		t.Error("must be inactive at 23:01 (window over)")
	}

	// Overnight window 23:45 → 00:45 (crosses midnight).
	overnight := store.When{Kind: store.WhenOneTime, Start: "23:45",
		Until: mustParse(t, "2026-07-04 00:45").Format(time.RFC3339)}
	if active, _, _ := ActiveNow(overnight, mustParse(t, "2026-07-03 23:00")); active {
		t.Error("overnight window must be pending at 23:00")
	}
	if active, _, _ := ActiveNow(overnight, mustParse(t, "2026-07-04 00:10")); !active {
		t.Error("overnight window must be active at 00:10")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/rules/ -run 'TestOneTimeStart|TestActiveNowOneTimeStartGating' -v`
Expected: FAIL — `undefined: OneTimeStart` (compile error is the failure mode for the first test; after Step 3's helper exists but before the `ActiveNow` edit, the gating test fails on "pending at 21:30").

- [ ] **Step 3: Implement `OneTimeStart` and gate `ActiveNow`**

In `internal/rules/translate.go`, insert after the `ActiveNow` function (after line 273's closing brace):

```go
// OneTimeStart derives the moment a one-time window opens: the stored start
// clock anchored to Until's date, minus a day when the window crosses
// midnight — the same anchoring translateSchedule uses. ok=false when the
// rule has no parseable start (legacy rules stored only Until).
func OneTimeStart(w store.When) (time.Time, bool) {
	if w.Kind != store.WhenOneTime || w.Start == "" {
		return time.Time{}, false
	}
	until, err := time.Parse(time.RFC3339, w.Until)
	if err != nil {
		return time.Time{}, false
	}
	startM, err := parseHM(w.Start)
	if err != nil {
		return time.Time{}, false
	}
	endM := until.Hour()*60 + until.Minute()
	day := until
	if endM < startM {
		day = until.AddDate(0, 0, -1)
	}
	return time.Date(day.Year(), day.Month(), day.Day(), startM/60, startM%60, 0, 0, until.Location()), true
}
```

Replace the one-time case of `ActiveNow` (lines 232–237):

```go
	case store.WhenOneTime:
		until, err := time.Parse(time.RFC3339, w.Until)
		if err != nil {
			return false, time.Time{}, false
		}
		if start, ok := OneTimeStart(w); ok && now.Before(start) {
			return false, until, true // scheduled, not yet started
		}
		return now.Before(until), until, true
```

- [ ] **Step 4: Run the package tests to verify they pass**

Run: `go test ./internal/rules/ -v`
Expected: PASS — including the pre-existing `TestActiveNow`, whose one-time case has no `Start` and must keep working via the `ok=false` fallback.
