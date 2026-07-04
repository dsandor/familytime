# "Give 30 More Minutes" — Design

**Date:** 2026-07-03
**Status:** Approved

## Purpose

When a group is currently paused (or a pause is scheduled), let a parent grant more internet time in one tap: the pause lifts now (or its start pushes out) and re-engages later, **keeping the pause's original end**. Completes the story begun by the delayed Quick Pause feature — the API-side "replace while paused" behavior becomes reachable from the UI.

## Semantics (approved)

New endpoint: `POST /api/pause/{profileId}/delay` with body `{"delay": "15m" | "30m" | "1h"}`. The server acts on the group's existing pause rule; the client sends no timestamps.

| Current pause state | Result of a grant of `d` |
|---|---|
| **Active, timed** (one-time window ending at T) | Replaced by a scheduled pause: starts `now + d`, still ends at T. Internet is on in between. |
| **Active, "Until I resume"** (`WhenAlways`) | Replaced by a scheduled pause: starts `now + d`, ends at the first 7:00 AM strictly after that start (same "morning" rule as `handlePause`; a scheduled restart needs an end time). |
| **Pending** (scheduled one-time, start S in future) | Start pushes to `S + d` ("30 MORE minutes"), end T unchanged. |
| **Legacy one-time without a usable start** (`rules.OneTimeStart` not ok) | Treated as active: starts `now + d`, ends at its T. |
| **No pause rule** | 404 "Nothing is paused." |
| **Unknown delay value** | 400 "Unknown pause delay." |

**Collapse rule:** if the original end T would arrive before — or within one minute of — the new start (`!T.After(newStart.Add(time.Minute))`), the grant outlasts the pause: remove the pause entirely instead of rescheduling. Response `{"removed": true}`; otherwise 200 with the stored `FamilyRule` (same shape as `POST /api/pause`).

## Backend

`internal/server/rules_handlers.go` + route in `handlers.go` next to the existing pause routes.

- Handler `handlePauseDelay`: parse delay → find the profile's pause rule from the store snapshot (`ProfileID == {profileId} && Pause`; profile must exist → 404 otherwise, matching `handlePause`) → compute the new window per the table → same gateway-first replace flow as `handlePause` (`removePauseRules` then `createFamilyRule`), or `removePauseRules` alone for the collapse case.
- Extract the "first 7:00 AM strictly after t" computation from `handlePause` into a small helper (e.g. `nextMorning(t time.Time) time.Time`) and use it from both call sites — a third inline copy would compound the duplication already noted in review.
- Validation before any mutation, consistent with `handlePause` (bad requests never remove an existing pause).
- No changes to `internal/rules` — `OneTimeStart` and `translateSchedule` already cover the needed windows.

## Frontend (web/static)

- Active (green) pausebox: secondary compact button **"30 more min"** next to Resume.
- Pending (amber) pausebox: secondary compact button **"+30 min"** next to Cancel.
- Both call new `grantMore(profileId)` → `POST /api/pause/{profileId}/delay {"delay":"30m"}` → `loadCore()`; errors go to the banner like `pause`/`unpause`. After a grant the card flips to the Scheduled state ("Pauses at 8:30 PM · until 7:00 AM").
- UI ships only the fixed 30-minute grant; the API accepts 15m/1h for the future.
- Small CSS rule for the secondary pausebox button reusing existing glass tokens (the pausebox currently only styles a primary button).

## Edge cases

- Grant on a pause whose remaining time ≤ the grant → pause removed (collapse rule); UI shows the un-paused card after refresh.
- Repeated grants: each grant acts on the current (now pending) rule — start pushes out again, end unchanged, until collapse.
- Degenerate same-clock-minute windows can 400 in the gateway translation (pre-existing class; rare).
- "Everyone" profile behaves like any group.

## Testing

`internal/server` handler tests in the existing style (fixed clock, fake gateway, JSON round-trips): active-timed keeps end; short-remaining collapses (store and gateway both empty, `removed:true`); indefinite becomes morning-ended; pending pushes start from S not now; no pause → 404; unknown delay → 400 without side effects; repeated grant pushes again. Full `gofmt`/build/suite green.
