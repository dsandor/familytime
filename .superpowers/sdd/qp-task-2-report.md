# Task 2 Report — Delayed Quick Pause Implementation (2026-07-03)

## Summary

Successfully implemented Task 2: added `delay` parameter support to `POST /api/pause` and `startsAt` field to the rules list response in the Family Time app's HTTP layer.

## What Was Implemented

### 1. `handlePause` Function (internal/server/rules_handlers.go, lines 254–309)

**Changes:**
- Added `Delay string` field to input struct
- Implemented delay parsing with validation:
  - Accepts: `""` (absent), `"15m"`, `"30m"`, `"1h"`
  - Rejects unknown delays with HTTP 400 "Unknown pause delay."
- Modified schedule computation to use `start` (delayed time) instead of `now` for all duration calculations
- Added validation: `delay` with `"indefinite"` returns HTTP 400 with message: `"Until I resume" starts right away — pick a timed duration to schedule ahead.`
- Morning boundary calculation now anchors to `start`, not `now` (ensures "morning" duration calculates from the delay-adjusted start time)

**Unchanged:** Everything after the `// Replace any existing pause` comment remains exactly as before, including the `createFamilyRule` call and error handling.

### 2. `ruleView` Struct (internal/server/rules_handlers.go, line 206)

**Changes:**
- Added `StartsAt string` field with JSON tag: `json:"startsAt,omitempty"`
- Added inline comment: `// one-time rule scheduled but not yet started`

### 3. `handleRulesList` Function (internal/server/rules_handlers.go, lines 221–223)

**Changes:**
- Extended the `if fr.Enabled` block to populate `StartsAt`
- Added logic: if the rule is a one-time rule (`rules.OneTimeStart` returns ok=true) AND the current time is before its start, populate `v.StartsAt` with the start time in RFC3339 format
- Maintains existing `Active` and `Until` population; `StartsAt` is omitted for rules that are already active or non-one-time

**Unchanged:** `handleStatus` function requires no edit (inherits corrected Active semantics through `rules.ActiveNow`).

## Test Results

### Failing Tests (Step 2) — VERIFIED

Before implementation, ran:
```bash
go test ./internal/server/ -run 'TestPauseDelayed|TestPauseDelayValidation' -v
```

Expected failures confirmed:
- `TestPauseDelayedStart`: start was "20:00" (ignored delay), until was wrong, startsAt was empty
- `TestPauseDelayedMorningAnchorsToStart`: start was "23:50" instead of "00:20"
- `TestPauseDelayValidation`: unknown delay returned 200 instead of 400, indefinite+delay returned 200 instead of 400

### Green Tests (Step 4) — VERIFIED

**New tests only:**
```bash
go test ./internal/server/ -run 'TestPauseDelayed|TestPauseDelayValidation' -v
```

Result:
```
=== RUN   TestPauseDelayedStart
--- PASS: TestPauseDelayedStart (0.07s)
=== RUN   TestPauseDelayedMorningAnchorsToStart
--- PASS: TestPauseDelayedMorningAnchorsToStart (0.05s)
=== RUN   TestPauseDelayValidation
--- PASS: TestPauseDelayValidation (0.05s)
PASS
ok  	familytime/internal/server	0.382s
```

**Full package tests:**
```bash
go test ./internal/server/ -v
```

Result: **PASS** — All 59 tests passed (0:03.331s), including:
- Pre-existing `TestPauseReplaceAndUnpause` ✓ (immediate pauses store Start = now's clock minute, never in future, stay active-on-create)
- Pre-existing `TestStatusReportsActiveWindows` ✓ (status endpoint reports pauses correctly)
- New `TestPauseDelayedStart` ✓
- New `TestPauseDelayedMorningAnchorsToStart` ✓
- New `TestPauseDelayValidation` ✓

## Self-Review

### Completeness
- [x] All three test functions appended (TestPauseDelayedStart, TestPauseDelayedMorningAnchorsToStart, TestPauseDelayValidation)
- [x] `handlePause` input struct extended with `Delay` field
- [x] Delay validation with exact values: "15m", "30m", "1h"
- [x] Unknown delay returns 400 with exact message
- [x] Indefinite + delay returns 400 with exact message
- [x] Start time calculation uses delay
- [x] Morning duration anchors to delay-adjusted start
- [x] `ruleView` struct extended with `StartsAt` field
- [x] `handleRulesList` populates `StartsAt` for pending one-time rules
- [x] `handleStatus` unchanged (as required)

### Quality
- Code style matches file conventions (comment density, spacing, error handling patterns)
- Delay switch statement follows Go idiom (case "", case values, default)
- RFC3339 format used consistently (matches existing code)
- Omitempty tag used correctly on optional JSON fields
- Error messages are clear and user-facing

### Discipline (YAGNI)
- No extra functionality added beyond the brief
- No new dependencies introduced
- No changes to test helpers or unrelated functions
- Implementation is minimal and focused

### Testing Rigor
- TDD followed: tests written first, verified RED, then implementation GREEN
- All pre-existing tests still pass (no regressions)
- New tests cover:
  - Happy path: delayed pause with each duration (15m, 30m, 1h)
  - Edge case: morning duration with delay crossing midnight
  - Error cases: unknown delay, indefinite + delay
  - Rules list reports pending state correctly

## Files Changed

1. **internal/server/rules_handlers.go**
   - Lines 254–309: `handlePause` function (input struct, delay parsing, schedule computation)
   - Lines 202–207: `ruleView` struct (added StartsAt field)
   - Lines 221–223: `handleRulesList` function (populate StartsAt)

2. **internal/server/rules_handlers_test.go**
   - Lines 208–292: Added TestPauseDelayedStart, TestPauseDelayedMorningAnchorsToStart, TestPauseDelayValidation

## Issues & Concerns

None. The implementation is complete, tested, and ready for Task 3 (frontend consumption of `delay` parameter and `startsAt` field).
