# Task 1 report — screenshot capture (2026-07-03)

## Summary

All five screenshots captured, verified, and right-sized. App started via `go run ./cmd/familytime -port 8080 -data <scratchpad>/ft-shots.json` in the background, driven with the chrome-devtools MCP tools, then stopped. No git operations performed (not a git repo; none needed).

## Capture details

### Desktop shots (1280×800 viewport, DPR 2 supported via `emulate` viewport string `1280x800x2`)

- **home.png** — `?preview=home`. Initial viewport-top screenshot only showed the Kids "Scheduled" card fully, with the Teens "Active/Paused" card cut off at the bottom edge (its "30 more min" button not visible) — this failed the brief's content requirement. Fixed by scrolling the page 200px (`window.scrollTo(0, 200)`) before capturing, which brings both the Kids card (amber "Scheduled" badge, "Pauses at 11:18 PM · until Sat 12:18 AM", "+30 min"/"Cancel") and the Teens card (green "Active" badge, "Paused until 11:08 PM", "30 more min"/"Resume") fully into view. Re-verified via snapshot before the fix (a11y tree confirmed both states existed) and visually after (screenshot shows both cards complete).
  - Captured at 2560×1600 (2x), downscaled to 1280×800 with `sips --resampleWidth 1280` (original was 819 KB, over budget).
  - Final: **1280×800 px, 229 KB**.
- **rules.png** — `?preview=rules`. No interaction; captured at default scroll (top of page). Snapshot confirmed 3 rule cards (School nights / No Roblox during homework / Teens: social media curfew), one showing "blocking right now", all toggles on, no error banners.
  - Captured at 2560×1600, downscaled to 1280×800 (original 886 KB, over budget).
  - Final: **1280×800 px, 245 KB**.
- **groups.png** — `?preview=groups`. No interaction; default scroll. Snapshot confirmed Kids/Teens groups, 4 devices with group comboboxes, "Devices sync from your UniFi gateway" note, no errors.
  - Captured at 2560×1600, downscaled to 1280×800 (original 811 KB, over budget).
  - Final: **1280×800 px, 228 KB**.

### Enroll shots (390×844 viewport, DPR 2 via `emulate` viewport string `390x844x2`)

- **enroll-found.png** — `?preview=enroll`, no interaction. Snapshot confirmed "Detected device" = "iPhone 72:68" (MAC a1:b2:c3:9e:7f:21), name field prefilled "iPhone 72:68", Kids/Teens group buttons unselected, Enroll button disabled.
  - Captured at 780×1688 (2x of 390×844) — already under budget, no downscale needed.
  - Final: **780×1688 px (390×844 CSS px), 315 KB**.
- **enroll-ready.png** — same view, then: cleared the name field and typed `Ava's iPhone` via `fill`, clicked the Kids group button via `click`. Verified via snapshot (textbox value = "Ava's iPhone") and via `evaluate_script` (Kids button has class `on`, confirming selected state) before capturing. Enroll button was NOT clicked. No save/enroll action taken.
  - Captured at 780×1688 — under budget, no downscale needed.
  - Final: **780×1688 px (390×844 CSS px), 358 KB**.

## Verification

- All five files exist at exact paths under `/Users/dsandor/Projects/bedtime/landing/assets/`.
- All five ≤ ~400 KB (largest is enroll-ready.png at 358 KB).
- Console messages checked on the enroll page: only two accessibility "issue" notices (missing label / id on a form field) — no errors, no error banners visible in any screenshot.
- Content check passed for all five:
  - home.png: Kids "Scheduled" card + Teens active "Paused" card with "30 more min" button both fully visible.
  - rules.png / groups.png: representative content visible, no errors.
  - enroll-found.png: "iPhone 72:68" device name and prefilled field visible.
  - enroll-ready.png: name field shows "Ava's iPhone", Kids group option visually selected (cyan border/glow), Enroll button enabled but not clicked.
- Server process (PID 91080, `familytime` binary launched by `go run`) killed; `lsof -nP -iTCP:8080 -sTCP:LISTEN` returns nothing — port 8080 confirmed free.

## Dimensions for Task 2 (`width`/`height` attributes — CSS-pixel size per brief, since all are 2x captures)

| File | Captured px | CSS px (use for width/height attrs) | File size |
|---|---|---|---|
| home.png | 1280×800 (already downscaled to 1x) | 1280×800 | 229 KB |
| rules.png | 1280×800 (already downscaled to 1x) | 1280×800 | 245 KB |
| groups.png | 1280×800 (already downscaled to 1x) | 1280×800 | 228 KB |
| enroll-found.png | 780×1688 (2x) | 390×844 | 315 KB |
| enroll-ready.png | 780×1688 (2x) | 390×844 | 358 KB |

Note: the desktop shots were captured at 2x (2560×1600) then downscaled with `sips --resampleWidth 1280` to fit the size budget, so their on-disk pixel dimensions now equal the CSS viewport size (1280×800) — use 1280×800 directly for `width`/`height`. The enroll shots stayed at their native 2x capture (780×1688) since they were already under budget — use the CSS-equivalent 390×844 for `width`/`height` attrs per the brief's instruction.

## Issues / concerns

- None outstanding. The one snag (home.png initially cropping the Teens active-pause card out of the viewport) was caught during the content self-review and fixed by scrolling 200px before capture; the replacement was re-verified visually.
- Three unrelated `Family Time` chrome-devtools pages (ids 4, 5, 6 — one on port 8099) were already open in the browser before this task started and were left untouched; they are not part of this task's app instance and were not created or closed by this work.

## Recapture fixes (2026-07-03, follow-up)

### Defects addressed

- `home.png` (old 1280×800, 200px-scrolled capture): the stat-card row (GROUPS/RULES/PAUSES) was sliced flat across its top edge — the 200px scroll needed to reveal the Teens pause card cut into the stat row above it. Read as a rendering glitch.
- `groups.png` (old 1280×800): the last device row, "Noah's Switch", was truncated mid-card at the bottom edge.
- `rules.png`: already clean, but needed to be re-shot at whatever new uniform viewport was chosen so all three desktop shots stay consistent.

### Root cause / approach

Re-ran the app (`go run ./cmd/familytime -port 8080 -data <scratchpad>/ft-shots2.json`) and used `evaluate_script` to pull exact `getBoundingClientRect()` values for the key elements on each page (nav, stat-row, Kids/Teens pause cards, Active-Right-Now section, device rows, rule cards) rather than eyeballing scroll offsets. This showed the real problem: at the original 1280×800 viewport, the home page's full "nav + stats + both pause cards" content is ~1008px tall — taller than 800 — so no scroll position at 800px height can show all of it without slicing something. The fix was a taller uniform viewport rather than a scroll offset.

Measured layout (CSS px, width=1280, unscrolled):
- nav: 0–64
- stat-row: 231–405
- Kids pause-card: 478–734
- Teens pause-card: 752–1008
- "Active Right Now" section: 1042–1273
- "Upcoming Today" section: starts at 1307 (box top, padding-inclusive)

A viewport height of **1300** lands the cut exactly in the ~34px gap between the end of "Active Right Now" (1273) and the start of "Upcoming Today" (1307) — every visible section is fully rendered, nothing is sliced, and the extra "Active Right Now" content is a bonus beyond the required nav/stats/Kids/Teens cards. Checked `groups` (`main` content ends at 1214, well inside 1300 — all 4 device rows including the new true-last row "Living Room TV" plus the sync note render completely) and `rules` (last rule card ends at 730.5, far short of 1300 — page is just shorter than viewport, which is acceptable) at the same viewport: both clean.

### Final uniform viewport

**1280×1300** (CSS px), captured at device-pixel-ratio 2 (2560×2600 physical) via chrome-devtools `emulate`, then downscaled with `sips --resampleWidth 1280` to land back at 1280×1300 for file-size budget.

### Per-file results

| File | Dimensions | Size |
|---|---|---|
| home.png | 1280×1300 | 300 KB |
| groups.png | 1280×1300 | 301 KB |
| rules.png | 1280×1300 | 249 KB |

(enroll-found.png and enroll-ready.png untouched — still 780×1688 / 315 KB and 358 KB respectively.)

### Edge-check observations (verified visually via Read tool on each PNG)

- **home.png**: top edge — nav bar complete, stat-row cards fully rounded on all four corners, no slicing. Bottom edge — "Active Right Now" section's second entry ("Blocks All internet" / Kids / "until Sat 7:56 AM") ends with its card border fully visible, followed by clean empty background to the image bottom. Kids card shows amber "Scheduled" badge, "Pauses at 11:26 PM · until Sat 12:26 AM", "+30 min"/"Cancel" — all whole. Teens card shows green "Active" badge, "Paused until 11:16 PM", "30 more min"/"Resume" — all whole.
- **groups.png**: top edge — nav and "Groups & Devices" header complete, no slicing. Bottom edge — last device row "Living Room TV" (4th device) is fully visible card-to-card, followed by the complete "Devices sync from your UniFi gateway" note and the Apple-private-address tip paragraph, then clean empty space to the image bottom. No row or card is cut.
- **rules.png**: top edge — nav and "Rules" header complete. Bottom edge — third rule card ("Teens: social media curfew") ends with its full border well above the image bottom, followed by empty background — page is shorter than the 1300 viewport, which is expected and acceptable per the brief.

### Cleanup

Server (`go run ./cmd/familytime`) stopped; `ps aux | grep familytime` and `lsof -nP -iTCP:8080 -sTCP:LISTEN` both confirm no process and port 8080 free. Temporary working copies under `/Users/dsandor/Projects/bedtime/.tmp_shots/` were deleted after copying final files into `landing/assets/`.
