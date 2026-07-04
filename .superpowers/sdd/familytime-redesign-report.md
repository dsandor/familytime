# Family Time rebrand + dark sci-fi glassmorphism redesign — report

Date: 2026-07-03
Scope: (A) repo-wide rename Bedtime → Family Time. (B) UI restructure (top nav,
Home dashboard, Groups & Devices, flat Rules list, Who-first wizard) retheme
to dark sci-fi glassmorphism, per reference mockup layout (colors replaced).

No git commands were used (not a git repo). The live gateway (192.168.0.1)
was never contacted — all interactive testing used `?preview=` mock data or
a scratch store file pointed at the RFC 5737 test address `192.0.2.1`.

---

## A) Rename inventory

Mechanical, ordered literal + regex substitution (Python script), reviewed
file-by-file afterward. Categories of identifiers changed:

| Category | Before | After |
|---|---|---|
| Go module | `module bedtime` | `module familytime` |
| Import paths | `bedtime/internal/...`, `bedtime/web` | `familytime/internal/...`, `familytime/web` |
| Binary dir | `cmd/bedtime/` | `cmd/familytime/` |
| Default data path | `<UserConfigDir>/bedtime/bedtime.json` | `<UserConfigDir>/familytime/familytime.json` |
| Rule tag prefix (const) | `DescriptionPrefix = "[bedtime] "` | `DescriptionPrefix = "[family-time] "` |
| Ownership-check function | `IsBedtime(desc)` | `IsFamilyTime(desc)` (all callers updated: `janitor.go`, tests) |
| Janitor local var | `bedtimeGw` | `familytimeGw` |
| E2E env gate | `BEDTIME_E2E` | `FAMILYTIME_E2E` |
| E2E gateway override | `BEDTIME_GATEWAY` | `FAMILYTIME_GATEWAY` |
| E2E rule tag | `[bedtime-e2e]` | `[family-time-e2e]` |
| Test fixture tag | `[bedtime-probe]` (testdata JSON + client_test.go) | `[family-time-probe]` |
| Session cookie name | `bedtime_session` | `familytime_session` |
| Log line prefixes | `"bedtime: ..."` | `"familytime: ..."` |
| Test identifier | `TestCleanupDeletesOrphanedBedtimeRulesAfterTwoPasses` | `TestCleanupDeletesOrphanedFamilyTimeRulesAfterTwoPasses` |
| Prose / comments / UI copy | "Bedtime" | "Family Time" (package doc comments, error strings, README, `<title>`, headings) |
| Startup banner emoji | 🌙 | 🛡️ (thematic shift to shield motif) |

`UNIFI_API_KEY` was left untouched, as instructed. `docs/`, `.superpowers/`,
and `.env` were left untouched (historical/context files).

### Files renamed (directory move)
- `cmd/bedtime/main.go` → `cmd/familytime/main.go`
- `cmd/bedtime/main_test.go` → `cmd/familytime/main_test.go`

### Files modified in place (rename substitutions + prose cleanup)
`go.mod`, `internal/e2e/e2e_test.go`, `internal/rules/translate.go`,
`internal/rules/translate_test.go`, `internal/server/auth.go`,
`internal/server/auth_test.go`, `internal/server/fake_test.go`,
`internal/server/handlers.go`, `internal/server/handlers_test.go`,
`internal/server/janitor.go`, `internal/server/janitor_test.go`,
`internal/server/rules_handlers.go` (also: 15m pause, see below),
`internal/server/rules_handlers_test.go` (also: 15m pause test),
`internal/server/server.go`, `internal/server/settings_handlers.go`,
`internal/store/store.go`, `internal/store/store_test.go`,
`internal/unifi/client_test.go`, `internal/unifi/testdata/trafficrules_probe.json`,
`README.md`, `web/embed.go` — **20 files**.

### Files fully rewritten (UI restructure, workstream B)
`web/static/index.html`, `web/static/js/app.js`, `web/static/css/app.css` — **3 files**.

### Deleted
- `./bedtime` — stale pre-rename build artifact at repo root.

### Verification
- `go build ./... && go vet ./... && go test ./...` — all green (see Test
  output below).
- `gofmt -l .` — empty (clean).
- Repo-wide case-insensitive grep for `bedtime` outside `docs/`,
  `.superpowers/`, and `.env` — **zero matches**.

---

## B) UI restructure summary

### Structural change: bottom tabbar → top nav
`web/static/index.html` now renders a sticky glass `<nav class="topnav">`
(brand mark + wordmark + Home / Groups & Devices / Rules / Settings tabs)
whenever `authedView()` is true. The PIN login and setup wizard remain
full-screen, nav-free, exactly as before.

### Home (`view: 'home'`)
- Time-of-day greeting (`greeting()`: Good morning/afternoon/evening/night)
  + fixed one-line subtitle, inside a "HUD corner-bracket" hero frame (the
  redesign's signature element — see Design tokens below).
- Three glass stat cards: **Groups** (count + total devices), **Rules**
  (count + how many active now), **Pauses** (active count) — all computed
  client-side from `/api/profiles` + `/api/rules` (no new endpoints).
- **Quick pause**: one glass card per real group (Everyone is intentionally
  excluded, matching the reference mockup) — avatar, name, device count,
  green "Active" badge when a pause rule is active. Pause buttons **15m /
  30m / 1h** (primary chips) plus **Until morning** / **Until I resume**
  (full-width secondary chips), and a **Resume** button replacing the chips
  while paused.
- **Active right now**: merged ledger of every currently-active enabled rule
  (pause and non-pause) across all groups, red/orange dot + "until" time; an
  **All clear** shield state (green check) when the list is empty.
- **Upcoming today**: enabled recurring rules whose window starts later
  today, computed client-side from `rule.when.days`/`start` vs. now, sorted
  by start time.
- "Manage rules →" on each group card opens Rules pre-filtered to that
  group (`goRules(profileId)`).

### Groups & Devices (`view: 'groups'` + `view: 'groupEditor'`)
Replaces the old Family tab. One page: **Groups** list (avatar, name,
device count, edit pencil → `groupEditor`, delete trash → confirm) + **New
group** button; below it, **Devices** — every device from `/api/devices`
with name, monospace MAC, online dot, and a **group-assignment `<select>`**
(groups + "Unassigned"). Changing the dropdown calls `reassignDevice()`,
which PUTs the old group (device removed) before PUTting the new group
(device added) — remove-before-add so the backend's "already belongs to
another profile" guard never trips. Device search box preserved. Footer:
"Devices sync from your UniFi gateway."

`groupEditor` is the old profile editor **minus the device picker** — name,
emoji, color (now 6 curated, contrast-verified accent swatches instead of
pastel colors that would have failed on a dark background), save/cancel/
delete. It still PUTs the profile's existing `devices` array unchanged.

### Rules (`view: 'rules'` + `view: 'wizard'`)
Flat list across every group (was per-profile drill-down): name, group
chip (color-coded to the group's accent, "Everyone" gets a cyan chip), "​Blocks
X · days · window" summary (`ruleSummary`), neon toggle, Edit (opens the
wizard prefilled), delete, all reusing the existing rule PUT/DELETE
endpoints. A search box filters by name, target label, and group name;
"New rule" opens the wizard. A dismissible filter chip shows when the view
was reached via a group's "Manage rules →" link.

The wizard gained a **Who** step (group picker incl. Everyone) that only
appears when **creating** a rule — `handleRuleUpdate` on the backend never
reassigns `ProfileID` on PUT, so editing an existing rule intentionally
skips Who and starts at **What**, to avoid presenting a control that would
silently do nothing. Step sequence: `wizardSteps()` returns
`['who','what','when','name']` (create) or `['what','when','name']` (edit).

### Settings (`view: 'settings'`)
Unchanged content/endpoints, restyled only; now reached via the top nav
instead of a gear icon.

### `mockPreview` sample data
Rewritten for `?preview=home|groups|rules|wizard|settings`: two groups
(Kids/Teens) with 3 assigned + 1 unassigned device (matching the reference
mockup's naming), 3 recurring rules + 1 active pause rule, all with
`when`/`what`/`active`/`until` computed relative to `Date.now()` so the
Active-now/Upcoming-today sections always populate regardless of when the
screenshot is taken.

### Backend addition
`internal/server/rules_handlers.go` `handlePause`'s duration switch gained
a `"15m"` case (`oneTimeUntil(now, now.Add(15*time.Minute))`), matching the
UI's 15m quick-pause chip. `TestPauseReplaceAndUnpause` in
`rules_handlers_test.go` now exercises `15m` (asserting `until = now+15m`)
before the existing `30m` replace-no-stack assertion.

---

## Design tokens

Dark sci-fi glassmorphism, deep-space background with two fixed low-opacity
nebula glows (indigo + cyan), glass panels (`rgba(255,255,255,.055)` fill +
`blur(18px)` + `rgba(255,255,255,.14)` border), glassy transparent buttons
(cyan/violet gradient glass primary CTA, transparent ghost secondary),
neon-glow toggle switches, inline-SVG shield brand mark (no external
assets, no CDN fonts/icons), inline-SVG data-URI favicon.

| Token | Value | Use |
|---|---|---|
| `--void` | `#070b14` | Page background |
| `--glass-fill` | `rgba(255,255,255,.055)` | Standard card |
| `--glass-fill-hover` | `rgba(255,255,255,.085)` | Hover/elevated card |
| `--glass-fill-strong` | `rgba(255,255,255,.12)` | Nav bar |
| `--glass-border` | `rgba(255,255,255,.14)` | Card border |
| `--text` | `#f3f6fb` | Primary text |
| `--text-dim` | `rgba(255,255,255,.68)` | Secondary text |
| `--text-faint` | `rgba(255,255,255,.55)` | Uppercase micro-labels |
| `--cyan` / `--cyan-bright` | `#22d3ee` / `#67e8f9` | Primary accent, links, focus |
| `--violet` / `--violet-bright` | `#8b5cf6` / `#a78bfa` | Secondary accent |
| `--green` / `--green-bright` | `#34d399` / `#6ee7b7` | On/active state |
| `--danger` / `--danger-bright` | `#ff7a59` / `#ff8f73` | Blocking/danger |
| `--ink-on-accent` | `#06121a` | Text on the cyan→violet CTA gradient |
| `--track-off` | `rgba(255,255,255,.34)` | Toggle OFF track |
| `--track-on` | `#0e7490` | Toggle ON track (deep teal; neon look comes from a glow shadow, not the fill hue) |
| Group accent swatches | `#22d3ee` `#8b5cf6` `#34d399` `#fbbf24` `#f472b6` `#60a5fa` | Curated palette replacing the old pastel color picker so every choice is pre-verified for contrast |

Typography: system font stack throughout (`-apple-system, ... sans-serif` —
no CDN fonts per the no-external-resources constraint); the "display" feel
comes from weight/spacing (greeting: 200-weight, 42px, tight tracking), not
a different family. Uppercase, letter-spaced (`.12em`) micro-labels for
section headers. `ui-monospace/SF Mono/Menlo` for MACs and the settings
data-path `<code>`.

Signature element: a HUD-style corner-bracket frame (cyan top-left, violet
bottom-right) around the Home greeting, echoing a targeting/monitoring
readout without being literal about it — the one place this UI spends its
visual boldness, per the "spend your boldness in one place" design
principle. The shield motif (brand mark, all-clear state, favicon, loading
spinner) reinforces the security/protection theme throughout.

### Computed WCAG contrast table

*Re-verified 2026-07-03 against the actual shipped `app.css` — see*
*"Post-review fixes" below for the three corrected rows, the new*
*`.sub.blocking` row, and the hover-state additions.*

All ratios computed with a Python relative-luminance/contrast script
(`(L1+.05)/(L2+.05)` per WCAG 2.1), against the **actual blended glass
background** (glass rgba fill alpha-composited over `--void`), not the raw
fill alpha. Text ≥ 4.5:1, non-text UI components ≥ 3:1. Two failures were
found and fixed before finalizing tokens (white CTA text on the gradient →
switched to dark ink; the initial toggle OFF/ON track colors both failed
against their knob/panel → track alphas/hue retuned). Selected results:

| Pair | Ratio | Min | Result |
|---|---|---|---|
| Primary text `#f3f6fb` on glass panel | 16.33:1 | 4.5 | PASS |
| Secondary text `rgba(255,255,255,.68)` on glass | 8.64:1 | 4.5 | PASS |
| Micro-label `rgba(255,255,255,.55)` on glass | 6.07:1 | 4.5 | PASS |
| Cyan `#67e8f9` link/label on glass | 12.20:1 | 4.5 | PASS |
| Violet `#a78bfa` on glass | 6.50:1 | 4.5 | PASS |
| Green `#6ee7b7` on glass | 11.61:1 | 4.5 | PASS |
| Danger `#ff8f73` on glass | 7.94:1 | 4.5 | PASS |
| CTA dark ink `#06121a` on cyan→violet gradient midpoint | 6.44:1 | 4.5 | PASS |
| CTA white text on same gradient (rejected) | 2.94:1 | 4.5 | **FAIL → not used** |
| Badge "Active" text on green badge bg | 8.48:1 | 4.5 | PASS |
| Badge "Blocking" text on danger badge bg (`.badge.badge-blocking`) | 5.98:1 | 4.5 | PASS *(corrected 2026-07-03 from an erroneous 7.61:1 — see Post-review fixes; the rule itself was dead/unused CSS and was deleted in the same pass)* |
| Group-chip text (worst curated swatch: violet, 72% mix) | 5.46:1 | 4.5 | PASS |
| Mono MAC text `.device-row .mac` (`--text-faint`, 55% white) on glass | 6.07:1 | 4.5 | PASS *(corrected 2026-07-03 — same color+background pairing as the Micro-label row above; the previously listed 72%-alpha/9.56:1 variant did not match any shipped CSS and has been removed)* |
| `.sub.blocking` "⛔ blocking right now" text (`--danger-bright`) on glass card | 7.94:1 | 4.5 | PASS *(added 2026-07-03; identical pairing to the "Danger on glass" row — reuses the already-verified token)* |
| Input placeholder `rgba(255,255,255,.42)` on input bg | 4.09:1 | 3.0 | PASS (informative only) |
| Nav active tab cyan on nav-bar glass | 10.14:1 | 4.5 | PASS |
| Toggle OFF track vs glass panel | 3.11:1 | 3.0 | PASS *(was 1.63:1 before retuning to 34% white alpha)* |
| Toggle ON track `#0e7490` vs glass panel | 3.30:1 | 3.0 | PASS |
| Knob `#f3f6fb` vs OFF track | 5.24:1 | 3.0 | PASS |
| Knob `#f3f6fb` vs ON track | 4.95:1 | 3.0 | PASS *(was 1.67:1 with raw `#22d3ee` track — retuned)* |
| Focus ring `#67e8f9` vs glass panel | 12.20:1 | 3.0 | PASS |
| Swatch border (worst: violet, 85% alpha) vs glass | 3.36:1 | 3.0 | PASS *(corrected 2026-07-03 from an erroneous 3.12:1 — see Post-review fixes; still PASS)* |

All 6 curated group-accent swatches (cyan/violet/green/amber/pink/blue) were
individually verified for both their badge text (≥5.46:1, worst case) and
swatch-button border (≥3.36:1, worst case) — violet was the binding
constraint in both checks and set the final tuning (72% white mix for
badge text, 85% alpha for swatch borders).

#### Hover states

Added 2026-07-03 (previously not covered by the table). Same script and
methodology, against each control's real containing surface (`.pause-card`
and rule-row/group-editor/settings cards are all `.card` → glass-over-void;
the nav tabs sit on the `.topnav` bar, whose background reduces to the same
composited value since its base color equals `--void`):

| Pair | Ratio | Min | Result |
|---|---|---|---|
| `.pause-actions .chip.secondary:hover` text (`--cyan-bright`) on hover bg | 10.57:1 | 4.5 | PASS |
| `button:hover` text (`--text`) on hover bg (glass-fill-hover over a card) | 12.90:1 | 4.5 | PASS |
| `.topnav .tabs button:hover` text (`--text`) on hover bg | 16.33:1 | 4.5 | PASS |
| `.ghost:hover` text (`--cyan-bright`) on hover bg | 10.27:1 | 4.5 | PASS |

---

## Browser verification results

Server built to a scratch binary and run on `127.0.0.1:8899` with a scratch
store file under the session scratchpad — **never** the live gateway. The
setup wizard's "Test connection" was never clicked (that would have hit the
auto-detected local gateway at `192.168.0.1`); instead the configured/authed
state needed for the login-PIN and empty-authed-view checks was produced by
writing a store JSON directly with `gateway.host = "192.0.2.1"` (RFC 5737
TEST-NET-1, guaranteed non-routable/unreachable), a locally-generated
bcrypt PIN hash, and a random session secret — so login and the real
(non-preview) `/api/profiles` and `/api/rules` calls could be exercised
with zero risk of contacting a real device. Attempting `/api/devices`
against that address correctly surfaced the existing "Can't reach your
UniFi gateway right now." error banner (expected — confirms error-path
styling), and the actual `?preview=` flows (the ones the task asked to be
screenshotted) used the client-side mock and made no network calls to a
gateway at all.

| View | Result | Console |
|---|---|---|
| Setup wizard, step 1 (unconfigured root `/`) | Renders correctly: shield mark, gradient CTA, env-key hint banner | 0 errors (2 minor DOM advisories re: password field not in a `<form>`, pre-existing pattern) |
| Login PIN pad (real, configured+unauthed) | Renders correctly; wrong-PIN shows "Wrong PIN." in danger color; correct PIN (`1234`) logs in | 0 errors |
| Home, real empty state (0 groups/rules) | Stat cards show 0s, all-clear shield, "Add your first group…" empty-state copy | 0 errors |
| Groups & Devices, real empty state + forced gateway-unreachable | "Can't reach your UniFi gateway right now." banner renders correctly (expected, placeholder gateway is deliberately unreachable) | 1 expected 502 (deliberate); 1 DOM advisory |
| `?preview=home` | Full dashboard: greeting, 3 stat cards (2 groups/3 devices, 3 rules/1 active, 1 pause active), 2 quick-pause cards (Kids with 15m/30m/1h + secondary chips, Teens showing green "Active" badge + Resume), Active-right-now ledger (2 entries), Upcoming-today ledger (2 entries) | 0 errors |
| `?preview=groups` | Groups list (Kids/Teens, edit+delete icons) + Devices list (4 devices, MACs in monospace, online dots, group dropdowns pre-set correctly) | 0 errors (DOM advisories only) |
| `?preview=rules` | 3 rules, group chips color-coded per accent, one showing "⛔ blocking right now", toggles in neon-glow ON state | 0 errors (DOM advisory only) |
| `?preview=wizard` — Who step | Everyone / Kids / Teens tiles, "Kids" pre-selected with cyan glow ring | 0 errors (DOM advisories only) |
| `?preview=wizard` — What step | Preset grid, categories, website/all-internet chips; Next disabled until a selection is made, then enables | 0 errors |
| `?preview=wizard` — When step | Day chips, time inputs, "🌒 Overnight" hint rendering correctly for the 20:00–07:00 default | 0 errors |
| `?preview=wizard` — Name step | Auto-suggested name "No YouTube on school nights", recap sentence, gradient "Create rule" CTA | 0 errors |
| Group editor (`editGroup`) | Name field, icon grid, 6 curated color swatches (cyan ring selected), Save/Cancel/Delete — no device picker | 0 errors (DOM advisories only) |
| `?preview=settings` | Gateway/PIN/data-path sections render, primary CTAs correctly disabled (dimmed) until inputs are filled | 0 errors (1 pre-existing password-field-outside-form advisory) |
| Mobile viewport (390×844) | Stat cards stack to a single column, chips wrap, no horizontal scroll (`scrollWidth === innerWidth`) | 0 errors |
| Desktop viewport (1280×900) | No horizontal overflow (`scrollWidth === innerWidth`) | 0 (1 DOM advisory) |

The only recurring console entries across all views are Chrome DevTools'
advisory-level (`issue`/`verbose`, not `error`) notes that form inputs
aren't wrapped in `<form>` elements or lack `name`/`label` attributes — this
pattern pre-dates the redesign (same in the original app) and produced zero
`error`-level console messages on any view.

Server was killed and all scratch files (binary, data JSON, server log, a
throwaway bcrypt-hash generator) were removed after verification.

---

## Test output (final run)

```
go build ./...    → (no output, success)
go vet ./...      → (no output, success)
go test ./...     → ok  familytime/cmd/familytime
                     ok  familytime/internal/e2e
                     ok  familytime/internal/rules
                     ok  familytime/internal/server   (3.2s)
                     ok  familytime/internal/store
                     ok  familytime/internal/unifi
                     ?   familytime/web  [no test files]
gofmt -l .        → (empty — clean)
```

68 individual `--- PASS` test cases across all packages, 0 failures. New/
changed coverage for this task:
- `TestPauseReplaceAndUnpause` (`internal/server/rules_handlers_test.go`)
  now asserts the `15m` pause duration (`until = now+15m`) before its
  existing 30m/morning/indefinite/unpause assertions.
- `TestCleanupDeletesOrphanedFamilyTimeRulesAfterTwoPasses` (renamed from
  the Bedtime-named version) still exercises `rules.IsFamilyTime` /
  `[family-time]` tag orphan cleanup unchanged in behavior.
- `TestDescriptionRoundTrip` (`internal/rules/translate_test.go`) exercises
  the renamed `IsFamilyTime`/`Description`/`FamilyRuleID` against the new
  `[family-time] ` prefix.

The opt-in live-gateway E2E test (`internal/e2e/e2e_test.go`,
`FAMILYTIME_E2E=1`) was **not** run, per instructions.

The `e2e` package's own unit-level compile/skip path (`go test ./...`
without `FAMILYTIME_E2E=1`, which just skips the one test) **was** included
in the full-suite run above and reports `ok`.

---

## Post-review fixes

Date: 2026-07-03. Three review findings addressed, static-only (no git
commands — not a git repo; the live gateway at 192.168.0.1 was never
contacted). Interactive verification used `?preview=rules` against a
scratch binary + scratch store on `127.0.0.1:8903` (killed and cleaned up
afterward).

### Fix 1 (Important) — `reassignDevice` partial-failure rollback

`web/static/js/app.js`, `reassignDevice()`. The function still does
remove-before-add (two sequential PUTs), unchanged — the backend rejects
adding a MAC that's still assigned to another profile, so that ordering is
load-bearing and was kept. What changed is failure handling once the first
PUT (remove from the old group) has already succeeded:

- **First PUT (remove from old) fails** — nothing changed on the backend
  yet; behavior is unchanged from before: banner = the error message,
  refresh devices, stop.
- **Second PUT (add to new) fails, and there was an old group** — the
  device has already been removed from `old` on the backend, so a
  compensating PUT re-adds it to `old` (merging it back into `old.devices`,
  de-duped by MAC first).
  - Rollback succeeds → banner: *"Couldn't move `<device>` — it's back in
    `<old group>`. Check your connection and try again."*
  - Rollback also fails → banner: *"`<device>` was removed from
    `<old group>` but couldn't be added to `<new group>` — it's currently
    unassigned. Pick its group again."*
- **Second PUT fails, and there was no old group (device was Unassigned)**
  — no rollback is possible or needed (there was no first PUT to undo);
  banner = the add error's message.
- **Unassigned as target** (moving a device to Unassigned) — there is no
  second PUT at all, so there's no new failure mode here; this path is
  unchanged (happy path).
- In every path above, `loadCore()` + `loadDevices()` are refreshed
  unconditionally at the end so the UI always reflects real backend state,
  whether that's the happy-path result, a successful rollback, or a
  stranded-unassigned device.

Diff:

```diff
--- a/web/static/js/app.js (reassignDevice, before)
+++ b/web/static/js/app.js (reassignDevice, after)
@@
     // reassignDevice moves a device between groups: remove it from its old
     // group's device array (if any), then add it to the new one (if any),
     // as two PUTs — remove-before-add so the backend's "already belongs to
     // another profile" guard never trips.
+    //
+    // Partial-failure handling: if the first PUT (remove from old) fails,
+    // nothing changed on the backend — surface the error as before. If the
+    // second PUT (add to new) fails, the device has *already* been removed
+    // from `old` on the backend, so we attempt a compensating PUT to put it
+    // back where it was, rather than leaving it stranded unassigned.
     async reassignDevice(device, newProfileId) {
       const oldProfileId = device.profileId || '';
       newProfileId = newProfileId || '';
       if (newProfileId === oldProfileId) return;
-      try {
-        if (oldProfileId) {
-          const old = this.profiles.find(p => p.id === oldProfileId);
-          if (old) {
-            await this.api('PUT', '/api/profiles/' + old.id, {
-              name: old.name, emoji: old.emoji, color: old.color,
-              devices: old.devices.filter(d => d.mac !== device.mac),
-            });
-          }
-        }
-        if (newProfileId) {
-          const next = this.profiles.find(p => p.id === newProfileId);
-          if (next) {
-            await this.api('PUT', '/api/profiles/' + next.id, {
-              name: next.name, emoji: next.emoji, color: next.color,
-              devices: [...next.devices, { mac: device.mac, name: device.name }],
-            });
-          }
-        }
-        await Promise.all([this.loadCore(), this.loadDevices()]);
-      } catch (e) {
-        this.banner = e.message;
-        await this.loadDevices(); // reset the dropdown to the real state
-      }
+      const old = oldProfileId ? this.profiles.find(p => p.id === oldProfileId) : null;
+      const next = newProfileId ? this.profiles.find(p => p.id === newProfileId) : null;
+
+      try {
+        if (old) {
+          await this.api('PUT', '/api/profiles/' + old.id, {
+            name: old.name, emoji: old.emoji, color: old.color,
+            devices: old.devices.filter(d => d.mac !== device.mac),
+          });
+        }
+      } catch (e) {
+        this.banner = e.message;
+        await this.loadDevices(); // reset the dropdown to the real state
+        return;
+      }
+
+      if (next) {
+        try {
+          await this.api('PUT', '/api/profiles/' + next.id, {
+            name: next.name, emoji: next.emoji, color: next.color,
+            devices: [...next.devices, { mac: device.mac, name: device.name }],
+          });
+        } catch (addErr) {
+          // remove-before-add already pulled the device out of `old` (if
+          // any) above; the add to `next` failed, so the device is
+          // currently unassigned on the backend unless we can roll it back.
+          if (old) {
+            try {
+              await this.api('PUT', '/api/profiles/' + old.id, {
+                name: old.name, emoji: old.emoji, color: old.color,
+                devices: [...old.devices.filter(d => d.mac !== device.mac), { mac: device.mac, name: device.name }],
+              });
+              this.banner = `Couldn't move ${device.name} — it's back in ${old.name}. Check your connection and try again.`;
+            } catch (rollbackErr) {
+              this.banner = `${device.name} was removed from ${old.name} but couldn't be added to ${next.name} — it's currently unassigned. Pick its group again.`;
+            }
+          } else {
+            // Unassigned as source: there was no first PUT, so there's
+            // nothing to roll back — just surface the add failure.
+            this.banner = addErr.message;
+          }
+        }
+      }
+
+      // Always refresh, regardless of which branch above ran, so the UI
+      // reflects the real backend state (happy path, rolled-back, or
+      // stranded-unassigned).
+      await Promise.all([this.loadCore(), this.loadDevices()]);
     },
```

### Fix 2 (Important) — corrected contrast table

Three entries in the "Computed WCAG contrast table" above didn't match the
actually-shipped `app.css`. All three were recomputed with a Python
relative-luminance/alpha-compositing script (same `(L1+.05)/(L2+.05)` WCAG
2.1 formula as the original table), compositing each real rgba() value over
its real containing surface rather than reusing the old numbers:

| Row | Was | Now | Why |
|---|---|---|---|
| Mono MAC text | `rgba(255,255,255,.72)` → **9.56:1** | `--text-faint` (55% white) → **6.07:1** | `.device-row .mac` (`app.css`) sets `color: var(--text-faint)`, not a standalone 72%-alpha value — no such rule exists anywhere in the shipped CSS. 6.07:1 is identical to the already-correct Micro-label row (same color, same glass background) since they use the same token. |
| Swatch border | violet, 85% alpha → **3.12:1** | violet, 85% alpha → **3.36:1** | `.chips .swatch` border is `color-mix(in srgb, var(--accent) 85%, transparent)` = `rgba(139,92,246,.85)`, composited over the real `.card` background (`--glass-fill` `.055` over `--void #070b14` = `rgb(20.6,24.4,32.9)`). Still PASS against the 3:1 non-text minimum either way; the stored number was simply computed wrong. |
| Badge "Blocking" text | **7.61:1** | **5.98:1** | `.badge.badge-blocking` background `rgba(255,122,89,.18)` composited over the real `.card` background gives `rgb(62.8,42.0,43.0)`; `--danger-bright` (`#ff8f73`) text against that composited badge background is 5.98:1, not 7.61:1. Still PASS against the 4.5:1 text minimum (12px/700-weight badge text does not meet the "large text" bold threshold, so 4.5:1 is the applicable minimum). This rule was also identified as dead CSS (see Fix 3) and deleted in the same pass — kept in the table, marked corrected, for audit trail. |

New row added per Fix 3 (see below): `.sub.blocking` text (`--danger-bright`
on the same real `.card` background) → **7.94:1** — identical pairing to
the already-verified "Danger `#ff8f73` on glass" row, since both are plain
`--danger-bright` text directly on a `.card`.

A **new "Hover states" subsection** was added to the table (previously
uncovered): `.pause-actions .chip.secondary:hover` (10.57:1),
`button:hover` (12.90:1), `.topnav .tabs button:hover` (16.33:1), and
`.ghost:hover` (10.27:1) — all computed against each control's real
containing surface (`.card` for the first two and the last; the `.topnav`
bar, which composites to the same value as a `.card` here because the
nav's own base color equals `--void`). All comfortably PASS the 4.5:1 text
minimum.

The table in "Computed WCAG contrast table" above has been edited in place
with these corrections and is marked **re-verified 2026-07-03**.

### Fix 3 (Minor) — style the "blocking right now" indicator

`web/static/index.html` renders
`<p class="sub blocking" x-show="r.active">⛔ blocking right now</p>` in the
flat Rules list, but `app.css` had no `.sub.blocking` text rule — the text
rendered in the default `.sub` color (`--text-dim`, a neutral gray), not
the danger color used for "blocking" everywhere else in the app (ledger
dot, badges, etc.).

Added to `web/static/css/app.css`, next to the other `.rule-row` rules:

```css
/* "⛔ blocking right now" — danger text on the .card glass fill; see
   familytime-redesign-report.md contrast table (reuses the already-verified
   --danger-bright-on-glass pairing). */
.sub.blocking { margin: 4px 0 0; color: var(--danger-bright); font-weight: 600; }
```

`--danger-bright` (`#ff8f73`) on the real `.card` background computes to
**7.94:1** (see table above) — well above the 4.5:1 text minimum, and
reuses a token/background pairing that was already independently verified
elsewhere in the table ("Danger `#ff8f73` on glass").

Also grepped the repo for `badge-blocking` / `badge_blocking` /
`badgeBlocking` across `web/` — confirmed `.badge.badge-blocking` in
`app.css` had zero references in `index.html` or `app.js` (only
`.ledger-dot.blocking` is used for the Home dashboard's active-now ledger,
and `.sub.blocking` — added by this fix — for the Rules list). Deleted the
dead `.badge.badge-blocking` rule from `app.css`.

### Verification

```
$ go build ./...
(no output, success)

$ go vet ./...
(no output, success)

$ go test ./... -count=1
ok  	familytime/cmd/familytime	0.253s
ok  	familytime/internal/e2e	0.355s
ok  	familytime/internal/rules	0.464s
ok  	familytime/internal/server	2.856s
ok  	familytime/internal/store	0.797s
ok  	familytime/internal/unifi	0.584s
?   	familytime/web	[no test files]

$ gofmt -l .
(empty — clean)

$ node --check web/static/js/app.js
(no output, success)
```

Browser check: built a scratch binary
(`cmd/familytime` → scratch dir), ran it on `127.0.0.1:8903` with a scratch
store file, and loaded `?preview=rules` via chrome-devtools. Confirmed via
`getComputedStyle`: the "⛔ blocking right now" element has
`color: rgb(255, 143, 115)` (`--danger-bright`) and no `.badge-blocking`
element exists anywhere in the DOM. Screenshot confirmed the text renders
in the same warm red-orange used for the neon toggle glow and other danger
elements. Console showed only the one pre-existing advisory-level
(`issue`) message about a form field missing an `id`/`name` attribute — the
same pre-existing pattern called out in the original browser verification
table above, no new errors introduced. Server was killed
(`lsof -ti tcp:8903 | xargs kill`) and the scratch binary/store/log were
removed afterward; the live gateway at 192.168.0.1 was never contacted.
