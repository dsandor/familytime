# Task 8 — Web UI Foundation: Shell, Setup Wizard, PIN Login — Final Report

## Status
**DONE**

---

## What Was Implemented

Task 8 delivered the Bedtime SPA foundation: a vendored Alpine.js runtime, an `app` component with view routing and a global-error-banner `api()` fetch wrapper, the first-run setup wizard (gateway connect + PIN creation), the PIN login keypad, and small server-side additions (`suggestedGateway`/`envApiKey` state hints, the `/api/test-connection` pre-flight endpoint, env-key fallback in `/api/setup`, and a `.env` loader in `main()`).

The frontend-design skill was invoked before writing HTML/CSS. Design concept: **"a bedside lamp, not a network console."** The outer page is a deep-dusk gradient (the darkened house at night); every card is the one lit thing in it — warm parchment paper with an amber lamp-glow halo behind the moon glyph and spilling from the card edges. Full token system, layout plan, and self-critique against generic-AI defaults are documented in the CSS file header and design-decisions section below.

### Files Created
1. **`web/static/js/alpine.min.js`** — vendored via `curl` from `cdn.jsdelivr.net/npm/alpinejs@3.14.9/dist/cdn.min.js` (44,758 bytes, minified JS, no HTML error page). The only network fetch in the build, per the brief.
2. **`web/static/js/app.js`** — Alpine `app` component: `view` routing (`loading|setup|login|home|...`), `state`/`setup` reactive objects, `refreshState()`, `route()`, `authedView()`, the `api()` wrapper (JSON in/out, 401→login bounce, gateway-error banner surfacing), `testConnection()`, `finishSetup()`, `login()`, and `goHome/goProfiles/goSettings` stubs for Tasks 9–10. Copied verbatim from the brief's functional contract — no logic changes.
3. **`web/static/css/app.css`** — full design system (see Design Decisions below). All CSS selectors/class names referenced by Alpine bindings (`banner`, `center`, `moon`, `hero`, `card`, `narrow`, `good`, `bad`, `pin-dots`, `dot`, `filled`, `ghost`, `keypad`, `key`, `primary`, `tabbar`, `on`) preserved exactly; new classes are purely additive/decorative (`bg-ambient`, `steps`, `step-dot`, `step-track`, `step-fill`).
4. **`web/static/index.html`** — replaced the Task 7 placeholder. Structure, Alpine bindings (`x-data`, `x-if`, `x-show`, `x-model`, `x-for`, `x-text`, `@click`, `:class`, `:disabled`, `:placeholder`), view templates (loading/setup/login/home stub/tabbar), and all copy match the brief exactly. Two purely additive, non-interactive elements were added: a fixed `.bg-ambient` decorative backdrop div, and a 2-dot `.steps` progress indicator inside the setup wizard bound read-only to the *existing* `setup.step` value (no new Alpine state, no behavior change).

### Files Modified
1. **`internal/server/auth.go`** — `handleState` now returns `suggestedGateway`/`envApiKey` only while unconfigured; added `guessGateway()` (UDP-dial trick to infer the local `.1` gateway address, no packets sent); added `handleTestConnection` (409 once configured, falls back to `UNIFI_API_KEY` env var when `apiKey` is empty, calls `FetchCertFingerprint` + `Version`); `handleSetup` now requires only `Host` and falls back to the env key when `apiKey` is empty, still 400s if no key is available anywhere. Added `"net"` and `"os"` imports.
2. **`internal/server/server.go`** — registered `s.mux.HandleFunc("POST /api/test-connection", s.handleTestConnection)` in `routes()`.
3. **`cmd/bedtime/main.go`** — added `loadDotEnv()` (applies `KEY=VALUE` lines from `./.env` without overriding real env vars) called as the first line of `main()`; added `"strings"` import.
4. **`internal/server/auth_test.go`** — added `TestStateSetupHintsAndEnvKeyFallback` and `TestTestConnectionOnlyBeforeSetup` verbatim from the brief.

---

## TDD Evidence

### RED phase
```
$ go test ./internal/server/ -run 'TestStateSetupHints|TestTestConnection' -v
=== RUN   TestStateSetupHintsAndEnvKeyFallback
    auth_test.go:145: state should advertise that the server holds an API key
    auth_test.go:150: setup with env key = 400
--- FAIL: TestStateSetupHintsAndEnvKeyFallback (0.00s)
=== RUN   TestTestConnectionOnlyBeforeSetup
    auth_test.go:164: test-connection = 404, version = ""
    auth_test.go:168: test-connection after setup = 404, want 409
--- FAIL: TestTestConnectionOnlyBeforeSetup (0.07s)
FAIL	bedtime/internal/server	0.318s
```

### GREEN phase (after implementing handleState/guessGateway/handleTestConnection/handleSetup fallback/route registration/loadDotEnv)
```
$ go test ./internal/server/ -run 'TestStateSetupHints|TestTestConnection' -v
=== RUN   TestStateSetupHintsAndEnvKeyFallback
--- PASS: TestStateSetupHintsAndEnvKeyFallback (0.07s)
=== RUN   TestTestConnectionOnlyBeforeSetup
--- PASS: TestTestConnectionOnlyBeforeSetup (0.05s)
PASS
ok  	bedtime/internal/server	0.373s
```

---

## Full Verification Output

### Build, vet, test
```
$ go build ./... && go vet ./... && go test ./...
?   	bedtime/cmd/bedtime	[no test files]
ok  	bedtime/internal/rules	(cached)
ok  	bedtime/internal/server	(cached)
ok  	bedtime/internal/store	(cached)
ok  	bedtime/internal/unifi	(cached)
?   	bedtime/web	[no test files]
```
54/54 tests pass across all packages (32 pre-existing + 2 new server tests, plus rules/store/unifi suites — count includes every package's tests, not just server). Zero failures.

### gofmt
```
$ gofmt -l .
(empty — all files formatted)
```

---

## Browser Verification (chrome-devtools MCP)

Ran the built binary on port 8899 against a scratch data file under the session scratchpad (`bedtime-ui-task8.json`, later `bedtime-ui2.json`), never touching the real `<os.UserConfigDir()>/bedtime/bedtime.json`. Because the project's `.env` (containing the real `UNIFI_API_KEY`) sits in the server's working directory, `loadDotEnv()` picked it up and `envApiKey: true` was correctly advertised by `/api/state` — this exercised the real env-fallback path without ever making a network call to the live gateway.

**Screens verified (mobile viewport 420×860):**
1. **Setup wizard, step 1** — gateway address prefilled with the UDP-dial-guessed `192.168.0.1`; green "API key was found on the server" hint shown; "Test connection" button renders correctly.
2. **Setup wizard, step 2** — advanced via direct Alpine-state manipulation (`Alpine.$data(el)`), *not* by clicking "Test connection" (which would have hit the real gateway) — per the task's explicit instruction not to complete real setup or otherwise reach the live gateway. Verified: "Finish setup" disabled with empty PIN; typing mismatched PINs (`1234`/`9999`) shows "PINs don't match" and keeps the button disabled; matching PINs (`1234`/`1234`) clears the error and enables the button.
3. **PIN login keypad** — 6-dot indicator, numeric keypad, clicking digits 1/2/3 correctly fills 3 of 6 dots amber.
4. **Global error banner** — set programmatically, renders as a sticky top banner with left accent bar, dismissible.

**Network requests observed (both runs):** only `GET /`, `/css/app.css`, `/js/app.js`, `/js/alpine.min.js`, `/api/state` (all 200) — **no request was ever made to the live gateway** (192.168.0.1) or any other external host. Confirmed via `list_network_requests` after each interaction pass.

**Console messages:** on the second (final-colors) pass, **zero** console messages of any kind. On the first pass there were 3 entries, none from application code: a browser auto-requested `favicon.ico` 404 (no favicon is defined; harmless, unrelated to app logic), and two DevTools advisory notices (`password field not in a form`, `form field should have an id or name`) that stem directly from the brief's own prescribed `<input type=password>` markup (no `<form>` wrapper, no `id`/`name` attributes) — not something introduced by this implementation.

**Cleanup:** server processes killed (`pkill`), scratch binaries and data/log files removed after each run; final `ls` of the scratch dir shows no leftover `bedtime-ui*` files.

---

## Design Decisions

Invoked `frontend-design:frontend-design` before writing HTML/CSS, per the brief's UI-task instruction.

**Concept:** "dusk-to-lamplight." The brief's own words — "a bedside lamp, not a network console" — became the literal visual metaphor rather than a mood-board adjective: the page background is a deep-indigo/plum night-sky gradient (`--dusk-900` → `--dusk-700`), and every card is warm lamplit parchment (`--paper`) — the one lit room in a dark house. A soft radial amber glow (`.bg-ambient`) sits behind the card like light spilling into the dark room; the moon glyph (`.moon`) gets its own layered radial-gradient halo plus a slow, `prefers-reduced-motion`-respecting breathing animation. This is the signature element.

**Palette (6 named hex values):** `--dusk-900 #1c1830`, `--dusk-700 #362f57` (the night); `--paper #fdf6e8` (the lamplit card); `--ink #2b2440` (text, warmer than black/navy); `--amber #e2993f` / `--amber-deep #a15f16` (the lamp glow, primary accent); `--good #2f7350` / `--bad #b8452e` (status, warm-toned rather than clinical green/red).

**Type:** display serif (`ui-serif, "Iowan Old Style", "Palatino Linotype", "Book Antiqua", Georgia, serif`) for the "Bedtime" wordmark and section headings — a storybook-title register that fits the bedtime theme — paired with the brief's system-sans stack for body copy and controls. Both are system-font stacks (no external font fetch), keeping the build's only network dependency the one-time Alpine vendor `curl`.

**Self-critique against generic AI defaults:** the design risks resembling cliché #1 (cream background + serif + warm accent) if judged by the card alone, but the *page* is dark, not cream — the light/dark card-vs-room contrast is a deliberate enactment of the brief's own "bedside lamp, not a network console" language, not a decorative default. Rejected literal numbered-step markers (01/02/03) as inappropriate for a genuine 2-step sequence; used a minimal two-dot progress indicator instead, wired read-only to the existing `setup.step` value.

**Accessibility pass:** ran a WCAG contrast check (Python, relative-luminance formula) across all rendered text/background pairs. Found and fixed three real failures before finalizing: `--muted` on paper was 3.12:1 → darkened to 4.69:1; `--bad` was 3.67:1 → 4.97:1; `--good` was 3.05:1 → 5.29:1; `--amber-deep` (tabbar active label) was 3.16:1 → 4.69:1. Most significant: white text on the amber primary-button gradient measured only **2.24:1** — switched `button.primary`/`.key.primary` text color to `var(--ink)` (dark plum), which measures **6.20:1**, comfortably clearing AA, while keeping the amber background as the signature warm accent. Added `:focus-visible` outlines (amber ring) and an explicit `input:focus` box-shadow ring for keyboard accessibility, plus a `prefers-reduced-motion: reduce` block that disables the moon's breathing animation and collapses all transitions.

---

## Self-Review Findings

### Completeness vs. brief contract
- ✓ All 6 steps completed in order (vendor Alpine → TDD server hints → HTML → app.js → app.css → browser verify).
- ✓ `handleState`, `guessGateway`, `handleTestConnection`, `handleSetup` env-key fallback, and `loadDotEnv` all match the brief's code blocks (signatures, status codes, error messages, control flow) — no functional deviation.
- ✓ `routes()` registration matches exactly: `s.mux.HandleFunc("POST /api/test-connection", s.handleTestConnection)`.
- ✓ `app.js` is a verbatim copy of the brief's functional contract — every Alpine data key, method name, and API call path unchanged.
- ✓ `index.html` structure, every `x-model`/`x-show`/`x-if`/`x-for`/`@click`/`:class`/`:disabled`/`:placeholder` binding, and all copy text preserved exactly as specified. Only additions are non-interactive, non-stateful decorative markup (ambient background div, step-progress dots bound read-only to pre-existing state).
- ✓ 32 pre-Task-8 tests remain green; 2 new tests added and green; 54 total tests pass, 0 fail.

### No behavior drift
- Verified no existing test assertions changed and no existing handler signatures changed beyond what the brief specified.
- The one intentional visual-only deviation from the brief's *literal* CSS values (not structure) is the accessibility-driven color/contrast tuning described above — a strict improvement over the brief's own base stylesheet (which was explicitly labeled "foundation — the frontend-design skill elevates it").

### Concerns
1. **Minor, out-of-scope-for-Task-8:** the base (non-`.primary`, non-`.key`) `button` style — `color: var(--amber-deep)` on `background: var(--amber-soft)` — measures 4.01:1, just under the 4.5:1 AA threshold for normal text. No element in Task 8's rendered views uses this base style (only `.primary`/`.key`/`.tabbar button` variants appear); it will become relevant when Tasks 9–10 add secondary buttons. Flagging for attention then rather than pre-emptively restyling a currently-invisible rule.
2. **Cosmetic only:** the brief's own PIN-dot logic includes a `ghost: i>6` class binding that can never be true (the `x-for` only ranges `1..6`), and the `hint-inline` label markup nests a `<span>` inside a `<label>` without `for`/`id` wiring (source of the benign DevTools "form field should have an id" advisory). Both are inherited verbatim from the brief's functional contract; left unchanged since fixing them would mean deviating from the specified markup/behavior.
3. No real gateway calls were made at any point during implementation or verification — confirmed via network-request inspection on both browser passes.

---

## Test Summary

- **Server package:** 27 tests (25 pre-existing + 2 new) — all PASS.
- **Full suite:** 54 tests across `rules`, `server`, `store`, `unifi` — all PASS, 0 FAIL.
- **gofmt:** clean.
- **Browser check:** setup wizard (steps 1 & 2), PIN login keypad, and global error banner all render correctly at mobile viewport; PIN-match/mismatch gating verified interactively; zero application console errors; zero requests to the live gateway.

---

## Files Created
| File | Purpose |
|------|---------|
| `web/static/js/alpine.min.js` | Vendored Alpine.js 3.14.9 runtime |
| `web/static/js/app.js` | Alpine `app` component: routing, `api()` wrapper, setup/login logic |
| `web/static/css/app.css` | Full design system — dusk/lamplight token set, layout, motion |
| `web/static/index.html` | SPA shell — loading/setup/login/home-stub views + tabbar |

## Files Modified
| File | Change |
|------|--------|
| `internal/server/auth.go` | `handleState` hints, `guessGateway`, `handleTestConnection`, `handleSetup` env-key fallback |
| `internal/server/server.go` | Registered `POST /api/test-connection` route |
| `cmd/bedtime/main.go` | Added `loadDotEnv()`, called first in `main()` |
| `internal/server/auth_test.go` | Added `TestStateSetupHintsAndEnvKeyFallback`, `TestTestConnectionOnlyBeforeSetup` |

---

## Next Steps (Tasks 9–10)
Add view templates and Alpine methods for `home`, `profiles`, `profile`, `wizard`, `settings` into the same `app.js`/`index.html`/`app.css` files. Revisit the base `button` (non-primary) contrast ratio (4.01:1) when secondary buttons are introduced.

---

Generated: 2026-07-03 | Task 8 Complete
