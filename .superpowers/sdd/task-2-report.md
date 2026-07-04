# Task 2 Report — UniFi API Client Implementation

## Summary

Successfully implemented Task 2: the UniFi API client package (`bedtime/internal/unifi`). All 8 unit tests pass, full verification passes, code follows the brief exactly.

## What Was Implemented

Created three files as specified in the task brief:

1. **`internal/unifi/types.go`** — Type definitions and constants
   - Constants: `ActionBlock`, `MatchDomain`, `MatchAppCategory`, `MatchInternet`, `ModeAlways`, `ModeEveryDay`, `ModeEveryWeek`, `ModeOneTime`, `TargetTypeClient`, `TargetTypeAllClients`, `DirectionTo`
   - Types: `Domain`, `TargetDevice`, `Schedule`, `TrafficRule`, `Site`, `NetClient`
   - Helper: `NewBlockRule()` — creates a BLOCK rule with all slices pre-initialized to empty arrays (not null)

2. **`internal/unifi/client_test.go`** — Comprehensive test suite (8 tests)
   - Parses probe fixture (4 real rules from live gateway)
   - Tests rule creation accepting both 200 and 201 status codes
   - Verifies empty arrays marshal as `[]` not `null`
   - Tests PUT-to-ID path for updates
   - Tests error mapping (401/403 → ErrUnauthorized, 404 → ErrNotFound)
   - Tests v1 API client pagination (official API)
   - Tests TLS certificate pinning verification
   - Tests FirstSite v1 API call

3. **`internal/unifi/client.go`** — HTTP client implementation
   - `New(host, apiKey, certFingerprint string) *Client` — builds client; handles bare IPs and full URLs; supports pinned-certificate verification
   - `FetchCertFingerprint(ctx, host string) (string, error)` — SHA-256 hex fingerprint of leaf cert (trust-on-first-use)
   - Error types: `ErrUnauthorized`, `ErrNotFound`, `ErrCertChanged` (all work with `errors.Is`)
   - v1 API methods: `Version(ctx)`, `FirstSite(ctx)`, `ListClients(ctx, siteID)`
   - v2 API methods: `ListTrafficRules(ctx)`, `CreateTrafficRule(ctx, rule)`, `UpdateTrafficRule(ctx, rule)`, `DeleteTrafficRule(ctx, id)`
   - Internal `do()` helper: handles JSON marshaling, header setup, error mapping, and context cancellation

## TDD Evidence

### RED Phase (Test Fails)

Command: `go test ./internal/unifi/ -v`

Output (excerpt):
```
internal/unifi/client_test.go:39:7: undefined: New
internal/unifi/client_test.go:77:8: undefined: New
internal/unifi/client_test.go:98:7: undefined: New
...
internal/unifi/client_test.go:125:45: undefined: ErrUnauthorized
internal/unifi/client_test.go:125:89: undefined: ErrNotFound
...
FAIL	bedtime/internal/unifi [build failed]
```

Tests fail as expected with undefined symbols (`New`, `Client`, error types).

### GREEN Phase (Tests Pass)

Command: `go test ./internal/unifi/ -v`

Output:
```
=== RUN   TestListTrafficRulesParsesProbeFixture
--- PASS: TestListTrafficRulesParsesProbeFixture (0.00s)
=== RUN   TestCreateTrafficRuleAccepts200And201
--- PASS: TestCreateTrafficRuleAccepts200And201 (0.00s)
=== RUN   TestCreateSendsEmptyArraysNotNull
--- PASS: TestCreateSendsEmptyArraysNotNull (0.00s)
=== RUN   TestUpdateTrafficRulePutsToIDPath
--- PASS: TestUpdateTrafficRulePutsToIDPath (0.00s)
=== RUN   TestErrorMapping
--- PASS: TestErrorMapping (0.00s)
=== RUN   TestListClientsPaginates
--- PASS: TestListClientsPaginates (0.00s)
=== RUN   TestPinnedCertVerification
--- PASS: TestPinnedCertVerification (0.01s)
=== RUN   TestFirstSite
--- PASS: TestFirstSite (0.00s)
PASS
ok  	bedtime/internal/unifi	0.263s
```

All 8 tests pass.

## Files Changed

Created:
- `/Users/dsandor/Projects/bedtime/internal/unifi/types.go` (98 lines)
- `/Users/dsandor/Projects/bedtime/internal/unifi/client.go` (244 lines)
- `/Users/dsandor/Projects/bedtime/internal/unifi/client_test.go` (215 lines)

Preexisting (unchanged):
- `/Users/dsandor/Projects/bedtime/internal/unifi/testdata/trafficrules_probe.json` (fixture with 4 probe rules from live gateway)

## Full-Suite Verification

Command: `go build ./... && go vet ./... && go test ./...`

Output:
```
ok  	bedtime/internal/store	(cached)
ok  	bedtime/internal/unifi	0.207s
```

✓ Build succeeds with no errors
✓ `go vet` finds no issues
✓ All tests pass across all packages

## Self-Review

### Completeness vs Brief ✓

- [x] All types defined exactly as specified
- [x] All constants defined exactly as specified
- [x] `New()` constructor handles bare IPs and full URLs
- [x] `FetchCertFingerprint()` implemented with proper error handling
- [x] TLS certificate pinning uses SHA-256 hex matching
- [x] All v1 API methods implemented (Version, FirstSite, ListClients with pagination)
- [x] All v2 API methods implemented (ListTrafficRules, CreateTrafficRule, UpdateTrafficRule, DeleteTrafficRule)
- [x] Error types (`ErrUnauthorized`, `ErrNotFound`, `ErrCertChanged`) work with `errors.Is()`
- [x] All 8 test cases from the brief pass
- [x] Fixture parsing works (4 probe rules with various shapes: weekly, ALL_CLIENTS, one-time, INTERNET)

### Code Quality

- **YAGNI:** No over-engineering. Each method does exactly what it says.
- **Naming:** Clear, idiomatic Go (e.g., `do()`, `trafficRulesPath()`, `pinnedTLSConfig()`).
- **Error Handling:** Wrapped errors with context, sentinel errors for matching.
- **Testing:** 8 comprehensive tests covering:
  - JSON parsing with real fixture
  - HTTP status code variants (200/201, 401, 403, 404)
  - Array serialization (empty arrays as `[]` not `null`)
  - PUT path construction
  - Pagination loop
  - TLS verification (correct pin, wrong pin, certificate fetch)
  - Official v1 API parsing
- **Comments:** Explain why TLS verification is skipped (self-signed cert) and the pinning strategy.

### Concerns / Notes

**None.** Implementation is clean, tests are robust, and it matches the brief exactly.

The TLS handshake error logged during the certificate pinning test is expected (it's testing the rejection case) and is not a real error — the test asserts that the connection fails with the correct `ErrCertChanged`.

## Verification Command Outputs (Raw)

### RED Phase
```
# bedtime/internal/unifi [bedtime/internal/unifi.test]
internal/unifi/client_test.go:39:7: undefined: New
internal/unifi/client_test.go:77:8: undefined: New
internal/unifi/client_test.go:98:7: undefined: New
internal/unifi/client_test.go:116:7: undefined: New
internal/unifi/client_test.go:125:45: undefined: ErrUnauthorized
internal/unifi/client_test.go:125:89: undefined: ErrNotFound
internal/unifi/client_test.go:129:8: undefined: New
internal/unifi/client_test.go:164:7: undefined: New
internal/unifi/client_test.go:182:7: undefined: New
internal/unifi/client_test.go:187:9: undefined: New
internal/unifi/client_test.go:187:9: too many errors
FAIL	bedtime/internal/unifi [build failed]
FAIL
```

### GREEN Phase
```
=== RUN   TestListTrafficRulesParsesProbeFixture
--- PASS: TestListTrafficRulesParsesProbeFixture (0.00s)
=== RUN   TestCreateTrafficRuleAccepts200And201
--- PASS: TestCreateTrafficRuleAccepts200And201 (0.00s)
=== RUN   TestCreateSendsEmptyArraysNotNull
--- PASS: TestCreateSendsEmptyArraysNotNull (0.00s)
=== RUN   TestUpdateTrafficRulePutsToIDPath
--- PASS: TestUpdateTrafficRulePutsToIDPath (0.00s)
=== RUN   TestErrorMapping
--- PASS: TestErrorMapping (0.00s)
=== RUN   TestListClientsPaginates
--- PASS: TestListClientsPaginates (0.00s)
=== RUN   TestPinnedCertVerification
2026/07/03 00:29:27 http: TLS handshake error from 127.0.0.1:64579: remote error: tls: bad certificate
--- PASS: TestPinnedCertVerification (0.01s)
=== RUN   TestFirstSite
--- PASS: TestFirstSite (0.00s)
PASS
ok  	bedtime/internal/unifi	0.263s
```

### Full Verification
```
ok  	bedtime/internal/store	(cached)
ok  	bedtime/internal/unifi	0.207s
```
