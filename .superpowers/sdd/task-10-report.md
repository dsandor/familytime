# Task 10 Report — Web UI: rules list, add-rule wizard, settings

## Status: DONE

## What was implemented

Replaced the Task 10 `view==='rules'` placeholder with the full Rules list, Add-rule
wizard (3 steps), and Settings screens, wiring all required Alpine state/methods
exactly per the brief's functional contract (markup/bindings/method bodies copied
verbatim; only CSS was translated into the established Task 8/9 design language and
extended). Also applied the three carry-over fixes from the Task 9 review.

### Files changed

- `/Users/dsandor/Projects/bedtime/web/static/index.html`
  - Replaced the `view==='rules'` placeholder (`<main class="card"><p>Rules — Task
    10</p>...`) with the brief's three templates verbatim: `view==='rules'` (rule
    list with toggle switch + delete), `view==='wizard'` (3-step: what/when/name),
    `view==='settings'` (gateway form, PIN form, data-path + lock).
  - No other markup changes; tab bar bindings (`goHome`/`goProfiles`/`goSettings`)
    untouched, per the carry-over instruction being scoped to `authedView()` in
    JS, not the tab "on" highlighting logic.

- `/Users/dsandor/Projects/bedtime/web/static/js/app.js`
  - Added state: `gwForm: { host: '', apiKey: '', error: '' }`,
    `pinForm: { current: '', next: '', error: '', ok: '' }`, `whenChoices` array —
    verbatim from the brief.
  - Replaced the Task 9 `goRules` stub with the full method block, in the brief's
    exact order: `profileName`, `goRules`, `ruleSummary`, `toggleRule`,
    `deleteRule`, `startWizard`, `wizardWhatValid`, `pickWhen`, `toggleDay`,
    `wizardWhenValid`, `wizardWhat`, `wizardWhen`, `whatText`, `suggestName`,
    `wizardRecap`, `saveRule`, `goSettings`, `saveGateway`, `trustCert`,
    `savePin`, `logout`.
  - Removed the Task 8 `goSettings` stub at the bottom of the file (superseded by
    the real implementation above).
  - **Carry-over fix #1:** `authedView()` now includes `'rules'` (was
    `['home','profiles','profile','wizard','settings']`, now also has `'rules'`)
    — the bottom tab bar no longer disappears on the Rules view.
  - Extended `mockPreview(view)`: seeds `this.rulesProfileId='p1'`,
    `this.presets`/`this.categories` (subset mirroring the real preset/category
    catalog in `internal/rules/presets.go` so `ruleSummary()`/`whatText()` resolve
    real names), two sample `ruleView` objects (`r1` enabled+active "No YouTube on
    school nights", `r2` disabled "No gaming on weekends"), and
    `this.settings = { host: '192.168.0.1', siteName: 'default', dataPath:
    '/example/bedtime.json' }`. For `?preview=wizard`, calls `startWizard()`
    before setting `this.view = view` last, as instructed.

- `/Users/dsandor/Projects/bedtime/web/static/css/app.css`
  - **Carry-over fix #2:** lightened `button.ghost:hover` fill from
    `rgba(226,153,63,.1)` → `.04`, and `.ghost.danger:hover` from
    `rgba(184,69,46,.08)` → `.06`. Verified via a manual sRGB/WCAG contrast
    calculation (script discarded after use): ghost hover was 4.36:1 → now
    4.55:1; danger hover was 4.45:1 → now 4.58:1. Both clear AA 4.5:1 with margin.
  - **Carry-over fix #3:** added `.chips button.on:hover:not(:disabled) {
    background: var(--amber); }` — at equal-or-higher specificity than the
    generic `.chips button:hover:not(:disabled)` rule it was previously losing
    to, so a selected chip now keeps its amber fill on hover. Verified live
    (hovered the selected "School nights" chip — stayed amber, screenshot below).
  - Appended the brief's rules/wizard/settings styles, translated from
    `--card`/`--accent`/`--accent-soft` (not part of this app's token set) into
    the established `--paper`/`--amber`/`--amber-soft` tokens, with contrast
    fixes discovered during translation (see Design decisions below):
    `.pagehead.withback`, `.back` (+ dark-bg and `.card`-nested light-bg
    variants), `.blocking`, `.iconbtn` (+ `.danger` variant), `.preset-grid` (+
    `.on`/hover states), `.pemoji`, `textarea`, `.times`/`input[type=time]`, the
    `.switch` toggle, and `code`.

## Design decisions

The brief's literal CSS, translated token-for-token, would have introduced new
sub-AA contrast failures (the same class of bug as carry-over fixes #2/#3, just in
new code). I checked each new interactive/text pairing with a manual WCAG
luminance-contrast calculation before committing to it, rather than translating
blind:

- **`.preset-grid button.on`**: the brief only restyles `border-color`/`background`
  on select, leaving the base button's `color: var(--amber-deep)` text on top of
  `var(--accent-soft)`. Mapped literally (`--amber-deep` on `--amber-soft`) that's
  ~4.0:1 — fails. Added `color: var(--ink)` on `.on` (mirrors how `.chips
  button.on` already solves this exact problem elsewhere) — ink-on-amber-soft
  measures ~11.7:1.
- **`.preset-grid button` (unselected)**: mapping `var(--card)` → `--paper-soft`
  gave `--amber-deep` on `--paper-soft` ~4.2:1 — fails. Used plain `--paper`
  instead (matches ~4.7:1, already the established ghost-button contrast) and let
  the existing `1.5px solid var(--line)` border provide tile definition instead of
  a background tint.
- **`.preset-grid button:hover` / `.iconbtn:hover`**: with no explicit override,
  these would fall back to the global `button:hover { background: #f1d9a9; }`,
  which measures ~3.7:1 against `--amber-deep` text — fails. Gave both an explicit
  `rgba(226,153,63,.04)` hover fill (same value validated for the ghost-button
  fix), landing at ~4.55:1.
- **`.back` on the dark backdrop (Rules view)**: the brief's `color: var(--accent)`
  doesn't map to a single existing token that works in both contexts the brief's
  own markup puts it in. Used `--amber` (7.25:1 / 5.21:1 against the two gradient
  stops) for the dark-backdrop case (Rules), and added `.card .back { color:
  var(--amber-deep); }` for the wizard, whose `pagehead.withback` sits inside
  `<main class="card">` (a paper background) — `--amber` on paper only measures
  ~2.2:1, so the light-bg variant needed the darker token (~4.7:1).
  - Also added `.card .pagehead h1 { color: var(--ink); text-shadow: none; }` and
    `.card .pagehead .sub { color: var(--muted); }`, mirroring the pre-existing
    `.card .hero h1` override — the brief's `.pagehead h1` CSS is white-with-
    shadow (correct for the dark backdrop the Rules/Settings/Home headers sit on)
    but the wizard's header is nested in a paper card, where white text would be
    unreadable.
  - `.back:hover` on the dark backdrop: lightening the hover fill (as ghost
    buttons do on paper) would push the background *toward* amber's own
    lightness and erode contrast, since amber is the lighter of the two colors
    here — verified this numerically (white-tint hover dropped to ~3.6–4.3:1
    depending on gradient position). Used a darkening `rgba(0,0,0,.15)` overlay
    instead, which only ever widens the gap (verified 7.6:1 / 5.8:1 against the
    gradient's two stops).
- **Toggle switch**: hid the native checkbox with `position:absolute; opacity:0`
  instead of the brief's `display:none`, and added `:focus-visible + .slider`
  outline — keeps the control keyboard-focusable and gives it a visible focus
  ring, rather than removing it from the accessibility tree entirely. Bindings
  (`:checked`, `@change="toggleRule(r)"`) are unchanged.
- Minor polish, no contrast implications: `code` (data-file path) got a
  `--paper-soft` chip background for legibility, matching how inputs already sit
  a shade off the card.

## Browser verification (chrome-devtools MCP, port 8899, scratch data file)

Built `/private/tmp/.../scratchpad/bedtime-ui`, ran with
`-data ./bedtime-ui-scratch.json` on port 8899. Never touched the live gateway.

- **`?preview=rules`** — both sample rules render (`ruleSummary()` output correct:
  "YouTube — Sun Mon Tue Wed Thu · 20:00–07:00", "Gaming — Fri Sat · 21:00–09:00"),
  "⛔ blocking right now" shown only on the active rule, toggle switches reflect
  `enabled` state (green/off), delete icon present. **Tab bar visible** (carry-over
  fix #1 confirmed — previously absent on this view). Zero console messages of any
  kind.
- **`?preview=wizard`** — full 3-step walkthrough:
  - Step 1: clicked the YouTube preset tile — selected state renders with amber
    border/fill and ink text (readable, matches the contrast fix above); "Next →"
    became enabled per `wizardWhatValid()`.
  - Step 2: loaded with `startWizard()`'s default `whenChoice:'school'` already
    selected (Sun–Thu highlighted, 20:00–07:00, overnight hint shown since
    start > end); explicitly clicked "School nights" chip and hovered it — stayed
    amber on hover (carry-over fix #3 confirmed live); advanced via "Next →".
  - Step 3 ("Name it"): suggested name **"No YouTube on school nights"** appeared
    automatically (from `suggestName()`, set on the "Next →" click per the
    brief), recap text read "Blocks YouTube for Emma, Sun, Mon, Tue, Wed, Thu from
    20:00 to 07:00." — matches `wizardRecap()` exactly.
  - Zero console errors at every step.
- **`?preview=settings`** — gateway section (host/site line, address + API-key
  fields, Update/Trust buttons, hint), Parent PIN section (current/new fields,
  Change PIN), and data-file/lock section all render; `code` chip shows
  `/example/bedtime.json`. Zero console messages.
- Checked `list_console_messages` filtered to `types:["error"]` explicitly on
  every view — empty everywhere.
- One non-error DevTools "Issues" advisory appeared only on `?preview=wizard`
  ("no label associated with a form field" / "missing id/name", count 3–4) for
  the textarea/time/text inputs. Investigated via `evaluate_script`: every one of
  those fields *is* correctly wrapped in a `<label>` (implicit association,
  matches the brief's markup exactly). Confirmed this is a pre-existing pattern,
  not a Task 10 regression: navigating to the untouched Task 8 setup view (`/`
  with a fresh data file) produces the identical advisory category
  ("A form field element should have an id or name attribute", count 4) on
  inputs I never touched. Chrome's Issues panel evaluates all DOM form fields for
  autofill-style id/name association regardless of implicit label wrapping; it's
  informational, not a console error, and consistent across the whole app.
- Re-checked at a 375×700 mobile viewport (`?preview=wizard`) — preset grid,
  chips, and buttons reflow correctly, no horizontal overflow, zero console
  errors.
- Killed the scratch server, removed the scratch binary/data file/log and the
  scratch contrast-calculation script; scratchpad directory is empty afterward.

## Full-suite verification

```
go build ./... && go vet ./... && go test ./...
```
All packages built and passed (`bedtime/internal/rules`, `bedtime/internal/server`,
`bedtime/internal/store`, `bedtime/internal/unifi` all `ok`; `cmd/bedtime` and `web`
have no test files — 54 pre-existing Go tests unaffected, this task only touched
`web/static/*`).

```
gofmt -l .
```
Empty output (no Go files touched this task).

```
node --check web/static/js/app.js
```
Syntax OK. Also verified `index.html`'s new templates have balanced
template/main/header/section/div/label/button tags.

## Self-review

- Every method named in the brief is present with the brief's exact body:
  `profileName`, `goRules`, `ruleSummary`, `toggleRule`, `deleteRule`,
  `startWizard`, `wizardWhatValid`, `pickWhen`, `toggleDay`, `wizardWhenValid`,
  `wizardWhat`, `wizardWhen`, `whatText`, `suggestName`, `wizardRecap`,
  `saveRule`, `goSettings`, `saveGateway`, `trustCert`, `savePin`, `logout`, plus
  state (`gwForm`, `pinForm`, `whenChoices`) — confirmed via a full re-read of the
  final `app.js` against the brief text and a grep over all top-level method
  definitions.
- Markup for the three new views is byte-for-byte the brief's contract (no added
  or removed bindings/classes beyond what's documented under Design decisions,
  which are additive CSS-only or state-seeding changes, never a change to an
  Alpine directive or method signature).
- All three carry-over fixes applied and verified live in-browser (not just
  read-verified): tab bar visible on Rules, ghost/danger hover contrast
  recalculated and passing, `.chips button.on` hover-preserves confirmed by
  hovering the selected chip mid-wizard-walkthrough.
- New buttons/toggles checked against WCAG AA 4.5:1 including hover states before
  writing the CSS (not after) — every new interactive text pairing identified
  above was calculated, not assumed; three would have failed via a literal brief
  translation and were corrected during authoring.

## Concerns

- None blocking. The DevTools "Issues" advisory on wizard form fields is
  cosmetic/informational (not a console error) and is an app-wide pre-existing
  pattern from Task 8, out of this task's scope to change unilaterally.
- `mockPreview`'s settings seed intentionally leaves `gwForm.host` empty (the
  brief only specifies seeding `this.settings`, not `gwForm`) — this matches the
  brief exactly; the real `goSettings()` flow (not the preview) is what populates
  `gwForm.host` from `settings.host`. Worth knowing if someone later expects the
  gateway-address field to be pre-filled in `?preview=settings`.

## Fix: switch off-state contrast

**Finding addressed:** `.switch` rule-toggle's OFF state failed WCAG non-text
contrast — the white knob on the unchecked track (`background: var(--line)`,
`#ecdfc4`) computed to ~1.32:1 against the knob, and the track computed to
~1.23:1 against the card background (`--paper`, `#fdf6e8`). Both far below the
3:1 non-text minimum, making the off-state toggle barely perceptible. The ON
state (green, `--good`) was untouched — it already passed at 5.69:1.

**CSS changed** (`web/static/css/app.css`):

1. Added a new warm-palette token in `:root`, next to `--line`:

```css
--bad-line: #f2d5cd;
--line: #ecdfc4;
/* Deeper warm taupe for the switch's OFF track — the pale --line token
   reads at ~1.2:1 against the paper card and ~1.3:1 against the white
   knob, well below WCAG's 3:1 non-text minimum. This holds 4.07:1
   against --paper and 4.38:1 against a white knob (verified via the
   relative-luminance formula). */
--track-off: #8a7560;
```

2. Repointed `.slider`'s default (unchecked) background from `var(--line)` to
   the new token — `--line` itself was left untouched since it's shared by
   ~10 other border/divider rules elsewhere in the file:

```css
.slider {
  position: absolute;
  inset: 0;
  background: var(--track-off);   /* was: var(--line) */
  border-radius: var(--radius-full);
  transition: background-color .15s ease;
  cursor: pointer;
}
```

No HTML, JS, or Alpine state/method changes — the checked-state rule
(`.switch input:checked + .slider { background: var(--good); }`) is untouched.

**Computed ratios** (relative-luminance / WCAG contrast formula, verified with
a python3 script):

- Knob (`#ffffff`) vs. new OFF track (`#8a7560`): **4.38:1** (was 1.32:1)
- New OFF track (`#8a7560`) vs. card background (`#fdf6e8`): **4.07:1** (was 1.23:1)

Both comfortably clear the 3:1 non-text WCAG minimum with headroom, and sit
close to the ON state's 5.69:1 so the two states feel like a matched pair
rather than mismatched treatments.

**Visual confirmation:** Built `./cmd/bedtime` to a scratch binary, ran it
with a scratch data file on port 8899, opened `http://localhost:8899/?preview=rules`
in Chrome via the chrome-devtools MCP, and screenshotted the Rules list. The
second sample rule ("No gaming on weekends", OFF) now shows a clearly visible
warm taupe-brown track with a crisp white knob — obviously a control, and
clearly distinct from the green ON toggle on the first rule. Zero console
messages (no errors/warnings) reported by the page. Server killed and the
scratch binary/data/log files removed afterward.

**Build/test verification:** `go build ./...`, `go vet ./...`, and
`go test ./...` all pass (embed recompiled the updated CSS asset); `gofmt -l .`
reports no files. No Go source was modified.
