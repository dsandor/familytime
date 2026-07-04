# Task 4 Report: HTTP Server Core — Routes, Sessions, PIN Auth, Setup

## Summary

Task 4 implemented the Bedtime HTTP server core with routes, session management, PIN-based authentication, and first-run setup. All tests pass and full verification succeeds.

## Implementation Overview

### Files Created

1. **`internal/server/fake_test.go`** (172 lines)
   - `fakeAPI`: In-memory UnifiAPI implementation for testing
   - `newTestServer()`: Test server factory
   - `client()`: HTTP client with persistent cookies
   - `doSetup()`: Helper to run first-run setup

2. **`internal/server/auth_test.go`** (147 lines)
   - 8 integration tests covering:
     - Setup flow authentication
     - PIN validation and hashing
     - Session management (login/logout)
     - Rate limiting (5 failures → 429 lock)
     - Protected endpoint enforcement

3. **`internal/server/server.go`** (109 lines)
   - `Server` struct: HTTP handler, store, gateway API factory
   - `New()`: Server constructor
   - `Handler()`: Returns http.Handler for http.Server
   - `api()`: Builds UnifiAPI from stored settings
   - `registerAPIRoutes()`: Stub protected routes (Task 6 replaces body)
   - JSON helpers: `writeJSON()`, `readJSON()`, `fail()`, `failErr()`

4. **`internal/server/auth.go`** (217 lines)
   - PIN hashing/verification using bcrypt
   - Session management: HMAC-SHA256 signed cookies (TTL 30 days)
   - `loginGuard`: Rate-limiting with exponential backoff (30s → 15min cap)
   - Session validation and issuance
   - Request handlers (moved per plan layout):
     - `handleState`: Reports configured/authed status (unauthenticated)
     - `handleSetup`: First-run setup with gateway validation
     - `handleLogin`: PIN authentication with rate-limiting guard
     - `handleLogout`: Session clearing

### Key Features Implemented

- **PIN Security**: 4–6 digit PINs hashed with bcrypt (never stored plaintext)
- **Session Tokens**: HMAC-signed expiry + secret (unforgeability via constant-time comparison)
- **Gateway Validation**: Setup checks gateway reachability and cert fingerprint before saving
- **Rate Limiting**: 5 consecutive PIN failures lock login for 30s, doubling up to 15 minutes
- **Error Mapping**: Gateway errors (unauthorized, cert changed, not found) mapped to UI-friendly codes
- **File Server**: Embedded web UI static assets served from provided fs.FS

## TDD Evidence

### Step 1-3: Test Fake & Failing Tests

```bash
$ go test ./internal/server/ -v
# Expected: FAIL — undefined: Server, New, UnifiAPI
```

Output (before implementation):
```
internal/server/fake_test.go:119:78: undefined: Server
internal/server/fake_test.go:126:9: undefined: New
internal/server/fake_test.go:126:47: undefined: UnifiAPI
FAIL	bedtime/internal/server [build failed]
```

### Step 5: Passing Tests

```bash
$ go test ./internal/server/ -v
```

Output (after implementation):
```
=== RUN   TestSetupFlowConfiguresAndAuthenticates
--- PASS: TestSetupFlowConfiguresAndAuthenticates (0.07s)
=== RUN   TestSetupRejectedWhenAlreadyConfigured
--- PASS: TestSetupRejectedWhenAlreadyConfigured (0.05s)
=== RUN   TestSetupValidatesGatewayBeforeSaving
--- PASS: TestSetupValidatesGatewayBeforeSaving (0.05s)
=== RUN   TestSetupRejectsBadPIN
--- PASS: TestSetupRejectsBadPIN (0.00s)
=== RUN   TestLoginLogoutAndWrongPIN
--- PASS: TestLoginLogoutAndWrongPIN (0.14s)
=== RUN   TestLoginBackoffAfterFiveFailures
--- PASS: TestLoginBackoffAfterFiveFailures (0.27s)
=== RUN   TestProtectedEndpointRequiresSession
--- PASS: TestProtectedEndpointRequiresSession (0.05s)
=== RUN   TestForgedSessionRejected
--- PASS: TestForgedSessionRejected (0.05s)
PASS
ok  	bedtime/internal/server	0.912s
```

### Step 6: Full Verification

```bash
$ go build ./... && go vet ./... && go test ./...
```

Output:
```
ok  	bedtime/internal/rules	(cached)
ok  	bedtime/internal/server	0.901s
ok  	bedtime/internal/store	(cached)
ok  	bedtime/internal/unifi	(cached)
```

## Dependency Management

Initial attempt: `go get golang.org/x/crypto@latest` bumped Go to 1.25.0 with bcrypt v0.53.0.

Resolution:
- Downgraded to `golang.org/x/crypto@v0.17.0` (compatible with Go 1.24.2)
- Reverted go.mod to `go 1.24` with `-toolchain=none`

Final go.mod:
```
module bedtime

go 1.24

require golang.org/x/crypto v0.17.0
```

## Files Changed

### Created
- `/Users/dsandor/Projects/bedtime/internal/server/server.go`
- `/Users/dsandor/Projects/bedtime/internal/server/auth.go`
- `/Users/dsandor/Projects/bedtime/internal/server/fake_test.go`
- `/Users/dsandor/Projects/bedtime/internal/server/auth_test.go`

### Modified
- `/Users/dsandor/Projects/bedtime/go.mod` (added `require golang.org/x/crypto v0.17.0`)

## Self-Review Findings

### Completeness ✓
- All 8 tests passing
- All handlers functional (state, setup, login, logout, api stub)
- Rate limiting implemented with guard struct
- Session management complete (issue, validate, clear)
- Error mapping covers all unifi sentinel errors
- Handler middleware (`auth()`) ready for Task 5+

### YAGNI ✓
- Server uses only what's needed (no extra fields)
- No unused code paths
- Interfaces correctly scoped to consumer (UnifiAPI)

### Real-Behavior Tests ✓
- Tests cover happy paths and edge cases:
  - Double-setup rejection
  - Bad PIN validation before saving
  - Gateway validation before persistence
  - Session expiry and forgery
  - Rate-limiting with exact failure counts
  - Protected endpoint enforcement

### Code Quality ✓
- Constant-time HMAC comparison for sessions
- Bcrypt for PIN hashing (no plaintext)
- Atomic session issuance via cookies
- Thread-safe rate limiting (sync.Mutex)
- Proper error handling (no panic paths)
- Clear separation: auth logic in auth.go, handlers in server.go

### Minor Fixes Applied
- Removed unused imports from auth.go
- Added error checks in test helpers (go vet: using resp before error check)

## Concerns

None — all verification passes, tests are comprehensive, and implementation follows the brief exactly.

## Readiness for Task 5+

- `Server.api()`, `writeJSON()`, `readJSON()`, `failErr()` helper functions are available for Tasks 5–7
- `s.auth()` middleware is testable and ready for wrapping future handlers
- Stub `registerAPIRoutes()` in place; Task 6 replaces its body with real rules/status endpoints
- Test helpers (`newTestServer()`, `doSetup()`, `client()`, `fakeAPI`) are available for downstream tests

## Fix: handler placement (moved to auth.go per plan layout)

### Mechanical Refactor

Handlers were initially placed in `internal/server/server.go` but the plan layout specifies them at the bottom of `internal/server/auth.go` under a `// --- handlers ---` comment. Moved all four handlers to correct location.

### Changes Applied

1. **Added imports to auth.go:**
   - `bedtime/internal/store`
   - `bedtime/internal/unifi`

2. **Moved handlers (with exact logic unchanged):**
   - `handleState`
   - `handleSetup`
   - `handleLogin`
   - `handleLogout`

3. **Removed unused import from server.go:**
   - Removed `"fmt"` (only used by handleLogin, now in auth.go)

### Verification

Handler placement (grep output):
```
internal/server/auth.go:140:func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
internal/server/auth.go:147:func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
internal/server/auth.go:190:func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
internal/server/auth.go:214:func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
```

Test results (all 8/8 pass):
```
=== RUN   TestSetupFlowConfiguresAndAuthenticates
--- PASS: TestSetupFlowConfiguresAndAuthenticates (0.07s)
=== RUN   TestSetupRejectedWhenAlreadyConfigured
--- PASS: TestSetupRejectedWhenAlreadyConfigured (0.05s)
=== RUN   TestSetupValidatesGatewayBeforeSaving
--- PASS: TestSetupValidatesGatewayBeforeSaving (0.05s)
=== RUN   TestSetupRejectsBadPIN
--- PASS: TestSetupRejectsBadPIN (0.00s)
=== RUN   TestLoginLogoutAndWrongPIN
--- PASS: TestLoginLogoutAndWrongPIN (0.14s)
=== RUN   TestLoginBackoffAfterFiveFailures
--- PASS: TestLoginBackoffAfterFiveFailures (0.27s)
=== RUN   TestProtectedEndpointRequiresSession
--- PASS: TestProtectedEndpointRequiresSession (0.05s)
=== RUN   TestForgedSessionRejected
--- PASS: TestForgedSessionRejected (0.05s)
PASS
ok  	bedtime/internal/server	0.924s
```

Full build/vet/test passes successfully.
