# Task 3 Report — Delayed Quick Pause: Frontend (2026-07-03)

## Summary

Implemented Task 3 (final task) of the Delayed Quick Pause feature: the `Starting` delay selector, the Scheduled pending state, upcoming-list support for pending pauses, the preview mock rule, and the corresponding HTML/CSS. All 8 steps from the brief completed exactly as specified.

## What Was Implemented

### Step 1 — State and delay choices (`web/static/js/app.js`)
- Added `pauseDelay: {},` after `renameDraft: '',` (new per-profile delay selection map).
- Added `pauseDelayChoices` array (`now` / `15m` / `30m` / `1h` with labels `Now`, `in 15 min`, `in 30 min`, `in 1 hr`) after `whenChoices`.

### Step 2 — Pending helpers and delay in POST body
- Added `groupPending(profileId)` directly below `groupPause` — returns the pause rule for a profile that is `!active` and has `startsAt`, or `null`.
- Added `groupDelay(profileId)` (defaults to `'now'`) and `setGroupDelay(profileId, id)`.
- Replaced `pause(profileId, duration)`: now reads the selected delay, includes `delay` in the POST body only when it isn't `'now'`, resets `pauseDelay[profileId]` back to `'now'` after a successful call, then reloads core data.

### Step 3 — Upcoming-today strip merges pending pauses
- Replaced `upcomingTodayList()`: recurring-rule mapping is unchanged; added a second `pending` list sourced from `this.rules` (any rule, not just non-pause) filtered on `enabled && !active && startsAt` falling on today's date, mapped to the same shape as the recurring entries. Both lists are concatenated, then filtered to future times and sorted by start minute — identical to the brief's snippet.

### Step 4 — Preview mock pending rule
- Appended `r9` (Teens, "Internet pause", one-time, `pause: true`, `active: false`, `startsAt` = now+30min, `until` = now+90min) to the `mockPreview` `this.rules` array, after `r4` and before the closing `];`. All prior entries (`r1`–`r4`) untouched.

### Step 5 — Quick pause card rework (`web/static/index.html`, lines 151–189 region)
- Subtitle changed to "Give a group a break — right away, or in a little while."
- Added `badge-scheduled` "Scheduled" badge shown via `x-show="groupPending(p.id)"`.
- Wrapped the "no active pause" branch's condition to `!groupPause(p.id) && !groupPending(p.id)`, and added the `pause-start` row (`Starting` label + `pauseDelayChoices` chip loop bound to `groupDelay`/`setGroupDelay`) above the existing duration-chip row.
- Added `:disabled` / `:title` bindings on "Until I resume" so it disables whenever a delay other than `now` is selected.
- Added a new `x-if="groupPending(p.id)"` block rendering a `pausebox scheduled` with the exact text `Pauses at <time> · until <time>` and a `Cancel` button (calls the existing `unpause(p.id)`).
- Card head, the active pausebox, "Manage rules →" button, and the empty-state note are byte-identical to the prior version — confirmed by diffing only the intended lines.

### Step 6 — CSS (`web/static/css/app.css`, after `.pausebox button`)
- Added `.pause-start`, `.pause-start .label`, `.pause-start .chip`, `.pause-start .chip.on` rules.
- Added `.badge.badge-scheduled` (amber) and its `::before` dot.
- Added `.pausebox.scheduled` (amber-tinted background/border).
- Verified all referenced CSS custom properties already exist (`--cyan`, `--cyan-bright`, `--shadow-glow-cyan`, `--text-dim`).

## Verification Results

### Step 7 — gofmt / build / test
```
$ gofmt -l cmd internal && go build ./... && go test ./...
ok      familytime/cmd/familytime       (cached)
ok      familytime/internal/e2e         (cached)
ok      familytime/internal/rules       0.222s
ok      familytime/internal/server      3.422s
ok      familytime/internal/store       (cached)
ok      familytime/internal/unifi       (cached)
?       familytime/web  [no test files]
```
`gofmt -l` printed nothing (no formatting issues), build succeeded, all packages passed including `internal/e2e`.

### Step 8 — Browser verification (chrome-devtools MCP)

Found a stale `familytime` process already bound to port 8080 (leftover from an earlier task's verification run); killed it before starting a fresh server built from the current source, per the brief's exact command:
```
go run ./cmd/familytime -port 8080 -data /private/tmp/claude-501/-Users-dsandor-Projects-bedtime/18358d72-697b-4c57-9c0f-c8083c1d9c54/scratchpad/ft-preview.json
```
Loaded `http://localhost:8080/?preview=home` via `mcp__chrome-devtools__new_page`.

Checkpoints:
- **Starting row present, "Now" implicitly selected (no delay set) on load.** Confirmed via a11y snapshot: Kids card shows `STARTING` label with buttons `Now / in 15 min / in 30 min / in 1 hr` above the duration chips (`15m / 30m / 1h / Until morning / Until I resume`).
- **Selecting "in 30 min" disables "Until I resume".** Clicked the "in 30 min" chip (uid 1_27) on the Kids card. Post-click snapshot shows the chip focused/selected and the "Until I resume" button now `disableable disabled` with `description="Scheduled pauses need an end time — pick a timed duration."`. Screenshot confirms the "in 30 min" chip glowing cyan (the `.on` style) and "Until I resume" visibly dimmed (global `button:disabled` opacity).
- **Teens card shows Scheduled badge + scheduled pausebox.** The Teens card (mock rule `r9`) simultaneously shows both `Active` (green, from `r4`) and `Scheduled` (amber) badges, an active pausebox ("Paused until 10:12 PM" / Resume), and a second amber-tinted pausebox reading exactly "Pauses at 10:22 PM · until 11:22 PM" with a "Cancel" button. Confirmed both via a11y snapshot and screenshot (amber border/background rendered correctly).
- **Upcoming-today strip lists the pending pause.** The "Upcoming today" ledger lists three entries in start-time order: "Teens: social media curfew" (starts 10:00 PM), "Internet pause — Teens · blocks All internet" (starts 10:22 PM, the pending `r9` pause), and "No Roblox during homework" (starts 10:52 PM). Confirmed by screenshot.
- **Console error-free.** `mcp__chrome-devtools__list_console_messages` returned "no console messages found" — zero errors/warnings throughout the session (initial load + interaction).

Server was stopped after verification (killed the `go run` child process; port 8080 confirmed free afterward).

## Files Changed

- `/Users/dsandor/Projects/bedtime/web/static/js/app.js` — `pauseDelay` state, `pauseDelayChoices`, `groupPending`/`groupDelay`/`setGroupDelay`, rewritten `pause()`, rewritten `upcomingTodayList()`, added mock rule `r9`.
- `/Users/dsandor/Projects/bedtime/web/static/index.html` — Quick pause section reworked (subtitle, Scheduled badge, Starting row, disabled "Until I resume", new scheduled pausebox).
- `/Users/dsandor/Projects/bedtime/web/static/css/app.css` — new `.pause-start*`, `.badge.badge-scheduled*`, `.pausebox.scheduled` rules inserted after `.pausebox button`.

No other files were touched. Nothing was committed (per project rules — no git repo present, no git commands run).

## Self-Review

- **Completeness:** All 8 steps applied exactly as specified in the brief, including anchor points (verified each anchor line matched the brief's line numbers before editing).
- **UI copy byte-exact:** `Starting` label, chip labels `Now / in 15 min / in 30 min / in 1 hr`, badge text `Scheduled`, pending line `Pauses at <time> · until <time>`, button text `Cancel` — all verified against rendered DOM via the a11y snapshot, matching the Global Constraints section verbatim.
- **Fidelity:** Card head, active pausebox, "Manage rules →" button, and empty-state note left untouched — only the subtitle, badge row, the `x-if` condition (added `&& !groupPending(p.id)`), the new `pause-start` row, the "Until I resume" disabled binding, and the new scheduled pausebox were changed, matching the brief's own fidelity note.
- **Discipline (YAGNI):** No functionality added beyond the brief — no new dependencies, no unrelated refactors, no extra state or helpers.
- **Verification:** `gofmt -l` output empty, `go build ./...` and `go test ./...` both clean (all packages including `internal/e2e` PASS). Browser verification performed live via chrome-devtools MCP tools with a11y snapshots and screenshots for every checkpoint; console confirmed error-free both at initial load and after the interaction.

### Issues / Concerns

None. One minor operational note: a stale `familytime` server process from a prior task's verification was already occupying port 8080; it was identified, confirmed as an orphaned process (not part of this session's work), stopped, and a fresh server built from the current (Task 3) source was started in its place for verification. This is disclosed for transparency but did not affect the correctness of the verification.

## Final-review polish fixes

Applied two small polish fixes from code review:

### Fix 1: Strengthen delayed-pause test coverage

**File:** `/Users/dsandor/Projects/bedtime/internal/server/rules_handlers_test.go`

- **`TestPauseDelayedMorningAnchorsToStart`:** Added a discriminating test case at clock time 06:50 with 30m delay. This case verifies that when the delayed start (07:20) falls past 07:00, the morning duration anchor correctly rolls to the *next* day (07:00 on 2026-07-05). The original case at 23:50 + 30m couldn't discriminate between anchoring to delayed start vs. `now` because both happened to produce the same result at that clock time.

- **`TestPauseDelayedStart`:** Added a 15m-delay round-trip coverage line after the existing assertions. Verifies that a different delay value (15m, not just 30m) correctly produces the expected start time (20:15 from base 20:00).

### Fix 2: Preview mock shows an impossible state

**File:** `/Users/dsandor/Projects/bedtime/web/static/js/app.js`

- **`r9` profileId:** Changed from `'teens'` to `'kids'`. With real backend data, a profile can have at most one pause rule; the mock previously showed both an active pause (`r4`) and a pending pause (`r9`) for the same `'teens'` profile, which is impossible. Moved the pending pause (`r9`) to the `'kids'` profile, which has no existing pause rules, avoiding the impossible state. The `'kids'` profile was verified to have no active pauses (rules `r1` and `r2` are not pauses; `pause: false`).

- **Comment added:** Inserted a one-line comment directly above `r9`: `// Pending (scheduled) pause — with real data a profile has at most one pause rule.`

### Verification

```
$ go test ./internal/server/ -run 'TestPauseDelayedStart|TestPauseDelayedMorningAnchorsToStart' -v
=== RUN   TestPauseDelayedStart
--- PASS: TestPauseDelayedStart (0.07s)
=== RUN   TestPauseDelayedMorningAnchorsToStart
--- PASS: TestPauseDelayedMorningAnchorsToStart (0.05s)
PASS
ok  	familytime/internal/server	0.443s
```

```
$ gofmt -l cmd internal
(no output — all code properly formatted)
```

```
$ go build ./...
(build succeeded with no errors)
```
