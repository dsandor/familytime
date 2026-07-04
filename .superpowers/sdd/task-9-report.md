# Task 9 Report — Home dashboard + Family (profiles & devices)

## Status: DONE

## What was implemented

Replaced the Task 9 placeholders with working views and wired all required Alpine
methods, exactly per the brief's functional contract (markup/bindings/method bodies
copied verbatim; only CSS was translated into the Task 8 design language and extended).

### Files changed

- `/Users/dsandor/Projects/bedtime/web/static/index.html`
  - Replaced the `view==='home'` stub with the full Home dashboard (profile status
    cards, pause chips, pausebox, status lines, "Manage rules →", empty-state tip).
  - Added `view==='profiles'` (profile list), `view==='profile'` (profile editor:
    name, emoji picker, color picker, device checklist, save/cancel/delete), and
    `view==='rules'` (Task 10 placeholder with a working `‹ Back` button).
  - Markup is byte-for-byte the brief's contract (I initially added one extra
    `:style` binding on `.profile-card` for a personalization accent, decided it
    was unnecessary markup drift, and removed it before finishing — final markup
    has no additions beyond the brief).

- `/Users/dsandor/Projects/bedtime/web/static/js/app.js`
  - Added state fields `editError: ''`, `rulesProfileId: ''`.
  - Added preview-mode branch at the top of `init()` and the `mockPreview(view)`
    method, verbatim from the brief.
  - Replaced the three stubs with the full set of methods: `goHome`, `goProfiles`,
    `goRules`, `clockLine`, `fmtTime`, `pause`, `unpause`, `loadDevices`,
    `newProfile`, `editProfile`, `deviceChecked`, `toggleDevice`, `saveProfile`,
    `removeProfile`. `goSettings` remains the Task 10 stub, moved to the end.
  - No deviation from the brief's method bodies.

- `/Users/dsandor/Projects/bedtime/web/static/css/app.css`
  - Added `--bad-soft` / `--bad-line` tokens to `:root`, mirroring the existing
    `--good` / `--good-soft` pairing, used for the pause panel.
  - Appended a new "Home dashboard + Family" section translating the brief's CSS
    into the established tokens (`--accent` → `--amber`, `--accent-soft` →
    `--amber-soft`) and elevated to match the Task 8 visual language (serif
    `.pagehead` headings in the on-dark hero treatment, a soft amber halo on
    avatars echoing the moon-glyph glow, a lift/shadow hover on clickable profile
    rows, a warm rose pause panel instead of the brief's raw hex salmon).
  - Fixed the known dormant contrast issue for the new secondary controls:
    - `.chips button` now sets `color: var(--ink)` explicitly instead of
      inheriting the base `button` color (`--amber-deep` on `--amber-soft` is the
      documented ~4.01:1 fail); ink-on-amber-soft measures ~11.7:1.
    - `.chips button.on` keeps `color: var(--ink)` on `--amber` background
      (~6.2:1, matching `button.primary`'s already-verified pairing) instead of
      the brief's `color: white` (white-on-amber is the documented ~2.2:1 fail).
    - `button.ghost` (`--amber-deep` on `--paper`) measures ~4.7:1; `.ghost.danger`
      (`--bad` on `--paper`) measures ~5.0:1 — both clear AA.
    - `.device-list .check` uses `--ink` rather than `--amber-deep`, since the
      hover background I added (`rgba(226,153,63,.07)` over paper) pushed
      amber-deep close to the 4.5:1 edge; ink keeps a large margin.

## Browser verification (chrome-devtools MCP, port 8899, scratch data file)

Built `/private/tmp/.../scratchpad/bedtime-ui`, ran with
`--data ./bedtime-ui-scratch.json`, never touched the live gateway.

- `?preview=home` — all three mock profiles render: "Everyone" (no pause state,
  chips visible), "Emma" (active/scheduled status lines, distinct styling for
  ⛔ active vs 🕐 scheduled), "Jack" (paused panel with "until" time, Resume
  button). Zero console errors.
- `?preview=profiles` — Emma/Jack list rows with avatar, device summary text,
  chevron, "+ Add a profile". Zero console errors.
- `?preview=profile` — icon grid, color swatches, device checklist showing all
  three required states: checked/online-wireless (Emma's iPad), disabled/
  other-profile (Jack's phone, greyed with "— in another profile"), unchecked/
  online-wired (Living-room TV, 🔌), checked/offline (Switch, "(offline)").
  Interactive check: clicked the TV row (toggled on) and the disabled Jack's
  phone row (correctly ignored per `toggleDevice`'s guard); clicked an icon chip
  (selection state updated). Zero console errors.
- Confirmed `goRules(profileId)` directly against the Alpine component state
  (`Alpine.$data`): clicking Emma's "Manage rules →" set `view:'rules'`,
  `rulesProfileId:'p1'`; the Task 10 placeholder rendered with a working
  `‹ Back` button (`goHome()`). One chrome-devtools MCP `click`-by-uid call
  mis-clicked across a DOM mutation (a known stale-uid tool artifact, not an
  app bug) — reproduced cleanly via direct DOM `.click()` instead, confirmed
  correct.
- DevTools Issues panel flagged "no label associated with a form field" /
  "form field missing id/name" on the profile editor's `<label>Name <input
  x-model...></label>` pattern. This is an implicit label association (input is
  a label descendant) and is the exact same pattern already used unmodified
  throughout the Task 8 setup wizard and login views (`web/static/index.html`
  setup step 1/2 labels) — not a regression introduced in this task, and not a
  console *error* (`list_console_messages` with `types:["error"]` returned
  empty for every preview view I checked cleanly).
- Killed the scratch server (PID) and removed the scratch binary, data file,
  and server log after verification. Left an unrelated pre-existing
  `bedtime.log` in the scratchpad untouched (timestamped before this session's
  server ever started — not something this task created).

## Full-suite verification

```
go build ./... && go vet ./... && go test ./...
```
All packages built and passed (`bedtime/internal/rules`, `bedtime/internal/server`,
`bedtime/internal/store`, `bedtime/internal/unifi` all `ok`; `cmd/bedtime` and `web`
have no test files).

```
gofmt -l .
```
Empty output (no Go files were touched this task; embed still compiles cleanly).

## Self-review

- Every method named in the brief (`goHome`, `goProfiles`, `goRules`, `clockLine`,
  `fmtTime`, `pause`, `unpause`, `loadDevices`, `newProfile`, `editProfile`,
  `deviceChecked`, `toggleDevice`, `saveProfile`, `removeProfile`, `mockPreview`,
  the `init()` preview branch) is present, wired to the exact markup bindings in
  the brief, and verified live in the browser.
- `goSettings` intentionally left as the pre-existing stub (Task 10 scope), moved
  after the new methods.
- No markup/binding/method-body drift from the brief's functional contract —
  confirmed via a full re-read of the final `index.html` and `app.js` against the
  brief text.
- Only latitude taken was CSS: token renaming to match Task 8's established names,
  additional polish (avatar glow, hover lift, warm pause-panel tokens), and the
  contrast fixes documented above. All are purely visual/CSS, do not touch markup
  structure, Alpine directives, or JS logic.

## Concerns

- None blocking. The pre-existing implicit-label a11y "issue" (not error) noted
  above is inherited from Task 8's established pattern across the whole app, not
  something to fix locally in this task without a broader decision.
- The `rules` placeholder view is intentionally excluded from `authedView()` (so
  the tabbar is hidden there), which is inherited from Task 8's method and not
  something this task's brief asked to change; Task 10 should decide whether the
  real Rules view keeps the tabbar visible.
