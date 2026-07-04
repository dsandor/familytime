# Delayed Quick Pause Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a parent schedule a Quick Pause to start after a time offset ("you have 30 more minutes") — a `Starting: Now / in 15 min / in 30 min / in 1 hr` selector on each Quick Pause card that shifts the pause window forward.

**Architecture:** No new scheduling machinery. The UniFi `ONE_TIME_ONLY` schedule already enforces a future `TimeRangeStart` natively, and `oneTimeUntil(start, until)` already takes the window start as its first argument — a delayed pause is the existing one-time rule with the window shifted. Two real changes: `handlePause` computes `start = now + delay`, and `rules.ActiveNow` learns to gate one-time rules on their start (today it reports future-dated windows as active). A new `startsAt` field on the rules-list response drives a "Scheduled" pending state in the UI.

**Tech Stack:** Go (stdlib only), Alpine.js SPA in `web/static` (no build step — embedded via `go:embed`), UniFi gateway REST (existing client).

**Spec:** `docs/superpowers/specs/2026-07-03-delayed-quick-pause-design.md`

## Global Constraints

- **NEVER commit or push to git** (user rule — overrides this plan template's usual commit steps; none are included, and you must not add any).
- All Go commands run from the repo root: `/Users/dsandor/Projects/bedtime`.
- No new dependencies — Go stdlib and the existing Alpine.js only. The web UI has no build step; `go build ./...` embeds `web/static`.
- API wire values are exact: `delay` accepts `"15m" | "30m" | "1h"` (or absent); `duration` values are unchanged (`15m|30m|1h|morning|indefinite`). The pending JSON field is `startsAt` (RFC3339, `omitempty`).
- UI copy is exact: chip row label `Starting`, chips `Now / in 15 min / in 30 min / in 1 hr`, badge `Scheduled`, pending line `Pauses at <time> · until <time>`, button `Cancel`.
- `ls` is aliased on this machine — use `/bin/ls` when you need it.

---

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

---

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

---

### Task 3: Frontend — `Starting` selector, Scheduled state, upcoming list, and final verification

**Files:**
- Modify: `web/static/js/app.js` — state (after line 18), choices (after `whenChoices`, line 35), mock data (`mockPreview` rules array, lines 68–80), home helpers (`groupPause` area, lines 339–342), `upcomingTodayList` (lines 363–378), `pause` (lines 380–383)
- Modify: `web/static/index.html` — Quick pause section (lines 151–189)
- Modify: `web/static/css/app.css` — after the `.pausebox` block (line 481)

**Interfaces:**
- Consumes: `POST /api/pause` body `{profileId, duration, delay?}` and rules-list `startsAt` from Task 2; existing Alpine helpers `pauseRules()`, `fmtTime(rfc)`, `whatLabel(w)`, `profileName(id)`, `api(method, path, body)`, `loadCore()`, `unpause(profileId)`.
- Produces: Alpine members `pauseDelay` (object keyed by profile id), `pauseDelayChoices`, `groupDelay(profileId)`, `setGroupDelay(profileId, id)`, `groupPending(profileId)` — used only within these files.

- [ ] **Step 1: Add state and delay choices in `app.js`**

After `renameDraft: '',` (line 18) add:

```js
    pauseDelay: {},
```

After the `whenChoices` array's closing `],` (line 35) add:

```js
    pauseDelayChoices: [
      { id: 'now', label: 'Now' },
      { id: '15m', label: 'in 15 min' },
      { id: '30m', label: 'in 30 min' },
      { id: '1h',  label: 'in 1 hr' },
    ],
```

- [ ] **Step 2: Add pending helpers and send the delay**

In `app.js`, directly below `groupPause` (after line 342) add:

```js
    // groupPending returns the scheduled-but-not-started pause rule for a profile, or null.
    groupPending(profileId) {
      return this.pauseRules().find(r => r.profileId === profileId && !r.active && r.startsAt) || null;
    },

    groupDelay(profileId) { return this.pauseDelay[profileId] || 'now'; },
    setGroupDelay(profileId, id) { this.pauseDelay[profileId] = id; },
```

Replace `pause` (lines 380–383):

```js
    async pause(profileId, duration) {
      const delay = this.groupDelay(profileId);
      const body = { profileId, duration };
      if (delay !== 'now') body.delay = delay;
      try {
        await this.api('POST', '/api/pause', body);
        this.pauseDelay[profileId] = 'now';
        await this.loadCore();
      }
      catch (e) { this.banner = e.message; }
    },
```

- [ ] **Step 3: Surface pending pauses in the Today strip**

Replace `upcomingTodayList` (lines 363–378) — the recurring mapping is unchanged; scheduled one-time pauses starting later today are merged in:

```js
    upcomingTodayList() {
      const now = new Date();
      const nowM = now.getHours() * 60 + now.getMinutes();
      const today = ['sun','mon','tue','wed','thu','fri','sat'][now.getDay()];
      const recurring = this.nonPauseRules()
        .filter(r => r.enabled && !r.active && r.when.kind === 'recurring' && r.when.start
          && (r.when.days || []).includes(today))
        .map(r => {
          const [h, m] = r.when.start.split(':').map(Number);
          return { id: r.id, name: r.name, groupName: this.profileName(r.profileId) || 'Everyone',
                   label: this.whatLabel(r.what), startM: h * 60 + m,
                   startLabel: new Date(0, 0, 0, h, m).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' }) };
        });
      const pending = this.rules
        .filter(r => r.enabled && !r.active && r.startsAt
          && new Date(r.startsAt).toDateString() === now.toDateString())
        .map(r => {
          const d = new Date(r.startsAt);
          return { id: r.id, name: r.name, groupName: this.profileName(r.profileId) || 'Everyone',
                   label: this.whatLabel(r.what), startM: d.getHours() * 60 + d.getMinutes(),
                   startLabel: d.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' }) };
        });
      return recurring.concat(pending)
        .filter(x => x.startM > nowM)
        .sort((a, b) => a.startM - b.startM);
    },
```

- [ ] **Step 4: Add a pending pause to the preview mock**

In `mockPreview`, append one entry to the `this.rules` array (after the last existing rule object, keeping all existing entries):

```js
        { id: 'r9', profileId: 'teens', name: 'Internet pause',
          what: { type: 'everything' },
          when: { kind: 'onetime', start: hhmm(now + 30 * 60e3), until: new Date(now + 90 * 60e3).toISOString() },
          enabled: true, pause: true, active: false,
          startsAt: new Date(now + 30 * 60e3).toISOString(),
          until: new Date(now + 90 * 60e3).toISOString() },
```

- [ ] **Step 5: Rework the Quick pause card in `index.html`**

Replace the section body (lines 151–189) with:

```html
      <section class="section">
        <div class="section-head">
          <p class="eyebrow">Quick pause</p>
          <p class="sub">Give a group a break — right away, or in a little while.</p>
        </div>
        <template x-for="p in profiles" :key="p.id">
          <div class="card pause-card">
            <div class="pause-card-head">
              <span class="avatar" :style="'--accent:'+(p.color||'#22d3ee')" x-text="p.emoji || '🧒'"></span>
              <div class="grow">
                <h3 x-text="p.name"></h3>
                <p class="sub" x-text="(p.devices?p.devices.length:0) + ((p.devices&&p.devices.length===1)?' device':' devices')"></p>
              </div>
              <span class="badge badge-active" x-show="groupPause(p.id)">Active</span>
              <span class="badge badge-scheduled" x-show="groupPending(p.id)">Scheduled</span>
            </div>

            <template x-if="!groupPause(p.id) && !groupPending(p.id)">
              <div>
                <div class="pause-start">
                  <span class="label">Starting</span>
                  <template x-for="c in pauseDelayChoices" :key="c.id">
                    <button class="chip" :class="{ on: groupDelay(p.id) === c.id }"
                            @click="setGroupDelay(p.id, c.id)" x-text="c.label"></button>
                  </template>
                </div>
                <div class="pause-actions">
                  <button class="chip" @click="pause(p.id,'15m')">15m</button>
                  <button class="chip" @click="pause(p.id,'30m')">30m</button>
                  <button class="chip" @click="pause(p.id,'1h')">1h</button>
                  <button class="chip secondary" @click="pause(p.id,'morning')">Until morning</button>
                  <button class="chip secondary" @click="pause(p.id,'indefinite')"
                          :disabled="groupDelay(p.id) !== 'now'"
                          :title="groupDelay(p.id) !== 'now' ? 'Scheduled pauses need an end time — pick a timed duration.' : ''">Until I resume</button>
                </div>
              </div>
            </template>
            <template x-if="groupPause(p.id)">
              <div class="pausebox">
                <p class="sub">Paused<span x-show="groupPause(p.id).until" x-text="' until ' + fmtTime(groupPause(p.id).until)"></span></p>
                <button class="primary" @click="unpause(p.id)">Resume</button>
              </div>
            </template>
            <template x-if="groupPending(p.id)">
              <div class="pausebox scheduled">
                <p class="sub" x-text="'Pauses at ' + fmtTime(groupPending(p.id).startsAt)
                  + (groupPending(p.id).until ? ' · until ' + fmtTime(groupPending(p.id).until) : '')"></p>
                <button class="primary" @click="unpause(p.id)">Cancel</button>
              </div>
            </template>

            <button class="ghost" @click="goRules(p.id)">Manage rules →</button>
          </div>
        </template>
        <p class="center-note sub" x-show="profiles.length===0">
          Add your first group in <b>Groups &amp; Devices</b> to start pausing internet access.
        </p>
      </section>
```

(The card head, active pausebox, Manage rules button, and empty-state note are byte-identical to today; the changes are the subtitle, the Scheduled badge, the wrapping `x-if` gaining `&& !groupPending(p.id)` plus the `pause-start` row, the disabled state on "Until I resume", and the new scheduled pausebox.)

- [ ] **Step 6: Style the new pieces in `app.css`**

Insert after the `.pausebox button` rule (line 481):

```css
.pause-start { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 16px; }
.pause-start .label { font-size: 11px; font-weight: 700; letter-spacing: .06em; text-transform: uppercase; color: var(--text-dim); margin-right: 2px; }
.pause-start .chip { padding: 6px 12px; font-size: 13px; border-radius: var(--radius-full); }
.pause-start .chip.on { border-color: var(--cyan); background: rgba(34, 211, 238, .12); color: var(--cyan-bright); box-shadow: var(--shadow-glow-cyan); }

.badge.badge-scheduled { background: rgba(251, 191, 36, .15); color: #fcd34d; }
.badge.badge-scheduled::before { content: ""; width: 6px; height: 6px; border-radius: 50%; background: #fbbf24; box-shadow: 0 0 6px rgba(251, 191, 36, .8); }

.pausebox.scheduled { background: rgba(251, 191, 36, .07); border-color: rgba(251, 191, 36, .22); }
```

- [ ] **Step 7: Build and run the full test suite**

Run: `gofmt -l cmd internal && go build ./... && go test ./...`
Expected: `gofmt` prints nothing; build succeeds (embeds the updated static files); all packages PASS, including `internal/e2e`.

- [ ] **Step 8: Verify the UI in the browser**

Run: `go run ./cmd/familytime -port 8080 -data /private/tmp/claude-501/-Users-dsandor-Projects-bedtime/18358d72-697b-4c57-9c0f-c8083c1d9c54/scratchpad/ft-preview.json`
Open `http://localhost:8080/?preview=home` (the preview mode is client-side mock data; no gateway or setup needed) and confirm:
- Each Quick Pause card shows the `Starting` row (`Now` selected by default) above the duration chips.
- Selecting `in 30 min` disables **Until I resume** (dimmed via the global `button:disabled` style).
- The Teens card (mock `r9`) shows the amber **Scheduled** badge and the scheduled pausebox — "Pauses at \<time\> · until \<time\>" with a **Cancel** button — instead of duration chips.
- The "Upcoming today" strip lists the pending pause with its start time.

Then stop the server (Ctrl-C). If a browser isn't available, use the chrome-devtools MCP tools to load the page and screenshot it.

---

## Self-Review Notes

- Spec coverage: API `delay` + validation (Task 2), `ActiveNow` gating + `OneTimeStart` (Task 1), `startsAt`/pending state (Task 2), card UI + Scheduled badge + Cancel + upcoming list + copy (Task 3), janitor untouched (spec: no change needed), overnight edge (Task 1 tests), replace-not-stack with delay (Task 2 test).
- No commit steps by design — the user's global rules forbid git commits; do not add them.
- Type consistency: `OneTimeStart(store.When) (time.Time, bool)` is what Task 2 calls; JSON field `startsAt` matches between Task 2 tests and Task 3 JS.
