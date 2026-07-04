# Final-review fix wave — report

All findings from the approved whole-project review have been applied. No git commands were run (not a git repo). The live gateway (192.168.0.1) was never contacted — only the in-memory `fakeAPI` in `internal/server/fake_test.go` was exercised, and the e2e suite was left self-skipping (verified below).

---

## C1 (Critical): profile edits must re-target the profile's existing gateway rules

**Files/functions changed:**
- `internal/server/handlers.go`
  - `handleProfileUpdate` — rewritten. Now: snapshot store → validate input → compute `updated` profile → policy check (zero devices + existing rules ⇒ 400) → gateway-first loop over all of the profile's `store.FamilyRule`s (`rules.Translate(fr, updated)`, `UpdateTrafficRule`, recreate via `CreateTrafficRule` on `unifi.ErrNotFound`, mirroring `handleRuleUpdate`'s recreate pattern) → single `store.Update` that persists both the profile change and any recreated rule ids together.
  - New helper `persistRecreatedRuleIDs(recreated map[string][]string)` — on a mid-way gateway failure, persists only the ids of rules that were actually recreated (never the device change), preserving the store ⊆ gateway invariant. Logs (via `log.Printf`) if that best-effort persist itself fails.
  - `handleDevices` also touched for item 7 (see below); `log` added to imports.

**Covering tests** (`internal/server/handlers_test.go`):
- `TestProfileUpdateRetargetsGatewayRules` — seeds a profile + rule, PUTs the profile with a second device, asserts the fake gateway rule now has 2 `TargetDevices` and the store has 2 devices.
- `TestProfileUpdateRejectsZeroDevicesWithRules` — seeds a profile + rule, PUTs `devices: []`, asserts 400 and that both the gateway rule and the stored profile devices are unchanged.
- `TestProfileUpdateRecreatesVanishedGatewayRule` — seeds a profile + rule, wipes `fake.rules` (simulating a UniFi-app deletion), PUTs the profile, asserts the rule was recreated with a new gateway id and the store id was updated to match.

**Test command + result:**
```
go test ./internal/server/... -run 'TestProfileUpdate' -v
--- PASS: TestProfileUpdateRetargetsGatewayRules (0.05s)
--- PASS: TestProfileUpdateRejectsZeroDevicesWithRules (0.05s)
--- PASS: TestProfileUpdateRecreatesVanishedGatewayRule (0.05s)
PASS
```
`TestProfileCRUD` (pre-existing, `internal/server/handlers_test.go`) still passes — its zero-device update has no rules attached, so the new policy check is a no-op there, as expected.

---

## I1: rotate session secret on PIN change

**Files/functions changed:**
- `internal/server/settings_handlers.go` `handleSettingsPIN` — the `store.Update` closure now also sets `d.Auth.SessionSecret = newSecret()`. After the update succeeds, `s.issueSession(w)` issues the caller a fresh cookie signed with the new secret, so they stay logged in while every other browser's cookie (signed with the old secret) fails `validSession`.

**Covering test** (`internal/server/rules_handlers_test.go`):
- `TestSettingsPINChangeRotatesSessionSecret` — logs in a second "browser" (fresh cookie jar) alongside the setup client, confirms both are authed, changes the PIN via the first client, then asserts the second client's `/api/status` now returns 401 while the first client's still returns 200.

**Test command + result:**
```
go test ./internal/server/... -run 'TestSettingsPINChangeRotatesSessionSecret' -v
--- PASS: TestSettingsPINChangeRotatesSessionSecret (0.18s)
PASS
```

---

## I2: restrict env-API-key fallback to the detected gateway host

**Files/functions changed:**
- `internal/server/server.go` — added `detectGateway func() string` field to `Server`, initialized to `guessGateway` in `New`.
- `internal/server/auth.go`
  - New `normalizeHost(h string) string` — strips scheme (`scheme://`) and surrounding whitespace/trailing slash for comparison.
  - New `(s *Server) envKeyAllowedFor(host string) bool` — `true` only when `detectGateway()` is non-empty and `normalizeHost(host) == normalizeHost(detectGateway())`.
  - `handleSetup` and `handleTestConnection` — the `os.Getenv("UNIFI_API_KEY")` fallback now only fires when `s.envKeyAllowedFor(in.Host)`. `handleSetup` still 400s "An API key is required." when the key ends up empty.
  - `handleState` — `envApiKey` hint now `os.Getenv("UNIFI_API_KEY") != "" && s.detectGateway() != ""`.

**Covering tests** (`internal/server/auth_test.go`):
- `TestStateSetupHintsAndEnvKeyFallback` (updated) — now captures `srv` and sets `srv.detectGateway = func() string { return "http://fake" }` so the setup POST's host (`"http://fake"`) matches after normalization; unchanged assertions otherwise.
- `TestEnvKeyFallbackOnlyForDetectedGateway` (new) — (a) with `detectGateway` returning `"http://fake-gateway"`, a setup POST to a different host with an empty key gets 400 and leaves the store unconfigured; (b) with `detectGateway` returning `""`, `/api/state`'s `envApiKey` hint is `false` even though the env var is set.

**Test command + result:**
```
go test ./internal/server/... -run 'TestStateSetupHintsAndEnvKeyFallback|TestEnvKeyFallbackOnlyForDetectedGateway' -v
--- PASS: TestStateSetupHintsAndEnvKeyFallback (0.05s)
--- PASS: TestEnvKeyFallbackOnlyForDetectedGateway (0.00s)
PASS
```

---

## I3: log discarded gateway errors

**Files/functions changed:**
- `internal/server/server.go` `failErr` — default branch now does `log.Printf("bedtime: gateway error: %v", err)` before writing the friendly "Can't reach your UniFi gateway right now." response. `"log"` added to imports.

**Covering test** (new file `internal/server/server_test.go`):
- `TestFailErrLogsUnknownGatewayErrors` — sets `fake.failAll` to an opaque error, redirects `log` output to a buffer for the duration of the test, POSTs a rule create (hits the default `failErr` branch), asserts the response is 502 and the buffer contains the underlying error text.

**Test command + result:**
```
go test ./internal/server/... -run 'TestFailErrLogsUnknownGatewayErrors' -v
--- PASS: TestFailErrLogsUnknownGatewayErrors (0.05s)
PASS
```
(Side effect visible in the full suite run too: `TestSetupValidatesGatewayBeforeSaving` and `TestRuleCreateGatewayDownPersistsNothing` now print `bedtime: gateway error: http: Handler timeout` — expected, both already exercised the default branch.)

---

## I4: searchable device picker

**Files/functions changed:**
- `web/static/index.html` — added `<input type="text" x-model="deviceQuery" placeholder="Search by name or device address">` directly above `.device-list`; the `x-for` now iterates `filteredDevices()` instead of `devices`. No new CSS needed — the input picks up the existing `input[type=text]` rule (same border/padding/focus treatment as every other text field in the app).
- `web/static/js/app.js`
  - `deviceQuery: ''` added to the root state object.
  - New `filteredDevices()` method — case-insensitive substring match against device name or MAC; returns `this.devices` unfiltered when the query is blank.
  - `newProfile()` and `editProfile()` both reset `this.deviceQuery = ''`.

**Verification:** `node --check web/static/js/app.js` confirms no syntax errors; `go build ./...` (which embeds `web/static` via `web/embed.go`) succeeds. There's no JS test harness in this project (no npm/build step), so this was verified by static syntax check + code review rather than an automated test — consistent with how the rest of the vanilla-Alpine frontend is validated in this repo.

---

## Deferred-triage fixes

### (5) `internal/rules/translate.go` — zero-padded times

`translateSchedule`: after `parseHM` validates `w.Start`/`w.End` (recurring) or `w.Start` (one-time), the resulting minute counts are reformatted with `fmt.Sprintf("%02d:%02d", m/60, m%60)` into `TimeRangeStart`/`TimeRangeEnd`, instead of passing the raw user string through.

**Covering tests** (`internal/rules/translate_test.go`):
- `TestTranslateZeroPadsRecurringTimes` — `Start: "9:00"` → `TimeRangeStart == "09:00"`.
- `TestTranslateZeroPadsOneTimeStart` — one-time rule `Start: "9:00"` → `TimeRangeStart == "09:00"`.

```
go test ./internal/rules/... -run 'TestTranslateZeroPads' -v
--- PASS: TestTranslateZeroPadsRecurringTimes (0.00s)
--- PASS: TestTranslateZeroPadsOneTimeStart (0.00s)
PASS
```

### (6) `internal/rules/translate.go` — reject degenerate windows

Same function: recurring windows with non-empty `Start == End` now return `"start and end times can't be the same"`; one-time windows where `until`'s HH:MM equals `Start` now return `"the pause needs to end at a different time of day than it starts"`. The overnight-anchor date logic (`endM < startM` ⇒ anchor to previous day) was tightened from `<=` to `<` since equality is now rejected earlier in the same branch.

**Covering tests:**
- `TestTranslateRejectsDegenerateRecurringWindow` — `Start: "20:00", End: "20:00"` errors, message contains "can't be the same".
- `TestTranslateRejectsDegenerateOneTimeWindow` — `Start: "21:00"`, `until` at 21:00 same day errors, message contains "different time of day".

```
go test ./internal/rules/... -run 'TestTranslateRejectsDegenerate' -v
--- PASS: TestTranslateRejectsDegenerateRecurringWindow (0.00s)
--- PASS: TestTranslateRejectsDegenerateOneTimeWindow (0.00s)
PASS
```
All pre-existing `translate_test.go` cases (including the overnight-anchor test) still pass unchanged.

### (7) `internal/server/handlers.go` — `handleDevices` sort tiebreak

`sort.Slice` → `sort.SliceStable`, comparator now falls back to MAC comparison when two devices have the same `Name`.

**Covering test** (`internal/server/handlers_test.go`):
- `TestDevicesSortsByNameThenMAC` — two live clients with identical name `"Kid Tablet"` and MACs `bb:...:02` / `aa:...:01`; asserts response order is `aa:...:01` then `bb:...:02`.

```
go test ./internal/server/... -run 'TestDevicesSortsByNameThenMAC' -v
--- PASS: TestDevicesSortsByNameThenMAC (0.05s)
PASS
```

---

## Small accepted-but-cheap items

### M4 — `cmd/bedtime/main.go` `loadDotEnv` strips quotes

New `unquoteEnvValue(v string) string` strips one layer of matching `"…"` or `'…'` around a value; `loadDotEnv` now calls it on the trimmed value before `os.Setenv`.

**Covering test** (new file `cmd/bedtime/main_test.go`):
- `TestUnquoteEnvValue` — table test covering plain values, double- and single-quoted values, a lone quote character (left alone), empty string, mismatched quote pair (left alone), and `""` (empty quoted value → `""`).

```
go test ./cmd/bedtime/... -run TestUnquoteEnvValue -v
--- PASS: TestUnquoteEnvValue (0.00s)
PASS
```

### M5 — `internal/store/store.go` `Load` rejects newer versions

After successful JSON unmarshal, if `s.data.Version > 1`, `Load` now returns an error naming both the file path and the version: `"store: %s was created by a newer Bedtime (version %d)"`.

**Covering test** (`internal/store/store_test.go`):
- `TestLoadRejectsNewerVersion` — writes `{"version":2}` to a temp file, asserts `Load` errors and the message contains the file path and "newer Bedtime".

```
go test ./internal/store/... -run TestLoadRejectsNewerVersion -v
--- PASS: TestLoadRejectsNewerVersion (0.00s)
PASS
```

### M6 — `web/static/js/app.js` UI robustness

- `goRules(profileId)` — now clears `this.rules = []` at the start (before the fetch), and the catch block sets `this.banner = e.message` instead of silently swallowing the error.
- `saveRule()` — gets an in-flight guard: early-returns if `wizard.saving` is already true, sets it before the request, clears it in the failure path (success navigates away via `goRules`, so the wizard object is discarded). `wizard.saving: false` added to `startWizard()`'s initial state.
- `web/static/index.html` — the "Create rule" button now has `:disabled="!wizard.name.trim() || wizard.saving"` and swaps its label to "Creating…" while in flight (mirrors the existing `setup.testing` button pattern).

**Verification:** No JS test harness in this repo; verified via `node --check web/static/js/app.js` (passes) and manual code review against the existing `setup.testing`/`testConnection()` pattern this mirrors.

### M8 — `internal/e2e/e2e_test.go` cleanup uses a fresh context

The deferred cleanup closure now creates its own `context.WithTimeout(context.Background(), 30*time.Second)` (`cctx`/`ccancel`) instead of reusing the test's `ctx`, so cleanup can still run even if the outer 60s test context already expired.

**Verification:** This test self-skips without `BEDTIME_E2E=1` (per task instructions, it was **not** run). Confirmed it still compiles and self-skips cleanly as part of `go test ./...` (see full-suite output below: `--- SKIP: TestLiveGatewayRuleLifecycle (0.00s)`, reason `"set BEDTIME_E2E=1 to run against the real gateway"`). The live gateway was never contacted.

### M9 — settings gateway form copy

`web/static/index.html` — the gateway settings API key label changed from `"API key"` to `"API key (required to change gateway)"`.

---

## Full-suite verification (final run)

```
$ go build ./...
(clean)

$ go vet ./...
(clean)

$ gofmt -l .
(no output — clean)

$ go test ./...
ok  	bedtime/cmd/bedtime	0.163s
ok  	bedtime/internal/e2e	0.258s     (TestLiveGatewayRuleLifecycle: SKIP — BEDTIME_E2E not set)
ok  	bedtime/internal/rules	0.504s
ok  	bedtime/internal/server	2.505s
ok  	bedtime/internal/store	0.611s
ok  	bedtime/internal/unifi	(cached)
?   	bedtime/web	[no test files]
```

All packages pass. `internal/e2e` was confirmed to self-skip (no network contact with 192.168.0.1 or any gateway) — verified via `-v` output showing `set BEDTIME_E2E=1 to run against the real gateway`.

### New/changed test inventory
| Package | New tests |
|---|---|
| `internal/server` | `TestProfileUpdateRetargetsGatewayRules`, `TestProfileUpdateRejectsZeroDevicesWithRules`, `TestProfileUpdateRecreatesVanishedGatewayRule`, `TestDevicesSortsByNameThenMAC`, `TestSettingsPINChangeRotatesSessionSecret`, `TestEnvKeyFallbackOnlyForDetectedGateway`, `TestFailErrLogsUnknownGatewayErrors` |
| `internal/server` | `TestStateSetupHintsAndEnvKeyFallback` (updated in place) |
| `internal/rules` | `TestTranslateZeroPadsRecurringTimes`, `TestTranslateZeroPadsOneTimeStart`, `TestTranslateRejectsDegenerateRecurringWindow`, `TestTranslateRejectsDegenerateOneTimeWindow` |
| `internal/store` | `TestLoadRejectsNewerVersion` |
| `cmd/bedtime` | `TestUnquoteEnvValue` |

No git commands were run at any point; nothing was committed or pushed; the live gateway was never contacted.

---

## Fix: partial-failure intent persistence in handleProfileUpdate

**Finding (Major, re-review):** In `handleProfileUpdate`'s gateway-first retarget loop, if rule N (N>1) failed after rules 1..N-1 had already succeeded via `UpdateTrafficRule`, the handler returned an error *without* persisting the profile's new device list. The store/UI kept showing the OLD devices while rules 1..N-1 were already enforcing the NEW ones on the gateway — a silent, permanent divergence, since the janitor only reconciles rule-id existence, never target-device content.

**Fix:** on any mid-loop failure, persist the profile's new intent (name/emoji/color/devices) together with any recreated rule ids in one `store.Update`, then return a friendlier gateway error telling the parent to re-save. Retrying converges the rest of the rules, because rule translation always starts from the stored profile and gateway `PUT`s are idempotent. Full-success and the pre-loop 400 rejections (zero devices with rules, Everyone) are unchanged.

**Files changed:**
- `internal/server/handlers.go`
- `internal/server/fake_test.go` (test fake only)
- `internal/server/handlers_test.go` (new test)

### Diff — `internal/server/handlers.go`

```diff
-// persistRecreatedRuleIDs updates unifiRuleIds for rules that were recreated
-// on the gateway mid-way through a profile update, without touching the
-// profile itself. Keeps store rule ids a subset of what's really on the
-// gateway even when the overall request fails partway through.
-func (s *Server) persistRecreatedRuleIDs(recreated map[string][]string) {
-	if len(recreated) == 0 {
-		return
-	}
-	if err := s.store.Update(func(d *store.Data) error {
-		for i := range d.Rules {
-			if ids, ok := recreated[d.Rules[i].ID]; ok {
-				d.Rules[i].UnifiRuleIDs = ids
-			}
-		}
-		return nil
-	}); err != nil {
-		log.Printf("bedtime: failed to persist recreated rule ids after profile update abort: %v", err)
-	}
-}
+// persistProfileAndRuleIDs writes the profile's new name/emoji/color/devices
+// together with any rule ids recreated on the gateway so far, in a single
+// store update.
+func (s *Server) persistProfileAndRuleIDs(id string, updated store.Profile, recreated map[string][]string) error {
+	return s.store.Update(func(d *store.Data) error {
+		for i := range d.Profiles {
+			if d.Profiles[i].ID == id {
+				d.Profiles[i] = updated
+			}
+		}
+		for i := range d.Rules {
+			if ids, ok := recreated[d.Rules[i].ID]; ok {
+				d.Rules[i].UnifiRuleIDs = ids
+			}
+		}
+		return nil
+	})
+}
+
+// persistPartialProfileUpdate persists the profile's new intent (and any
+// rule ids recreated so far) when the gateway-first loop in
+// handleProfileUpdate stops partway through. Without this, a mid-loop
+// failure would leave rules 1..N-1 already enforcing the new devices on the
+// gateway while the store/UI kept showing the old ones — a silent,
+// permanent divergence the janitor can't detect (it only reconciles rule-id
+// existence, never target-device content). Persisting the intent here means
+// a later re-save, or any edit to one of this profile's rules, re-translates
+// from the stored profile and converges the rest of the rules (gateway PUTs
+// are idempotent, so retrying is safe).
+func (s *Server) persistPartialProfileUpdate(id string, updated store.Profile, recreated map[string][]string) {
+	if err := s.persistProfileAndRuleIDs(id, updated, recreated); err != nil {
+		log.Printf("bedtime: failed to persist profile intent after partial gateway failure: %v", err)
+	}
+}
+
+// failPartialProfileUpdate reports a gateway error that stopped
+// handleProfileUpdate's retarget loop partway through, after the caller has
+// already persisted the new intent via persistPartialProfileUpdate. It keeps
+// failErr's status/code mapping (so the UI's existing gateway-error handling
+// still applies) but swaps in a message that tells the parent their change
+// was saved and a re-save will finish syncing the gateway.
+func failPartialProfileUpdate(w http.ResponseWriter, err error) {
+	code := "unreachable"
+	switch {
+	case errors.Is(err, unifi.ErrUnauthorized):
+		code = "unauthorized"
+	case errors.Is(err, unifi.ErrCertChanged):
+		code = "cert_changed"
+	case errors.Is(err, unifi.ErrNotFound):
+		code = "not_found"
+	default:
+		log.Printf("bedtime: gateway error: %v", err)
+	}
+	writeJSON(w, http.StatusBadGateway, errBody{
+		Error: "Saved your changes, but some of this profile's rules couldn't be updated on the gateway — check your connection and save this profile again.",
+		Code:  code,
+	})
+}
```

```diff
-	// Gateway first: re-target every one of this profile's rules before
-	// persisting the device change, so the store never claims a gateway
-	// state that isn't real. recreated tracks ids we DID successfully
-	// recreate, so a mid-way failure doesn't orphan them (store ⊆ gateway).
+	// Gateway first: re-target every one of this profile's rules before
+	// reporting success, so a fully successful request never claims a
+	// gateway state that isn't real. If the loop stops partway through,
+	// each failure path persists the profile's new intent (and any ids
+	// recreated so far) anyway — see persistPartialProfileUpdate — because
+	// the alternative (silently keeping the old devices in the store while
+	// rules 1..N-1 already enforce the new ones on the gateway) is a worse,
+	// permanent divergence that nothing else can detect or repair.
 	recreated := map[string][]string{}
 	for _, fr := range profileRules {
 		tr, err := rules.Translate(fr, updated)
 		if err != nil {
-			s.persistRecreatedRuleIDs(recreated)
+			s.persistPartialProfileUpdate(id, updated, recreated)
 			fail(w, 400, err.Error())
 			return
 		}
 		if len(fr.UnifiRuleIDs) == 1 {
 			tr.ID = fr.UnifiRuleIDs[0]
 			err = s.api().UpdateTrafficRule(r.Context(), tr)
 		} else {
 			err = unifi.ErrNotFound
 		}
 		if errors.Is(err, unifi.ErrNotFound) {
 			// Defensive: clear any stale gateway ids so the recreate can't
 			// leave untracked rules enforcing (deleteGatewayRules tolerates
 			// not-found).
 			if derr := s.deleteGatewayRules(r.Context(), fr.UnifiRuleIDs); derr != nil {
-				s.persistRecreatedRuleIDs(recreated)
-				failErr(w, derr)
+				s.persistPartialProfileUpdate(id, updated, recreated)
+				failPartialProfileUpdate(w, derr)
 				return
 			}
 			tr.ID = ""
 			created, cerr := s.api().CreateTrafficRule(r.Context(), tr)
 			if cerr != nil {
-				s.persistRecreatedRuleIDs(recreated)
-				failErr(w, cerr)
+				s.persistPartialProfileUpdate(id, updated, recreated)
+				failPartialProfileUpdate(w, cerr)
 				return
 			}
 			recreated[fr.ID] = []string{created.ID}
 			err = nil
 		}
 		if err != nil {
-			s.persistRecreatedRuleIDs(recreated)
-			failErr(w, err)
+			s.persistPartialProfileUpdate(id, updated, recreated)
+			failPartialProfileUpdate(w, err)
 			return
 		}
 	}
 
-	err := s.store.Update(func(d *store.Data) error {
-		for i := range d.Profiles {
-			if d.Profiles[i].ID == id {
-				d.Profiles[i] = updated
-			}
-		}
-		for i := range d.Rules {
-			if ids, ok := recreated[d.Rules[i].ID]; ok {
-				d.Rules[i].UnifiRuleIDs = ids
-			}
-		}
-		return nil
-	})
-	if err != nil {
+	if err := s.persistProfileAndRuleIDs(id, updated, recreated); err != nil {
 		fail(w, 500, err.Error())
 		return
 	}
 	writeJSON(w, 200, updated)
 }
```

Notes on scope: the persist-and-friendlier-error treatment was applied to **all** failure exits inside the loop (translate error, `deleteGatewayRules` error, `CreateTrafficRule` error, `UpdateTrafficRule` error), not just the literal `UpdateTrafficRule` case named in the finding — the same silent-divergence risk exists for any of them once at least one earlier rule has already been retargeted on the gateway. The translate-error branch keeps its original `fail(w, 400, err.Error())` message (it's a data problem, not a gateway/connectivity one); only the three genuine gateway-call failures get the new `failPartialProfileUpdate` message.

### Diff — `internal/server/fake_test.go` (test fake, minimal per-rule failure support)

```diff
-// fakeAPI is an in-memory UnifiAPI. Set failAll to make every call error.
+// fakeAPI is an in-memory UnifiAPI. Set failAll to make every call error, or
+// failUpdateID to make UpdateTrafficRule fail only for that one rule id.
 type fakeAPI struct {
-	mu      sync.Mutex
-	rules   []unifi.TrafficRule
-	clients []unifi.NetClient
-	nextID  int
-	failAll error
+	mu           sync.Mutex
+	rules        []unifi.TrafficRule
+	clients      []unifi.NetClient
+	nextID       int
+	failAll      error
+	failUpdateID string
 }
```

```diff
 func (f *fakeAPI) UpdateTrafficRule(ctx context.Context, r unifi.TrafficRule) error {
 	if e := f.err(); e != nil {
 		return e
 	}
 	f.mu.Lock()
 	defer f.mu.Unlock()
+	if f.failUpdateID != "" && r.ID == f.failUpdateID {
+		return fmt.Errorf("simulated gateway failure updating rule %s", r.ID)
+	}
 	for i := range f.rules {
 		if f.rules[i].ID == r.ID {
 			f.rules[i] = r
 			return nil
 		}
 	}
 	return unifi.ErrNotFound
 }
```

### New test — `internal/server/handlers_test.go`

Added `TestProfileUpdatePersistsIntentOnPartialGatewayFailure` (inserted before `TestDevicesSortsByNameThenMAC`): seeds a 2-rule profile, fails only the second rule's `UpdateTrafficRule` via `fake.failUpdateID`, and asserts:
- (a) the PUT response is a 5xx with a non-empty friendly error message;
- (b) the store now holds the new 2-device list (the intent was persisted despite the failure);
- (c) gateway rule 1 has the new targets (2 devices), gateway rule 2 still has the old targets (1 device, since its update failed);
- (d) clearing `fake.failUpdateID` and re-saving the same profile returns 200 and converges rule 2 to the new targets too.

```go
// TestProfileUpdatePersistsIntentOnPartialGatewayFailure covers the
// gateway-first retarget loop stopping partway through: rule 1 succeeds on
// the gateway, rule 2's UpdateTrafficRule call fails. The handler must still
// persist the new device list (and any recreated rule ids) to the store —
// leaving the store on the old devices while the gateway already enforces
// the new ones on rule 1 would be a silent, permanent divergence. It should
// report an error so the UI can tell the parent to retry, and a later
// re-save (once the gateway call stops failing) must converge rule 2 too.
func TestProfileUpdatePersistsIntentOnPartialGatewayFailure(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	st.Update(func(d *store.Data) error {
		d.Profiles = []store.Profile{{
			ID: "p1", Name: "Emma", Emoji: "🦄",
			Devices: []store.Device{{MAC: "aa:aa:aa:aa:aa:01", Name: "iPad"}},
		}}
		return nil
	})
	var rule1, rule2 store.FamilyRule
	if code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p1","name":"No YouTube","what":{"type":"preset","presetId":"youtube"},"when":{"kind":"always"}}`, &rule1); code != 200 {
		t.Fatalf("create rule1 = %d", code)
	}
	if code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p1","name":"No TikTok","what":{"type":"preset","presetId":"tiktok"},"when":{"kind":"always"}}`, &rule2); code != 200 {
		t.Fatalf("create rule2 = %d", code)
	}
	gw2, ok := fake.ruleByDesc(rule2.ID)
	if !ok {
		t.Fatal("gateway rule2 missing")
	}

	// Make the second rule's gateway update fail.
	fake.mu.Lock()
	fake.failUpdateID = gw2.ID
	fake.mu.Unlock()

	var errOut errBody
	code := doJSON(t, c, "PUT", ts.URL+"/api/profiles/p1",
		`{"name":"Emma","emoji":"🦄","devices":[{"mac":"aa:aa:aa:aa:aa:01","name":"iPad"},{"mac":"aa:aa:aa:aa:aa:02","name":"Switch"}]}`, &errOut)
	if code < 500 || code >= 600 {
		t.Fatalf("partial gateway failure = %d, want 5xx; body=%+v", code, errOut)
	}
	if errOut.Error == "" {
		t.Error("expected a friendly error message in the response body")
	}

	// (b) store holds the NEW device list despite the mid-loop failure.
	got := st.Snapshot().Profiles[0].Devices
	if len(got) != 2 {
		t.Fatalf("store profile devices = %+v, want 2 (new list persisted)", got)
	}

	// (c) gateway rule 1 has the new targets, rule 2 still has the old ones.
	gw1, ok := fake.ruleByDesc(rule1.ID)
	if !ok {
		t.Fatal("gateway rule1 missing")
	}
	if len(gw1.TargetDevices) != 2 {
		t.Errorf("gateway rule1 target devices = %+v, want 2 (retargeted)", gw1.TargetDevices)
	}
	gw2After, ok := fake.ruleByDesc(rule2.ID)
	if !ok {
		t.Fatal("gateway rule2 missing")
	}
	if len(gw2After.TargetDevices) != 1 {
		t.Errorf("gateway rule2 target devices = %+v, want 1 (unchanged, update failed)", gw2After.TargetDevices)
	}

	// (d) clearing the failure and re-saving the same profile converges
	// rule 2 to the new targets with a 200.
	fake.mu.Lock()
	fake.failUpdateID = ""
	fake.mu.Unlock()

	code = doJSON(t, c, "PUT", ts.URL+"/api/profiles/p1",
		`{"name":"Emma","emoji":"🦄","devices":[{"mac":"aa:aa:aa:aa:aa:01","name":"iPad"},{"mac":"aa:aa:aa:aa:aa:02","name":"Switch"}]}`, nil)
	if code != 200 {
		t.Fatalf("retry profile update = %d, want 200", code)
	}
	gw2Final, ok := fake.ruleByDesc(rule2.ID)
	if !ok {
		t.Fatal("gateway rule2 missing after retry")
	}
	if len(gw2Final.TargetDevices) != 2 {
		t.Errorf("gateway rule2 target devices after retry = %+v, want 2 (converged)", gw2Final.TargetDevices)
	}
}
```

### Test output

```
$ go test ./internal/server/... -run 'TestProfileUpdate|TestProfileCRUD|TestProfileValidation|TestProfileRejectsMACAssignedElsewhere|TestProfileDeleteCascadesItsRules' -v
=== RUN   TestProfileCRUD
--- PASS: TestProfileCRUD (0.06s)
=== RUN   TestProfileValidation
--- PASS: TestProfileValidation (0.05s)
=== RUN   TestProfileRejectsMACAssignedElsewhere
--- PASS: TestProfileRejectsMACAssignedElsewhere (0.05s)
=== RUN   TestProfileUpdateRetargetsGatewayRules
--- PASS: TestProfileUpdateRetargetsGatewayRules (0.05s)
=== RUN   TestProfileUpdateRejectsZeroDevicesWithRules
--- PASS: TestProfileUpdateRejectsZeroDevicesWithRules (0.05s)
=== RUN   TestProfileUpdateRecreatesVanishedGatewayRule
--- PASS: TestProfileUpdateRecreatesVanishedGatewayRule (0.05s)
=== RUN   TestProfileUpdatePersistsIntentOnPartialGatewayFailure
2026/07/03 10:27:42 bedtime: gateway error: simulated gateway failure updating rule u2
--- PASS: TestProfileUpdatePersistsIntentOnPartialGatewayFailure (0.05s)
=== RUN   TestProfileDeleteCascadesItsRules
--- PASS: TestProfileDeleteCascadesItsRules (0.05s)
PASS
ok  	bedtime/internal/server	1.134s
```

All three pre-existing C1 tests (`TestProfileUpdateRetargetsGatewayRules`, `TestProfileUpdateRejectsZeroDevicesWithRules`, `TestProfileUpdateRecreatesVanishedGatewayRule`) still pass unchanged, confirming full-success and pre-loop-rejection behavior is untouched.

### Full verification

```
$ go build ./...
(clean)

$ go vet ./...
(clean)

$ gofmt -l .
(no output — clean)

$ go test ./...
ok  	bedtime/cmd/bedtime	(cached)
ok  	bedtime/internal/e2e	(cached)
ok  	bedtime/internal/rules	(cached)
ok  	bedtime/internal/server	(cached)
ok  	bedtime/internal/store	(cached)
ok  	bedtime/internal/unifi	(cached)
?   	bedtime/web	[no test files]
```

No git commands were run; nothing was committed or pushed; the live gateway (192.168.0.1) was never contacted — only `fakeAPI` in `internal/server/fake_test.go` was exercised.
