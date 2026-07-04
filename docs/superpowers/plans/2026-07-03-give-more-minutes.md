# "Give 30 More Minutes" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One-tap "give 30 more minutes" on a paused (or scheduled-to-pause) group: the pause lifts now (or its start pushes out) and re-engages later, keeping its original end.

**Architecture:** New server endpoint `POST /api/pause/{profileId}/delay` computes everything from the group's existing pause rule (client sends no timestamps) and reuses the gateway-first replace flow from `handlePause`. Frontend adds one `grantMore()` helper and a secondary button in each pausebox. No `internal/rules` changes — `OneTimeStart` and the one-time translation already cover the windows.

**Tech Stack:** Go stdlib, Alpine.js SPA in `web/static` (no build step, go:embed).

**Spec:** `docs/superpowers/specs/2026-07-03-give-more-minutes-design.md`

## Global Constraints

- **NEVER commit or push to git** (user rule — this plan intentionally has no commit steps; do not add any). The project is not a git repository; do not run git commands.
- All Go commands run from the repo root: `/Users/dsandor/Projects/bedtime`.
- No new dependencies — Go stdlib and the existing Alpine.js only.
- Wire contract is exact: route `POST /api/pause/{profileId}/delay`, body `{"delay":"15m"|"30m"|"1h"}`; unknown delay → 400 "Unknown pause delay."; no pause rule → 404 "Nothing is paused."; unknown profile → 404 "No such profile."; collapse response `{"removed":true}`; otherwise the stored `FamilyRule` JSON.
- Collapse rule is exact: remove instead of reschedule when `!until.After(newStart.Add(time.Minute))`.
- UI copy is exact: active pausebox button `30 more min`; pending pausebox button `+30 min`. UI always sends `"delay":"30m"`.
- `ls` is aliased on this machine — use `/bin/ls` when you need it.

---

### Task 1: Server — `POST /api/pause/{profileId}/delay` (+ `nextMorning` extraction)

**Files:**
- Modify: `internal/server/rules_handlers.go` (`handlePause` "morning" case ~line 296; new handler + helper after `handleUnpause` ~line 346)
- Modify: `internal/server/handlers.go` (route registration, after line 29's `POST /api/pause`)
- Test: `internal/server/rules_handlers_test.go` (append at end)

**Interfaces:**
- Consumes: existing `oneTimeUntil(now, until time.Time) store.When`, `removePauseRules(ctx, profileID)`, `createFamilyRule(ctx, fr, p)`, `profileForID`, `readJSON`, `fail`/`failErr`, `writeJSON`, `rules.OneTimeStart(w store.When) (time.Time, bool)`; test helpers `newTestServer`, `doSetup`, `seedProfile`, `fixedNow`, `doJSON`.
- Produces: `POST /api/pause/{profileId}/delay` endpoint per the Global Constraints wire contract; `nextMorning(t time.Time) time.Time` helper (first 7:00 AM strictly after `t`), now also used by `handlePause`. Task 2's `grantMore()` calls the endpoint.

- [ ] **Step 1: Write the failing tests**

Append to `internal/server/rules_handlers_test.go` (existing helpers: `fixedNow` re-assigns the server clock and can be called mid-test; 2026-07-03 is a Friday):

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/server/ -run 'TestPauseGrant' -v`
Expected: FAIL — every request to `/api/pause/p1/delay` returns 404 (route not registered), so the 200-expecting tests fatal with `grant = 404`, and `TestPauseGrantValidation`'s unknown-delay case gets 404 instead of 400.

- [ ] **Step 3: Extract `nextMorning` and implement the handler**

In `internal/server/rules_handlers.go`, replace the `"morning"` case body inside `handlePause` (currently):

```go
	case "morning":
		morning := time.Date(start.Year(), start.Month(), start.Day(), 7, 0, 0, 0, start.Location())
		if !start.Before(morning) {
			morning = morning.AddDate(0, 0, 1)
		}
		when = oneTimeUntil(start, morning)
```

with:

```go
	case "morning":
		when = oneTimeUntil(start, nextMorning(start))
```

Then insert after `handleUnpause` (after its closing brace, before `type statusLine`):

```go
// nextMorning returns the first 7:00 AM strictly after t.
func nextMorning(t time.Time) time.Time {
	m := time.Date(t.Year(), t.Month(), t.Day(), 7, 0, 0, 0, t.Location())
	if !t.Before(m) {
		m = m.AddDate(0, 0, 1)
	}
	return m
}

// handlePauseDelay grants more internet time on an existing pause: the pause
// lifts (or its scheduled start pushes out) and re-engages later, keeping its
// original end. A grant that outlasts the pause removes it instead.
func (s *Server) handlePauseDelay(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Delay string `json:"delay"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "Bad request.")
		return
	}
	var delay time.Duration
	switch in.Delay {
	case "15m":
		delay = 15 * time.Minute
	case "30m":
		delay = 30 * time.Minute
	case "1h":
		delay = time.Hour
	default:
		fail(w, 400, "Unknown pause delay.")
		return
	}
	d := s.store.Snapshot()
	p, ok := profileForID(d, r.PathValue("profileId"))
	if !ok {
		fail(w, 404, "No such profile.")
		return
	}
	var pr *store.FamilyRule
	for i := range d.Rules {
		if d.Rules[i].ProfileID == p.ID && d.Rules[i].Pause {
			pr = &d.Rules[i]
			break
		}
	}
	if pr == nil {
		fail(w, 404, "Nothing is paused.")
		return
	}
	now := s.now()
	base := now
	var until time.Time
	switch pr.When.Kind {
	case store.WhenAlways:
		// "Until I resume" has no end; a scheduled restart needs one.
		until = nextMorning(base.Add(delay))
	case store.WhenOneTime:
		t, err := time.Parse(time.RFC3339, pr.When.Until)
		if err != nil {
			fail(w, 500, "This pause looks corrupted — try Resume instead.")
			return
		}
		until = t
		if start, ok := rules.OneTimeStart(pr.When); ok && now.Before(start) {
			base = start // pending: "+30 min" counts from the promised start
		}
	default:
		fail(w, 400, "This pause can't be extended.")
		return
	}
	newStart := base.Add(delay)
	if err := s.removePauseRules(r.Context(), p.ID); err != nil {
		failErr(w, err)
		return
	}
	// The grant outlasts the pause — lifting it is the whole grant.
	if !until.After(newStart.Add(time.Minute)) {
		writeJSON(w, 200, map[string]bool{"removed": true})
		return
	}
	fr := store.FamilyRule{
		ID: store.NewID(), ProfileID: p.ID, Name: "Internet pause",
		What: store.What{Type: store.WhatEverything}, When: oneTimeUntil(newStart, until),
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
```

In `internal/server/handlers.go`, after the line `s.mux.Handle("POST /api/pause", s.auth(s.handlePause))` add:

```go
	s.mux.Handle("POST /api/pause/{profileId}/delay", s.auth(s.handlePauseDelay))
```

- [ ] **Step 4: Run the package tests to verify they pass**

Run: `go test ./internal/server/ -v`
Expected: PASS — all new `TestPauseGrant*` tests plus every pre-existing test (`handlePause`'s "morning" behavior is unchanged by the `nextMorning` extraction; `TestPauseReplaceAndUnpause` and `TestPauseDelayedMorningAnchorsToStart` prove it).

---

### Task 2: Frontend — grant buttons in both pauseboxes, and full verification

**Files:**
- Modify: `web/static/js/app.js` (add `grantMore` after `unpause`, ~line 428)
- Modify: `web/static/index.html` (active pausebox ~lines 189–192; pending pausebox ~lines 195–199)
- Modify: `web/static/css/app.css` (after the `.pausebox.scheduled` rule, ~line 491)

**Interfaces:**
- Consumes: `POST /api/pause/{profileId}/delay` with body `{"delay":"30m"}` from Task 1; existing Alpine helpers `api`, `loadCore`, `banner`.
- Produces: Alpine method `grantMore(profileId)`; used only by index.html.

- [ ] **Step 1: Add `grantMore` to `app.js`**

Directly below the `unpause` method, add:

```js
    // grantMore lifts the current pause (or pushes a scheduled one) and
    // re-engages it 30 minutes later, keeping its original end.
    async grantMore(profileId) {
      try { await this.api('POST', '/api/pause/' + profileId + '/delay', { delay: '30m' }); await this.loadCore(); }
      catch (e) { this.banner = e.message; }
    },
```

- [ ] **Step 2: Add the buttons in `index.html`**

Replace the active pausebox block:

```html
            <template x-if="groupPause(p.id)">
              <div class="pausebox">
                <p class="sub">Paused<span x-show="groupPause(p.id).until" x-text="' until ' + fmtTime(groupPause(p.id).until)"></span></p>
                <button class="primary" @click="unpause(p.id)">Resume</button>
              </div>
            </template>
```

with:

```html
            <template x-if="groupPause(p.id)">
              <div class="pausebox">
                <p class="sub">Paused<span x-show="groupPause(p.id).until" x-text="' until ' + fmtTime(groupPause(p.id).until)"></span></p>
                <div class="pausebox-actions">
                  <button class="more" @click="grantMore(p.id)">30 more min</button>
                  <button class="primary" @click="unpause(p.id)">Resume</button>
                </div>
              </div>
            </template>
```

Replace the pending pausebox block:

```html
            <template x-if="groupPending(p.id)">
              <div class="pausebox scheduled">
                <p class="sub" x-text="'Pauses at ' + fmtTime(groupPending(p.id).startsAt)
                  + (groupPending(p.id).until ? ' · until ' + fmtTime(groupPending(p.id).until) : '')"></p>
                <button class="primary" @click="unpause(p.id)">Cancel</button>
              </div>
            </template>
```

with:

```html
            <template x-if="groupPending(p.id)">
              <div class="pausebox scheduled">
                <p class="sub" x-text="'Pauses at ' + fmtTime(groupPending(p.id).startsAt)
                  + (groupPending(p.id).until ? ' · until ' + fmtTime(groupPending(p.id).until) : '')"></p>
                <div class="pausebox-actions">
                  <button class="more" @click="grantMore(p.id)">+30 min</button>
                  <button class="primary" @click="unpause(p.id)">Cancel</button>
                </div>
              </div>
            </template>
```

- [ ] **Step 3: Style the secondary button in `app.css`**

Insert after the `.pausebox.scheduled` rule:

```css
.pausebox-actions { display: flex; align-items: center; gap: 8px; flex: none; }
.pausebox button.more { padding: 10px 14px; font-size: 13px; font-weight: 600; color: var(--text-dim); background: none; border: 1px solid var(--glass-border); border-radius: var(--radius-full); white-space: nowrap; }
.pausebox button.more:hover:not(:disabled) { color: var(--cyan-bright); background: var(--glass-fill); }
```

(The existing `.pausebox button { margin: 0; width: auto; ... }` rule already normalizes both buttons inside the box.)

- [ ] **Step 4: Build and run the full test suite**

Run: `gofmt -l cmd internal && go build ./... && go test ./...`
Expected: `gofmt` prints nothing; build succeeds; all packages PASS.

- [ ] **Step 5: Verify the UI in the browser**

Run: `go run ./cmd/familytime -port 8080 -data /private/tmp/claude-501/-Users-dsandor-Projects-bedtime/18358d72-697b-4c57-9c0f-c8083c1d9c54/scratchpad/ft-preview2.json`
Open `http://localhost:8080/?preview=home` (client-side mock data; API calls are rejected, so verify presence/layout, not clicks):
- The Teens card (active pause) shows **30 more min** beside **Resume**, right-aligned as one group, no wrapping at desktop width.
- The Kids card (scheduled pause) shows **+30 min** beside **Cancel**.
- Zero console errors.
Then stop the server. Use the chrome-devtools MCP tools if no visible browser is available.

---

## Self-Review Notes

- Spec coverage: endpoint + wire contract (Task 1), all five semantic rows of the spec table tested (active-timed, outlasting incl. pending collapse, indefinite→morning, pending push, no-pause/unknown-delay/unknown-profile validation), `nextMorning` extraction (Task 1), UI buttons + `grantMore` + CSS (Task 2), edge cases: repeated grants covered by pending-push semantics; legacy one-time without Start falls into the `WhenOneTime` case with `base = now` (OneTimeStart not-ok path).
- Validation ordering: delay parse and rule lookup happen before `removePauseRules`; the collapse branch runs after removal by design (removal IS the outcome).
- No commit steps by design — the user's global rules forbid git; do not add them.
- Type consistency: `grantMore(profileId)` in Task 2 matches the route and body defined in Task 1; `nextMorning(t time.Time) time.Time` used in both `handlePause` and `handlePauseDelay`.
