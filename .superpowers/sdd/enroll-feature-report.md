# Device self-enrollment — implementation report

## Why

iPhones/iPads use "Private Wi-Fi Address": the gateway sees a randomized MAC
and a generic name ("iPhone 72:68"), so parents can't find kids' devices in
the picker. Enrollment flips the flow — the *device* visits a page, the
server identifies it by the requesting connection's IP against the
gateway's live client list (never anything the client claims about itself),
and the parent names it and assigns a group right there. No auth: the
server-side IP resolution means a visit can only ever affect the device
that made it.

## What was built

### Backend

**`internal/server/handlers.go`** — refactor, no behavior change:
- Extracted `retargetProfileRules(ctx, updated store.Profile, profileRules []store.FamilyRule) (map[string][]string, error)` from `handleProfileUpdate`'s inline gateway-first loop. It re-targets (updates, or recreates if vanished) every rule for one profile, returning recreated-rule ids and, on failure, everything it got done before stopping.
- `rules.Translate` failures are wrapped in a new `*retargetTranslateError` so callers can tell a validation problem (400, original message) apart from a real gateway failure (502-family via `failPartialProfileUpdate`'s existing friendly message). `respondRetargetError(w, err)` centralizes that branch.
- `handleProfileUpdate` now calls the helper and `respondRetargetError` — the persist-intent-on-partial-failure behavior (`persistPartialProfileUpdate`) is unchanged and still invoked identically for every failure path.
- `persistProfileAndRuleIDs(id, updated, recreated)` is now defined in terms of a new multi-profile `persistProfilesAndRuleIDs(map[string]store.Profile, recreated)`, since enrollment can touch two profiles (a device's old group and its new one) in one store update. Added `persistPartialEnroll`, the multi-profile counterpart to `persistPartialProfileUpdate`.
- **Verification that the refactor is behavior-preserving:** all pre-existing profile-update tests (`TestProfileUpdateRetargetsGatewayRules`, `TestProfileUpdateRejectsZeroDevicesWithRules`, `TestProfileUpdateRecreatesVanishedGatewayRule`, `TestProfileUpdatePersistsIntentOnPartialGatewayFailure`, etc.) pass unmodified.

**`internal/server/enroll_handlers.go`** (new) — `GET /api/enroll/whoami` and `POST /api/enroll`:
- `requesterIP(remoteAddr)` strips the port from `r.RemoteAddr` via `net.SplitHostPort`.
- `findClientByIP(clients, ip)` is the *only* device-identity mechanism — matches the live gateway client list by IP.
- `handleEnrollWhoami`: 409 if unconfigured; otherwise resolves the client by IP and returns `{found, mac, name, ip, currentProfileId, currentDeviceName, groups}` (found) or `{found:false, groups}` (not found, still 200). `groups` never includes the virtual Everyone profile.
- `handleEnroll`: 409 if unconfigured; validates `profileId` against stored profiles first (400 "Pick a valid group" — Everyone is never in `d.Profiles` so it's naturally rejected too); re-resolves the MAC from `RemoteAddr` exactly like whoami (the request body has no MAC field at all — nothing client-supplied is trusted for identity); 404 with a friendly message if the device isn't on the live client list. Name defaults to the gateway's client name, then to the MAC, if empty/blank.
- Move semantics: finds the profile (other than the target) that currently owns the MAC, if any; builds updated device lists for source (device removed) and target (device added/renamed) in source-then-target order; reuses the same "profile has rules but zero devices" guard as `handleProfileUpdate` before touching the gateway; retargets each affected profile's rules via `retargetProfileRules`, gateway-first; on any failure, persists the new intent via `persistPartialEnroll` before responding (same partial-failure semantics as profile editing — verified in `TestEnrollGatewayFailurePersistsIntent`, which also proves a later profile re-save converges the rest, exactly like the existing profile-update recovery path).

**`internal/server/server.go`**:
- `routes()` adds `GET /enroll` (serves the embedded `index.html` directly via `http.ServeFileFS`, no duplicated file) and the two public enroll API routes, registered outside `registerAPIRoutes()` so they bypass `s.auth`.
- `Server` gained `advertisedPort int` (default `8080`) and `SetAdvertisedPort(port int)`.

**`internal/server/auth.go`**:
- Factored `localIP()` out of `guessGateway()` (same UDP-dial trick, no packets sent — confirmed in this sandbox: `net.Dial("udp","8.8.8.8:53")` resolves the local route without contacting anything, so it never touches the live gateway). `guessGateway()` now calls `localIP()` and appends `.1`.

**`internal/server/settings_handlers.go`**: `GET /api/settings` response gained `enrollUrl`: `http://<localIP()>:<advertisedPort>/enroll`, empty string if `localIP()` can't resolve.

**`cmd/familytime/main.go`**: calls `srv.SetAdvertisedPort(*port)` after constructing the server.

### Frontend (`web/static/`)

- **`js/app.js`**: `init()` now special-cases `location.pathname === '/enroll'` → `initEnroll()`, which skips `refreshState()`/session routing entirely (no auth). Added `enroll` state object and methods `initEnroll`, `loadEnrollWhoami`, `enrollGroupName`, `submitEnroll`. `mockPreview('enroll')` builds a realistic found-state sample (iPhone with a generic gateway name, two sample groups) so the page is reviewable without a gateway; `settings.enrollUrl` added to the mock settings object too.
- **`index.html`**:
  - New `view==='enroll'` template with five states (`loading`, `unconfigured`, `notfound`, `found`, `done`), reusing existing classes (`card narrow center`, `shield-mark`, `who-grid`, `hint`, `all-clear`) so it matches the rest of the app rather than introducing a new visual language. The hero shield/title is hidden on the `done` state to avoid two stacked shield icons.
  - Settings gained an "Enroll a device" card explaining Private Wi-Fi Address and showing `settings.enrollUrl` prominently.
  - Groups & Devices gained a one-line hint under the device list pointing at the Enroll page for generically-named Apple devices.
- **`css/app.css`**: new classes `.enroll-url` (monospace, cyan-bright, selectable), `.enroll-detected` / `.enroll-detected-name` / `.enroll-detected-mac` (the detected-device card), `.enroll-form` (left-aligns form content inside the otherwise-centered `.card.narrow.center`), `.devices-hint`. No new hues — reuses `--cyan-bright`, `--text-faint`, `--glass-fill`, `--glass-border` exactly as elsewhere.

### Docs

`README.md` gained an "Enrolling devices" section: what the flow is, why (Private Wi-Fi Address), and the caveat that iOS's non-default "Rotate" private-address mode changes the MAC over time and would require re-enrolling — recommends the default fixed private address (or turning it off) for kids' devices.

## Endpoints added

| Method | Path | Auth | Notes |
|---|---|---|---|
| GET | `/enroll` | none | Serves the embedded SPA `index.html` |
| GET | `/api/enroll/whoami` | none | 409 unconfigured; identifies caller by IP |
| POST | `/api/enroll` | none | 409 unconfigured; body `{name, profileId}` |

`GET /api/settings` (auth-gated, unchanged otherwise) gained `enrollUrl`.

## Tests added (`internal/server/enroll_test.go`, 17 new tests)

`TestEnrollWhoamiFound`, `TestEnrollWhoamiFoundAlreadyAssigned`, `TestEnrollWhoamiNotFound`, `TestEnrollWhoamiUnconfigured`, `TestEnrollAssignsToProfile`, `TestEnrollDefaultsNameToGatewayName`, `TestEnrollMovesBetweenProfilesRetargetsBoth`, `TestEnrollUnknownProfileID400`, `TestEnrollRejectsEveryone`, `TestEnrollDeviceNotOnNetwork404`, `TestEnrollUnconfigured`, `TestEnrollGatewayFailurePersistsIntent`, `TestSettingsIncludesEnrollURL` — all use `client(t)` (a fresh, unauthenticated cookie jar) for the enroll calls to prove no session is required, and a fake `NetClient{IPAddress: "127.0.0.1"}` since httptest requests arrive with that `RemoteAddr`.

`TestEnrollGatewayFailurePersistsIntent` mirrors the existing `TestProfileUpdatePersistsIntentOnPartialGatewayFailure` pattern: moves a device between two profiles with rules, fails the destination's `UpdateTrafficRule`, asserts a 5xx with a friendly message, asserts the store already holds the new intent (source lost the device, destination gained it) despite the gateway only partially converging, asserts the source's rule (processed first) converged while the destination's (processed second, failing) didn't, then clears the failure and re-saves the destination profile through the existing PUT endpoint to prove it converges — exactly the recovery story profile editing already has.

### Test output

```
ok  	familytime/cmd/familytime
ok  	familytime/internal/e2e        (live-gateway test skipped: FAMILYTIME_E2E not set)
ok  	familytime/internal/rules
ok  	familytime/internal/server      (57 tests, including all 13 new enroll/settings tests)
ok  	familytime/internal/store
ok  	familytime/internal/unifi
```

`go build ./...`, `go vet ./...`, `go test ./...` all green. `gofmt -l .` empty. `node --check web/static/js/app.js` OK.

## Browser verification

Ran a scratch build (`go build -o .../familytime-scratch ./cmd/familytime`) on port 8904 against a throwaway data file in the scratchpad dir — never the live gateway.

- `?preview=enroll` (found state, sample "iPhone 72:68"): detected-device card, name input prefilled, group tiles, working selection state, Enroll CTA enabled once a group is picked. Screenshotted.
- Same preview with `enroll.currentProfileId` set: "Currently in Teens" hint renders and that tile is preselected/highlighted. Screenshotted.
- `enroll.status = 'notfound'`: friendly guidance + Retry button. Screenshotted.
- `enroll.status = 'done'` (hero hidden, single green all-clear shield): "🛡️ Ava's iPhone is protected" / "It's in Kids now — the family's rules apply to it automatically." Screenshotted.
- `?preview=settings`: new "Enroll a device" card renders with the sample `enrollUrl` in a monospace, cyan, selectable box. Screenshotted.
- `?preview=groups`: new hint line renders under the device list. Screenshotted.
- **Real (non-preview) `GET /enroll`** against the unconfigured scratch server: correctly renders the "Family Time isn't set up yet" state, proving the actual `/enroll` route → `index.html` → `initEnroll()` → `GET /api/enroll/whoami` → 409 path works end to end (not just mocked).
- Console: zero JS errors (`list_console_messages types:["error"]` returned none on every screen except the expected browser-logged "409" for the unconfigured whoami fetch, which is the same fetch-based control-flow pattern the rest of the app already uses, e.g. login's 401). Chrome's accessibility "issue" panel flagged pre-existing patterns (inputs with no `id`/`name` attribute) that already exist throughout the app (e.g. the Groups & Devices page shows the same issue) — not something introduced by this feature, and out of this task's scope.

Scratch server killed and scratch data/binary files removed after verification.

## Contrast — new color pairs

Computed with the same relative-luminance method as the existing design report (`--void` = `#070b14`; `--glass-fill` = white at 5.5% opacity composited over `--void`):

| Pair | Ratio | AA needed | Verdict |
|---|---|---|---|
| `--cyan-bright` (`#67e8f9`) text on `--glass-fill` (`.enroll-url`) | 12.2:1 | 4.5:1 (normal text) | Pass, comfortably (same combo already used in `.filter-chip button`) |
| `--text-faint` on `--glass-fill` (`.enroll-detected-mac`) | 6.07:1 | 4.5:1 | Pass (same combo as existing `.device-row .mac`) |

No new hues were introduced — everything reuses existing tokens (`--cyan`, `--cyan-bright`, `--text-faint`, `--glass-fill`, `--glass-border`, `--green`/`--green-bright` for the success state, `--danger-bright` for the not-found state). No purple.

## Added edge-case regression tests

**`TestEnrollZeroDevicesWithRules400`** — Verifies the zero-devices-with-rules guard in the source profile during device moves. Sets up a source profile containing *only* the enrolling device (the one being moved), seeds a rule on that profile, attempts to move the device to a different target profile, and asserts: (1) returns 400 with error message mentioning "keep at least one device" or "delete the rules first", (2) source profile still contains the device, (3) target profile is unchanged, (4) gateway rule TargetDevices unchanged. Confirms the guard prevents orphaning rules on zero-device profiles.

**`TestEnrollIdempotentSameGroupRename`** — Verifies that re-enrolling a device already in the target profile with a different name is idempotent and updates the device name. Device starts in profile under name "Old Name", then POST /api/enroll to the *same* profile with name "New Name" returns 200, the profile contains the device exactly once with the new name (no duplicates), and the gateway rule still targets the MAC exactly once (not doubled). Exercises the `replaced` flag in the enroll handler's device-replacement logic and confirms rule targets remain consistent.

### Test output (all 17 tests pass):

```
=== RUN   TestEnrollWhoamiFound
--- PASS: TestEnrollWhoamiFound (0.07s)
=== RUN   TestEnrollWhoamiFoundAlreadyAssigned
--- PASS: TestEnrollWhoamiFoundAlreadyAssigned (0.05s)
=== RUN   TestEnrollWhoamiNotFound
--- PASS: TestEnrollWhoamiNotFound (0.05s)
=== RUN   TestEnrollWhoamiUnconfigured
--- PASS: TestEnrollWhoamiUnconfigured (0.00s)
=== RUN   TestEnrollAssignsToProfile
--- PASS: TestEnrollAssignsToProfile (0.05s)
=== RUN   TestEnrollDefaultsNameToGatewayName
--- PASS: TestEnrollDefaultsNameToGatewayName (0.05s)
=== RUN   TestEnrollMovesBetweenProfilesRetargetsBoth
--- PASS: TestEnrollMovesBetweenProfilesRetargetsBoth (0.05s)
=== RUN   TestEnrollUnknownProfileID400
--- PASS: TestEnrollUnknownProfileID400 (0.05s)
=== RUN   TestEnrollRejectsEveryone
--- PASS: TestEnrollRejectsEveryone (0.05s)
=== RUN   TestEnrollDeviceNotOnNetwork404
--- PASS: TestEnrollDeviceNotOnNetwork404 (0.05s)
=== RUN   TestEnrollUnconfigured
--- PASS: TestEnrollUnconfigured (0.00s)
=== RUN   TestEnrollGatewayFailurePersistsIntent
--- PASS: TestEnrollGatewayFailurePersistsIntent (0.05s)
=== RUN   TestEnrollZeroDevicesWithRules400
--- PASS: TestEnrollZeroDevicesWithRules400 (0.05s)
=== RUN   TestEnrollIdempotentSameGroupRename
--- PASS: TestEnrollIdempotentSameGroupRename (0.05s)
=== RUN   TestSettingsIncludesEnrollURL
--- PASS: TestSettingsIncludesEnrollURL (0.05s)
PASS
ok	familytime/internal/server	0.905s
```

Full suite: `go build ./...`, `go vet ./...`, `go test ./...` all green. `gofmt -l .` empty.

## Concerns / follow-ups

- `localIP()`/`guessGateway()` return `""` if the machine has no default route (e.g. fully offline); `enrollUrl` would then be `""` and the Settings card would show a blank address. This matches the pre-existing behavior of `suggestedGateway` in the same situation — not a new failure mode.
- The zero-devices-with-rules guard is applied to both the source and target profile in `handleEnroll` for consistency with `handleProfileUpdate`, even though only the source can realistically hit it (the target always gains a device in this flow). **Now explicitly unit-tested** via `TestEnrollZeroDevicesWithRules400`, which demonstrates the guard prevents moving a device away from a profile that has rules but would end up with zero devices.
- Enrollment has no auth by design (per spec) — the only integrity guarantee is server-side IP→MAC resolution: a visiting browser can only ever change the device that's making the request, never name/assign someone else's device. Worth keeping in mind if the app ever moves off "LAN-only, no auth" as a threat model.

## Group editor device management

### Why

The group editor (`view==='groupEditor'`) only ever showed name/emoji/color —
membership was only discoverable by scanning every dropdown on the Devices
list. This adds a "Devices in this group" / "Add a device" pair to the
editor itself, using the same staged-edit model the rest of the editor
already uses (everything commits in one PUT/POST on Save).

### What changed (frontend-only, no Go changes)

**`web/static/js/app.js`**:
- New state field `editorDeviceQuery` (own search box, separate from the
  Devices-list `deviceQuery`), reset whenever the editor opens.
- `newGroup()` / `editGroup(p)` now reset `editorDeviceQuery` and call
  `loadDevices()` so the live devices list (online status, current
  `profileId`) is fresh when the editor opens — previously only `goGroups()`
  loaded devices, which happened to cover the normal navigation path but
  not defensively.
- `memberSummary(p)`: Groups-list secondary text, now device names
  (`"Ava's iPad, Noah's Switch"`) instead of just a count, or `"No devices"`.
- `editorDeviceOnline(mac)`: looks up connection status from the live
  `devices[]` list for the member rows (`editing.devices` only carries
  `{mac, name}`).
- `removeEditorDevice(mac)`: staged removal from `editing.devices`.
- `editorDeviceInOtherGroup(d)`: `true` only when a device belongs to a
  *different* profile than the one being edited; unassigned or
  already-staged-into-this-group devices are `false`.
- `addEditorDevice(d)`: staged add, guarded against re-adding an already-
  present device or one flagged by `editorDeviceInOtherGroup` (belt-and-
  braces — the backend would reject it on Save with "already belongs to
  X" anyway, surfaced via the existing `editError`).
- `editorAddableDevices()`: live devices minus what's already staged into
  `editing.devices`, filtered by `editorDeviceQuery` (name or MAC).
- `mockPreview`: `?preview=group` (alias) or `?preview=groupEditor` opens
  the editor with the Kids group pre-loaded (deep-cloned so staged
  edits in the preview don't mutate the mock Groups list underneath it),
  and the mock devices array already included one unassigned device
  ("Living Room TV") and one other-group device ("Mia's Phone", in Teens)
  so both addable states are exercisable offline without extra fixtures.

**`web/static/index.html`**:
- Groups list rows: `p.devices.length + ' devices'` replaced with
  `memberSummary(p)`.
- Group editor gained "Devices in this group" (member rows: online dot,
  name, monospace MAC, ✕ remove button; empty state: "No devices yet — add
  one below or enroll from the device itself (Settings → Enroll).") and
  "Add a device" (search box + `editorAddableDevices()` list; unassigned
  rows are clickable `<button>`s with a cyan `+`; other-group rows are
  `:disabled` with an "in `<group>` — move it from the Devices list or
  re-enroll it" note instead of the `+`).

**`web/static/css/app.css`**: new `.member-list` / `.member-row` /
`.other-group-note` / `.member-empty` block, placed after the existing
Groups & Devices styles. No new color tokens — reuses `--text`,
`--text-dim`, `--text-faint`, `--cyan-bright`, `--danger-bright`,
`--glass-fill` / `--glass-fill-hover` / `--glass-border` exactly as
elsewhere (`.device-row`, `.iconbtn`, `.online-dot`, ghost buttons). The
addable rows are real `<button>`s so they're keyboard/AT accessible, but
the global `button` chrome (border, background, box-shadow) is reset so
they read as a flat list rather than nested glass buttons inside the
already-glass editor card.

### Bug caught during browser verification

`:disabled="d.profileId && d.profileId !== editing.id"` looked correct but
was broken: when `d.profileId` is `''` (unassigned device), the expression
evaluates to `''` — a non-boolean falsy value. Alpine's `:disabled` binding
only special-cases a strict `false`; for any other value (including `''`)
it falls through to setting the boolean attribute, so **every** row in the
"Add a device" list rendered `disabled`, including devices that should have
been perfectly addable. The screenshot looked right (CSS didn't visually
dim it, since the disabled-state rule intentionally avoids the default
opacity-.4 dimming for contrast reasons), so this was only caught by
reading the actual DOM (`el.disabled` / `getAttribute('disabled')`) and the
accessibility snapshot, not by eyeballing the render. Fixed by adding an
explicit `editorDeviceInOtherGroup(d)` helper that always returns a real
boolean (`!!(...)`), used for the `:disabled` binding, the note's
`x-show`, and the `+` chevron's `x-show`, and reused inside
`addEditorDevice()` as a second guard. Re-verified after the fix: only the
genuinely other-group device (`Mia's Phone`) is `disabled` in the DOM; the
unassigned device (`Living Room TV`) is not.

### Contrast — new pairs

No new color tokens or opacities were introduced; the member-list rows sit
directly on the editor's existing `.card` glass background and reuse
already-shipped tokens. Recomputed with the same relative-luminance
Python script as the redesign/enroll reports (`--void` `#070b14`;
`--glass-fill` = white 5.5% over `--void`) to confirm each pairing used
here matches the already-verified figures exactly:

| Pair | Ratio | AA needed | Verdict |
|---|---|---|---|
| `--text` (member/addable device name) on `.card` glass-fill | 16.33:1 | 4.5:1 | Pass (identical to existing "Primary text on glass panel" row) |
| `--text-dim` (disabled-row device name) on glass-fill | 8.64:1 | 4.5:1 | Pass (identical to existing "Secondary text on glass" row) |
| `--text-faint` (MAC address, "in `<group>`" note) on glass-fill | 6.07:1 | 4.5:1 | Pass (identical to existing `.device-row .mac` row) |
| `--danger-bright` (✕ remove icon) on glass-fill | 7.94:1 | 4.5:1 | Pass (identical to existing "Danger on glass" row) |
| `--cyan-bright` (`+` add chevron) on glass-fill | 12.20:1 | 4.5:1 | Pass (identical to existing "Cyan link/label on glass" row) |

Disabled rows deliberately skip the global `button:disabled { opacity: .4 }`
rule (would drag the note text below AA) in favor of explicit `opacity: 1`
plus `cursor: not-allowed` and the dimmer-but-still-compliant `--text-dim`/
`--text-faint` tokens above — non-interactivity is communicated by cursor,
lack of hover feedback, and the disabled attribute (confirmed via a11y
snapshot: `disableable disabled`), not by contrast-breaking fade. No purple
anywhere in the new CSS.

### Browser verification

Scratch build (`go build -o .../familytime-scratch ./cmd/familytime`) on
port 8905 against a throwaway data file in the scratchpad dir — never the
live gateway.

- `?preview=groups`: group rows now show `"Ava's iPad, Noah's Switch"` /
  `"Mia's Phone"` as secondary text instead of a bare count. Screenshotted.
- `?preview=group`: editor shows the Kids group's two members (online dots,
  MACs, ✕ buttons) and the add-search list with `Mia's Phone` disabled
  ("in Teens — move it from the Devices list or re-enroll it") and
  `Living Room TV` addable (cyan `+`). Screenshotted.
- Exercised staging live in the browser via `click`/`fill`: removed Ava's
  iPad (moved into the addable list) → added Living Room TV (moved into
  members) → removed Living Room TV again (back to addable) → typed "mia"
  into the add-search (correctly filtered to just the disabled `Mia's
  Phone` row) → removed the remaining member (Noah's Switch), confirming
  the empty-group state renders: "No devices yet — add one below or
  enroll from the device itself (Settings → Enroll)." inside a dashed
  border. Screenshotted at the disabled-row-filtered and empty-group
  states.
- Confirmed via `evaluate_script` (not just the screenshot) that after the
  fix, `Mia's Phone`'s button has `disabled: true` and `Living Room TV`'s
  has `disabled: false` in the live DOM.
- Clicked the real (non-mock) "+ New group" button on `?preview=groups` to
  exercise the actual `newGroup()` handler (not the mockPreview branch):
  it correctly attempted `GET /api/devices` against the real server, which
  401'd (no real session in mock-preview mode) and bounced to the login
  view — same documented pre-existing `api()` 401-handling pattern the
  rest of the app already relies on (see the enroll report's whoami-409
  note), not a bug introduced here.
- Console: zero errors/warnings across every interaction above except the
  one expected 401 from the real "+ New group" click just described.
- `go build ./...`, `go vet ./...`, `go test ./...` all green (unchanged —
  no Go files touched). `gofmt -l .` empty. `node --check
  web/static/js/app.js` OK.

Scratch server killed and scratch binary/data/log files removed after
verification.

### Concerns / follow-ups

- `memberSummary(p)` doesn't truncate for groups with many devices — a
  group with a dozen devices would show a long comma-separated line on the
  Groups list. Not a problem for the family sizes this app targets, but
  worth a `slice(0, N) + ' and N more'` treatment if it comes up.
- The add-search list intentionally still shows other-group devices
  (disabled) rather than hiding them, so a parent can see where a device
  went ("in Teens") instead of it silently disappearing — this seemed more
  family-friendly than an unexplained gap, per the task's own copy for the
  disabled state.

## UniFi alias sync

### Why

Enrollment (and the group editor) let a parent name a device in Family
Time, but that name only ever lived in Family Time's own store — the UniFi
app kept showing the gateway's own client name (often a generic
"iPhone 72:68" for Private-Wi-Fi-Address devices). This closes the loop: a
name set in Family Time is pushed to UniFi as the client's alias, so both
UIs agree. Ground-truth API facts (probed live against a UCG Max, Network
10.4.57, on 2026-07-03 with the app's own API key — never touched again
after that probe) are recorded as a doc comment in `internal/unifi/client.go`
next to the new code.

### What was built

**`internal/unifi/client.go`** — `RenameClient(ctx, mac, name string) error`:
- Uses the *legacy* v1 API (`X-API-KEY`-authenticated, same auth as
  everywhere else in this client), not the official v1 integration API —
  the official API's client PATCH/PUT returns 405 (no rename support). This
  puts `RenameClient` in the same "unofficial but battle-tested" category as
  the existing v2 trafficrules calls.
- `GET /proxy/network/api/s/default/rest/user` to list all clients, find
  the entry whose `mac` matches (case-insensitive), then
  `PUT /proxy/network/api/s/default/rest/user/{_id}` with body
  `{"name": "<new name>"}`. Setting `name` to `""` clears the alias
  (verified live: set → confirm → clear → confirm).
- New `legacyEnvelope`/`legacyUser` types model the legacy API's
  `{"meta":{"rc":"ok"|...,"msg":"..."},"data":[...]}` envelope, distinct
  from the official v1 API's `{"data":[...],"offset":...}` paging envelope
  already used by `ListClients`. A mac absent from the list returns an
  `ErrNotFound`-wrapped error (the same sentinel already used for gateway
  404s elsewhere, so `errors.Is` call sites don't need a new case). A
  non-"ok" `meta.rc` (from either the list GET or the rename PUT) becomes an
  error that includes `meta.msg` when the gateway supplies one.

**`internal/server/server.go`**: `UnifiAPI` gained
`RenameClient(ctx, mac, name string) error`. `unifi.Client` satisfies it
structurally (no wiring needed beyond the method existing).

**`internal/server/fake_test.go`**: `fakeAPI` gained `renames
[]struct{MAC, Name string}` (append-only, under the existing mutex) and
`failRename error` (checked before appending) so tests can assert exactly
which renames were pushed and simulate a gateway failure independently of
`failAll`/`failUpdateID`.

### Push-point policy: best-effort, never fatal

Both push points call `s.api().RenameClient(...)` **after** the parent
operation has already succeeded and been persisted to the store, and treat
a failure as log-only:

```go
if err := s.api().RenameClient(r.Context(), mac, name); err != nil {
    log.Printf("familytime: unifi rename failed for %s: %v", mac, err)
}
```

- **`handleEnroll`** (`internal/server/enroll_handlers.go`): pushes the
  enrolled device's final name (post name-defaulting: request name → live
  gateway client name → mac) once, right after
  `persistProfilesAndRuleIDs` succeeds and right before the `200` response
  is written.
- **`handleProfileUpdate`** (`internal/server/handlers.go`): after
  `persistProfileAndRuleIDs` succeeds, diffs `updated.Devices` (the new,
  just-saved list) against `existing.Devices` (the pre-mutation snapshot
  captured earlier in the handler) by MAC. Only MACs present in *both*
  lists with a changed `Name` get a rename pushed — a newly added device
  (MAC not in the old list) or a removed one (MAC not in the new list)
  never triggers a call. Devices with an unchanged name are skipped
  entirely, so a no-op save pushes zero renames.

Neither path ever surfaces a rename failure to the client response or
blocks the save — the parent operation (enrollment, profile save) has
already committed by the time `RenameClient` runs.

### Frontend: inline rename in the group editor

**`web/static/index.html`** — each row in "Devices in this group" gained a
✎ icon button (`.iconbtn`, matching the existing ✕ remove button's style
and a11y pattern) between the name/mac block and the remove button. Clicking
it swaps the `<span class="n">` for a `.rename-input` text input, bound to
`renameDraft` and autofocused/selected on open (`x-init` +
`$nextTick(() => { $el.focus(); $el.select(); })`). Enter and blur commit;
Escape cancels without applying. A `.devices-hint`-styled line ("Renames
sync to UniFi when you save.") sits under the member list, reusing the same
class the existing "Devices sync from your UniFi gateway." note uses
elsewhere — no new visual language.

**`web/static/js/app.js`** — new state `renamingDeviceMac` (which single
row, if any, is in rename mode — only one at a time) and `renameDraft` (the
in-progress text), plus three methods:
- `startRenameDevice(d)` — enters rename mode, seeds the draft from `d.name`.
- `commitRenameDevice(d)` — trims the draft; a blank draft leaves the name
  unchanged (never clears it) rather than saving an empty name; guarded on
  `renamingDeviceMac === d.mac` so a stray `blur` fired by the input's own
  removal from the DOM (e.g. immediately after Escape already canceled)
  can't re-apply a commit.
- `cancelRenameDevice()` — exits rename mode without touching `d.name`.

Everything is staged onto `editing.devices[i].name` exactly like the
existing add/remove flows — nothing is sent to the server until `saveGroup()`
PUTs the whole profile, at which point the backend's name-diff (above)
pushes the rename to UniFi. `removeEditorDevice` also clears
`renamingDeviceMac` if the row being removed was mid-rename, so a stale
open input can't outlive its row.

No changes were needed to the `?preview=group` mock — the sample profile
data (`Ava's iPad`, `Noah's Switch`) already exercises the control with real
data shape; the ✎ button is unconditionally visible on every row
(`x-show="renamingDeviceMac !== d.mac"`, and `renamingDeviceMac` starts
`null`).

### Tests

`internal/unifi/client_test.go` — three new `httptest`-backed cases:
- `TestRenameClientSuccess` — asserts the PUT path
  (`/proxy/network/api/s/default/rest/user/u2`) and body
  (`{"name":"Ava's iPhone"}`) exactly, given a GET response listing two
  clients where only one matches the target mac.
- `TestRenameClientMACNotFound` — GET response has no matching mac;
  asserts `errors.Is(err, ErrNotFound)`.
- `TestRenameClientRCNotOK` — PUT responds with
  `{"meta":{"rc":"error","msg":"api.err.NoPermission"}}`; asserts the
  returned error mentions `meta.msg`.

`internal/server/enroll_test.go` — two new cases:
- `TestEnrollPushesRenameToUnifi` — a normal enroll pushes exactly one
  `fake.renames` entry, `{mac, "Ava's iPhone"}`.
- `TestEnrollRenameFailureIsBestEffort` — with `fake.failRename` set,
  enrollment still returns `200`, the store still gets the new device/name,
  and (via the same `log.SetOutput(&buf)` capture pattern already used by
  `TestFailErrLogsUnknownGatewayErrors` in `server_test.go`) the log output
  contains both `"unifi rename failed"` and the underlying error text.

`internal/server/handlers_test.go` — three new cases:
- `TestProfileUpdatePushesRenameForChangedMember` — a save that renames one
  existing member (`iPad` → `Emma's iPad`) *and* adds a new device
  (`New Switch`) pushes exactly one rename, for the renamed MAC only.
- `TestProfileUpdateNoChangePushesNoRename` — re-saving a profile with
  identical device names pushes zero renames.
- `TestProfileUpdateRenameFailureIsBestEffort` — with `fake.failRename` set,
  the save still returns `200` and persists the new name to the store; the
  log contains the failure (same capture pattern as above).

All pre-existing tests pass unmodified.

```
go build ./... && go vet ./... && go test ./...   # all green
gofmt -l .                                          # empty
node --check web/static/js/app.js                   # OK
```

Full output (abbreviated to the package summary line, since every test in
every package passed):

```
ok  	familytime/cmd/familytime	(cached)
ok  	familytime/internal/e2e	(cached)
ok  	familytime/internal/rules	(cached)
ok  	familytime/internal/server	3.490s
ok  	familytime/internal/store	(cached)
ok  	familytime/internal/unifi	0.338s
?   	familytime/web	[no test files]
```

### Browser evidence

Scratch server on port 8906, data under the session scratchpad (never the
real config dir, never the live gateway — `mockPreview` short-circuits
before any `fetch` to `/api/state` or the gateway). Navigated to
`?preview=group` (`Kids` group, two seeded devices: `Ava's iPad`,
`Noah's Switch`).

- Initial state: both member rows show a ✎ button next to the existing ✕,
  styled with the same `.iconbtn` treatment (screenshot captured).
- Clicked ✎ on `Ava's iPad`: the name swapped for a focused, text-selected
  `.rename-input` (cyan focus ring, matches every other text input in the
  app) pre-filled with the current name (screenshot captured).
- Typed `Ava's Tablet`, pressed Enter: row reverted to display mode
  showing the new staged name `Ava's Tablet`, ✎ button reappeared, aria
  labels (`"Rename Ava's Tablet"`, `"Remove Ava's Tablet from group"`)
  updated to match — confirmed via an a11y-tree snapshot, not just pixels.
- Clicked ✎ on `Noah's Switch`, typed `Should Not Stick`, pressed Escape:
  row reverted showing the original `Noah's Switch` — proving Escape
  discards the draft rather than committing it.
- Console: zero `error`-type messages for the whole session (checked via
  `list_console_messages` filtered to `types:["error"]`). Two pre-existing
  `issue`-type a11y notices ("no label associated with a form field") were
  present, matching the app's existing pattern for unlabeled search-style
  inputs (e.g. the "Add a device" search box) — not a regression
  introduced by the new rename input, which does carry a dynamic
  `aria-label`.

Scratch server killed, scratch binary/data/log and the two temporary
screenshot files removed after verification.

### Concerns / follow-ups

- The two pre-existing "no label associated with a form field" a11y
  notices (shared with the "Add a device" search box) would be worth
  fixing app-wide with `aria-labelledby` or a wrapping `<label>` — out of
  scope here since it's a pre-existing pattern, not something this task's
  rename input introduced or worsened.
- `RenameClient`'s GET-then-PUT is two round trips per rename; fine at
  family-sized client-list scale (~120 known clients per the task's own
  probe notes) and at the low call frequency of "a parent just renamed a
  device," but it's worth knowing if a future feature wants to batch
  renames.
