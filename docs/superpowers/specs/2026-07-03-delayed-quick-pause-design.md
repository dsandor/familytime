# Delayed Quick Pause — Design

**Date:** 2026-07-03
**Status:** Approved

## Purpose

Let a parent schedule a Quick Pause to start after a time offset instead of immediately — the "you have 30 more minutes" flow. The parent picks a delay (`in 15 min`, `in 30 min`, `in 1 hr`) and then taps a duration exactly as today; the pause engages at the scheduled time and ends per the chosen duration.

## Approach

The delay is a **modifier on the existing duration buttons**, not a separate feature. No new scheduling machinery: the UniFi `ONE_TIME_ONLY` schedule already carries a start time (`TimeRangeStart`) that the gateway enforces natively, and `translateSchedule` already handles arbitrary start/end including windows that cross midnight. A delayed pause is the same one-time rule with the window shifted forward.

Rejected alternatives:

- **Dedicated warning buttons** ("Pause in 30 min until morning") — hard-codes delay/duration pairings and bloats the card.
- **Server-side timer** that creates the rule at start time — state dies on app restart; the gateway already schedules natively.

## API change

`POST /api/pause` gains one optional field:

```json
{ "profileId": "…", "duration": "15m|30m|1h|morning|indefinite", "delay": "15m|30m|1h" }
```

- `delay` absent or empty → today's behavior, unchanged.
- With `delay`, `handlePause` computes `start = now + delay` and builds the window from `start`:
  - `15m` / `30m` / `1h` → `oneTimeUntil(start, start.Add(d))`
  - `morning` → first 7:00 AM strictly after `start` (same rule as today, anchored to `start` instead of `now`)
  - `indefinite` → **400**. "Until I resume" maps to `WhenAlways`, which has no schedule; the gateway one-time mode requires an end time, so a delayed indefinite pause is not expressible. The UI disables the chip when a delay is selected.
- Unknown `delay` value → 400.
- Replace-not-stack behavior is unchanged: scheduling a delayed pause first removes any existing pause rule for the profile (`removePauseRules`). Consequence, by design: if the group is currently paused, scheduling "pause in 30 min" lifts the current pause now and re-engages at the scheduled time — exactly the "30 more minutes" promise.

`oneTimeUntil(start, until)` needs no change — it already takes the window start as its first argument (`internal/server/rules_handlers.go:308`).

## Bug fix: `ActiveNow` must gate on the start time

`rules.ActiveNow` (`internal/rules/translate.go:232-237`) currently returns `active = now.Before(until)` for one-time rules, ignoring `When.Start` — so a future-dated pause would render as "Paused" immediately.

Fix: derive the full start timestamp from the stored clock string and the `Until` timestamp — start clock anchored to `Until`'s date, minus one day when the start clock is later than the end clock (window crosses midnight). This mirrors the date-anchoring logic already in `translateSchedule` (`translate.go:190-197`). Then:

- `now < start` → not active (**pending**)
- `start <= now < until` → active
- `now >= until` → not active (expired)

Expose the derived start so the server can report a pending state. Concretely: add a helper in `internal/rules` (e.g. `OneTimeStart(w store.When) (time.Time, error)`) used by both `ActiveNow` and the handler layer. Windows are always well under 24 h (max delay 1 h + max bounded duration ~24 h "until morning"; in practice « 24 h), so the derivation is unambiguous.

## Pending state in the rules list

`ruleView` (`internal/server/rules_handlers.go:202-206`) gains one field:

```go
StartsAt string `json:"startsAt,omitempty"` // RFC3339; set only while a one-time rule has not started yet
```

`handleRulesList` sets it for enabled one-time rules whose derived start is still in the future. `Active` stays `false` for pending rules (per the `ActiveNow` fix). Pending is therefore: `pause && !active && startsAt` present.

`GET /api/status` (`handleStatus`) is not used by the SPA; it inherits the corrected `Active` semantics and needs no other change.

## Frontend (web/static)

**Card — `index.html` Quick pause section (lines 151-189):**

- New `Starting:` chip row above the duration buttons: `Now · in 15 min · in 30 min · in 1 hr`. Default `Now`. Selection is per-group Alpine state (e.g. `delaySel[p.id]`, default `'now'`), reset to `Now` after a pause is scheduled.
- While a delay is selected, the **Until I resume** chip is disabled.
- New **pending** block (shown when `groupPending(p.id)`): "Pauses at 8:30 PM · until 7:00 AM" with a **Cancel** button calling the existing `unpause(p.id)` — `removePauseRules` deletes by profile regardless of active state, so the endpoint works unchanged. The duration chips are hidden while a pause is pending (same pattern as the active state).
- The "Active" badge stays tied to `groupPause(p.id)`; a pending pause shows a distinct **"Scheduled"** badge, styled like the existing badge but visually secondary to "Active".
- Section subtitle (line 154) updated to mention scheduling ahead.

**Logic — `js/app.js`:**

- `pause(profileId, duration)` includes `delay` in the POST body when the group's selection isn't `Now`.
- New `groupPending(profileId)` — first rule with `pause && !active && startsAt` for the profile.
- `upcomingTodayList()` (lines 363-378) currently handles only recurring rules; add pending one-time pauses that start today so the Today strip surfaces them.

**CSS — `css/app.css`:** small additions for the `Starting:` row and pending box, reusing `.chip` / `.pausebox` styles.

## Out of scope

- Live countdown timers (the app has no polling today; times render as clock times).
- Custom/free-form delay values — presets only.
- Per-device (sub-group) pause targeting.

## Edge cases

- **Overnight windows** (e.g. start 23:45, 1 h duration → ends 00:45): already handled by `translateSchedule` date anchoring; the `ActiveNow` fix uses the same rule.
- **Start == end clock minute** (e.g. delayed start lands exactly at 7:00 AM with `morning` → 24 h window): rejected by `translateSchedule` with a clear message. Pre-existing behavior class, not a regression.
- **Rule expires or is spent**: janitor cleanup is `Until`-based (`rules.Expired`) and works unchanged for delayed pauses.
- **App restart while a pause is pending**: no in-process state; the rule lives in the store and on the gateway, so nothing is lost.

## Testing

- `internal/server` handler tests: `delay` parsing (valid values, unknown → 400), window math (`start = now + delay`, durations relative to start, `morning` anchored to start), `delay` + `indefinite` → 400, replace-existing-pause behavior, `startsAt` in the rules list for pending rules.
- `internal/rules` translate tests: `ActiveNow` one-time start-gating — pending / active / expired, including a window crossing midnight; `OneTimeStart` derivation.
- Verify the web build/serving still works (embedded static files) before calling it done.
