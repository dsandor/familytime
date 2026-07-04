# Task 1 Report — Server `POST /api/pause/{profileId}/delay`

**Status:** DONE

**Date:** 2026-07-03

---

## What Was Implemented

Implemented the server endpoint `POST /api/pause/{profileId}/delay` that grants more internet time on an existing pause. The pause lifts (or its scheduled start pushes out) and re-engages later, keeping its original end. A grant that outlasts the pause removes it instead.

Additionally extracted the `nextMorning` helper function from the `handlePause` "morning" case and applied it there as well, improving code reuse.

---

## Files Modified

1. **`internal/server/rules_handlers.go`**
   - Extracted `nextMorning(t time.Time) time.Time` helper function (line 345–351)
   - Updated `handlePause` to use `nextMorning` instead of inlined logic (line 296)
   - Implemented `handlePauseDelay` handler (line 353–439) with:
     - Input validation for delay duration ("15m", "30m", "1h")
     - Profile and pause rule lookup
     - Proper handling of WhenAlways (indefinite pauses) and WhenOneTime kinds
     - Collapse logic: removes pause when grant outlasts it (`!until.After(newStart.Add(time.Minute))`)
     - Response with `{"removed":true}` when collapsed, or the rescheduled FamilyRule otherwise

2. **`internal/server/handlers.go`**
   - Registered route at line 31: `s.mux.Handle("POST /api/pause/{profileId}/delay", s.auth(s.handlePauseDelay))`

3. **`internal/server/rules_handlers_test.go`**
   - Appended 5 comprehensive test cases covering:
     - `TestPauseGrantKeepsOriginalEnd` — active pause, 30m grant keeps original end time
     - `TestPauseGrantOutlastingPauseRemovesIt` — grant that exceeds pause duration removes the pause (active and pending cases)
     - `TestPauseGrantIndefiniteEndsAtMorning` — indefinite pause granted shows next morning end
     - `TestPauseGrantPendingPushesPromisedStart` — pending pause grant counts delay from promised start, not now
     - `TestPauseGrantValidation` — validates 404 on no pause, 404 on unknown profile, 400 on unknown delay

---

## TDD Evidence

### RED Phase (Tests Fail)

```
$ go test ./internal/server/ -run 'TestPauseGrant' -v
=== RUN   TestPauseGrantKeepsOriginalEnd
    rules_handlers_test.go:419: grant = 404
--- FAIL: TestPauseGrantKeepsOriginalEnd (0.07s)
=== RUN   TestPauseGrantOutlastingPauseRemovesIt
    rules_handlers_test.go:447: grant = 404, out = map[], want removed:true
--- FAIL: TestPauseGrantOutlastingPauseRemovesIt (0.05s)
=== RUN   TestPauseGrantIndefiniteEndsAtMorning
    rules_handlers_test.go:472: grant = 404
--- FAIL: TestPauseGrantIndefiniteEndsAtMorning (0.05s)
=== RUN   TestPauseGrantPendingPushesPromisedStart
    rules_handlers_test.go:498: grant = 404
--- FAIL: TestPauseGrantPendingPushesPromisedStart (0.05s)
=== RUN   TestPauseGrantValidation
    rules_handlers_test.go:523: unknown delay = 404, want 400
--- FAIL: TestPauseGrantValidation (0.05s)
FAIL
FAIL	familytime/internal/server	0.522s
```

**Expected failures:** All new tests fail with 404 (route not registered), exactly as specified in the brief. The validation test also fails because the route doesn't exist to validate the delay parameter.

### GREEN Phase (Tests Pass)

```
$ go test ./internal/server/ -run 'TestPauseGrant' -v
=== RUN   TestPauseGrantKeepsOriginalEnd
--- PASS: TestPauseGrantKeepsOriginalEnd (0.07s)
=== RUN   TestPauseGrantOutlastingPauseRemovesIt
--- PASS: TestPauseGrantOutlastingPauseRemovesIt (0.05s)
=== RUN   TestPauseGrantIndefiniteEndsAtMorning
--- PASS: TestPauseGrantIndefiniteEndsAtMorning (0.05s)
=== RUN   TestPauseGrantPendingPushesPromisedStart
--- PASS: TestPauseGrantPendingPushesPromisedStart (0.05s)
=== RUN   TestPauseGrantValidation
--- PASS: TestPauseGrantValidation (0.05s)
PASS
ok  	familytime/internal/server	0.438s
```

All 5 new tests pass. The full package test suite also passes:

```
$ go test ./internal/server/ -v
...
=== RUN   TestPauseReplaceAndUnpause
--- PASS: TestPauseReplaceAndUnpause (0.05s)
=== RUN   TestPauseDelayedMorningAnchorsToStart
--- PASS: TestPauseDelayedMorningAnchorsToStart (0.05s)
...
PASS
ok  	familytime/internal/server	3.586s
```

**All 56 tests in the package pass**, including pre-existing pause tests which verify that the `nextMorning` extraction did not break `handlePause`.

---

## Implementation Details

### Wire Contract Compliance

✅ **Route:** `POST /api/pause/{profileId}/delay`  
✅ **Request body:** `{"delay":"15m"|"30m"|"1h"}`  
✅ **Unknown delay → 400:** "Unknown pause delay."  
✅ **No pause rule → 404:** "Nothing is paused."  
✅ **Unknown profile → 404:** "No such profile."  
✅ **Collapse response:** `{"removed":true}`  
✅ **Normal response:** stored `FamilyRule` JSON  

### Collapse Logic

Implemented exactly per spec:
```go
if !until.After(newStart.Add(time.Minute)) {
    writeJSON(w, 200, map[string]bool{"removed": true})
    return
}
```

### Reused Interfaces

- `oneTimeUntil(now, until time.Time)` — wraps times into store.When
- `removePauseRules(ctx, profileID)` — gateway-first deletion
- `createFamilyRule(ctx, fr, p)` — persists and syncs to gateway
- `profileForID(d, id)` — resolves profile including virtual "Everyone"
- `readJSON(r, &in)` — deserializes request body
- `fail(w, code, msg)` / `failErr(w, err)` — error responses
- `writeJSON(w, code, obj)` — JSON response
- `rules.OneTimeStart(w store.When)` — extracts scheduled start time

### Comment Density

Matches existing codebase style:
- Top-level function comment explaining purpose
- Inline comment for "pending: "+30 min" counts from the promised start" clarification
- Inline comment for collapse condition ("The grant outlasts...")
- Inline comment for "Until I resume" indefinite pause handling

---

## Self-Review Findings

✅ **Completeness:**
- All 4 steps from brief executed
- `nextMorning` extraction applied to both `handlePause` and new `handlePauseDelay`
- Exact error messages and collapse response used verbatim
- All interfaces consumed as specified

✅ **Quality:**
- Code style matches existing handlers
- Proper error handling with gateway-first pattern
- Comment clarity at appropriate level
- No over-engineering (YAGNI principle followed)

✅ **Testing:**
- RED evidence: all 5 tests fail with 404 (expected)
- GREEN evidence: all 5 tests pass (plus 51 pre-existing tests)
- Build succeeds: `go build ./cmd/familytime`
- Pre-existing pause tests remain green (verifying nextMorning extraction is correct)

✅ **No Issues or Concerns**

---

## Build Verification

```
$ go build -o /tmp/familytime-test ./cmd/familytime
(no errors)
```

Application builds cleanly with all changes.
