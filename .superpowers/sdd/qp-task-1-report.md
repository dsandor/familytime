# Task 1 Report — OneTimeStart helper and start-gated ActiveNow

## Summary

Implemented `OneTimeStart` helper function and gated `ActiveNow` to respect start times for one-time pause rules. A Quick Pause scheduled for a future time now correctly shows as pending (inactive) until the start moment arrives, while still reporting the end time to the UI.

## Implementation

### Files Changed
- `/Users/dsandor/Projects/bedtime/internal/rules/translate.go`
  - Added `OneTimeStart(w store.When) (time.Time, bool)` function (lines 278–300)
  - Updated `ActiveNow` one-time case (lines 232–240) to gate on `OneTimeStart`

- `/Users/dsandor/Projects/bedtime/internal/rules/translate_test.go`
  - Added `TestOneTimeStart` (lines 310–332)
  - Added `TestActiveNowOneTimeStartGating` (lines 334–357)

### What Was Implemented

**`OneTimeStart` function:**
- Derives the moment a one-time window opens: the stored start clock anchored to Until's date
- For overnight windows (when end time < start time), anchors to the day before Until to match the gateway's schedule encoding
- Returns `(time.Time, bool)` where `ok=false` for legacy rules without a `Start` field or non-one-time kinds
- Mirrors the date-anchoring logic already used in `translateSchedule`

**`ActiveNow` gating:**
- Before gating: one-time rules were active immediately when created (`now.Before(until)`)
- After gating: one-time rules are inactive (pending) if `now.Before(start)`, but still report the `until` time
- Maintains backward compatibility: legacy rules without `Start` fall through to original behavior via `ok=false`
- Returns the same signature: `(active bool, until time.Time, hasUntil bool)`

## Testing

### TDD Process

**RED phase:** Tests fail with compile error
```
go test ./internal/rules/ -run 'TestOneTimeStart|TestActiveNowOneTimeStartGating' -v
internal/rules/translate_test.go:314:15: undefined: OneTimeStart
...
FAIL	familytime/internal/rules [build failed]
```

**GREEN phase:** All tests pass
```
go test ./internal/rules/ -v

=== RUN   TestTranslatePresetForProfile
--- PASS: TestTranslatePresetForProfile (0.00s)
...
=== RUN   TestActiveNow
--- PASS: TestActiveNow (0.00s)
=== RUN   TestOneTimeStart
--- PASS: TestOneTimeStart (0.00s)
=== RUN   TestActiveNowOneTimeStartGating
--- PASS: TestActiveNowOneTimeStartGating (0.00s)
PASS
ok  	familytime/internal/rules	0.244s
```

### Test Coverage

**`TestOneTimeStart`:**
- Overnight window 23:45 → 00:45: start anchors to day before Until ✓
- Same-day window 20:30 → 21:30: start anchors to Until's own day ✓
- Legacy rule without `Start` field: returns `ok=false` ✓
- Non-one-time kinds: returns `ok=false` ✓

**`TestActiveNowOneTimeStartGating`:**
- Pending state at 21:30 (before 22:00 start): `active=false, until=23:00, hasUntil=true` ✓
- Active state at 22:30 (inside 22:00–23:00 window): `active=true` ✓
- Expired state at 23:01 (after 23:00 end): `active=false` ✓
- Overnight window pending at 23:00 (before 23:45 start): `active=false` ✓
- Overnight window active at 00:10 (inside 23:45–00:45 window): `active=true` ✓

**Backward compatibility:**
- Pre-existing `TestActiveNow` still passes (21 tests total, all green)
- Legacy rules without `Start` field work correctly via the `ok=false` fallback

## Code Review

### Adherence to Brief
- Function signatures match exactly: `OneTimeStart(w store.When) (time.Time, bool)` ✓
- `ActiveNow` signature unchanged ✓
- Implementation logic mirrors `translateSchedule` for consistency ✓
- Test cases match brief verbatim ✓

### Style & Quality
- Comment density matches file (doc comment on function) ✓
- Error handling pattern matches existing code ✓
- Variable naming consistent with file conventions ✓
- Logic structure mirrors overnight-detection pattern from `translateSchedule` ✓
- No extraneous code (YAGNI) ✓

### Edge Cases
- Overnight windows correctly detect midnight crossing (`endM < startM`) ✓
- Missing or unparseable `Start` handled gracefully (returns `ok=false`) ✓
- Location/timezone preserved via `until.Location()` ✓
- Works with both new rules and legacy rules without `Start` ✓

## No Concerns

All requirements met, all edge cases covered, backward compatible, tests pristine.
