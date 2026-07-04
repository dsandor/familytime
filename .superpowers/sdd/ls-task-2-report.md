# Task 2 Report — Landing page gallery + enrollment sections

## Summary

Implemented Steps 1–4 of the brief in `/Users/dsandor/Projects/bedtime/landing/index.html`, using the corrected image dimensions supplied in the task context. During Step 5 browser verification I found a functional bug caused by a class-name collision in the brief's own verbatim CSS/markup, and fixed it (see "Deviation from brief" below).

## Changes made (with line ranges, post-edit file)

1. **`#enroll` section inserted** — lines 588–614, directly after `#devices`'s closing `</section>` (line 587) and before the `<!-- ============ QUIET DIFFERENTIATORS ============ -->` comment. Copy is verbatim from the brief. Images:
   - `assets/enroll-found.png` width="390" height="844" (line 592)
   - `assets/enroll-ready.png` width="390" height="844" (line 597)
   - Both have `loading="lazy"`, `decoding="async"`, and the exact alt text from the brief.

2. **`#sturdy` card replaced** — lines 626–630. "Add a device by opening a link on it." card swapped for "Built for iPhone privacy features." card, copy verbatim from brief Step 3.

3. **`#peek` gallery section inserted** — lines 636–662, directly after `#sturdy`'s closing `</section>` (line 635) and before the `<!-- ============ HOW IT WORKS ============ -->` comment. Copy verbatim. Images (using corrected dimensions per task context, not the brief's 1280×800 placeholder):
   - `assets/home.png` width="1280" height="1300" (line 641)
   - `assets/rules.png` width="1280" height="1300" (line 647)
   - `assets/groups.png` width="1280" height="1300" (line 653)
   - All three have `loading="lazy"`, `decoding="async"`, and exact alt text from the brief.

4. **CSS appended** — lines 393–414, immediately before `</style>` (originally line 392, now 415). CSS is verbatim from the brief **except** one selector rename (see below).

### Deviation from brief: renamed `.step-n` → `.enroll-step-n` (markup + CSS)

The brief's Step 1 markup and Step 4 CSS both use the class `step-n` for the enrollment steps' numbered badges — but `step-n` is already an existing class in `#how` (the "How it works" section), defined with `counter-increment:step` and a `.step-n::before{content:counter(step)}` rule that generates the visible digit via a CSS counter (the existing `#how` spans are empty; only the counter supplies the number).

Because the enrollment spans contain a literal hard-coded digit (`<span class="step-n">1</span>`), reusing the class caused the counter's generated content to concatenate with the literal digit, rendering **"11", "12", "13"** instead of "1", "2", "3" in the `#enroll` section (verified in the browser, screenshot evidence below). It also caused the *original* `#how` section's badges to shrink from 44px/1rem to 30px/.875rem, since the two `.step-n` rules share specificity and the later (appended) one won for the overlapping properties — a real, visible regression to an "existing, untouched" section.

**Fix applied:** renamed the three enrollment `<span class="step-n">` → `<span class="enroll-step-n">` (lines 595–597) and renamed the corresponding CSS selector `.step-n{...30px...}` → `.enroll-step-n{...}` (line 403). This is a pure class-name change — no visible copy, layout, sizing, or color was altered from what the brief specified for either section; it just removes the accidental collision. Verified post-fix:
- `#enroll` badges render "1", "2", "3" correctly.
- `#how` badges are back to the original 44px / 1rem / correct alpha values, matching the pre-existing design untouched.

This is the only deviation from the brief's literal text; everything else (copy, tags, attributes, other CSS rules, insertion points) was applied byte-exact.

## Browser checkpoints (chrome-devtools MCP, server: `python3 -m http.server 8090` in `landing/`)

1. **1280×900 (desktop emulation)**
   - `#enroll`: copy left / two phone-frame `<figure>`s right, exactly as the brief's layout intent. Screenshot confirmed.
   - `#peek`: 3-up gallery with browser-chrome dot bars (`.shot-bar`) above each screenshot, captions correct, images sharp (native 1280×1300 image files, no distortion/broken-image icons). Screenshot confirmed.
   - Re-angled `#sturdy` card: "Built for iPhone privacy features." card renders with correct copy, sitting beside the untouched "It keeps working…" card. Screenshot confirmed.
   - `#how` section (pre-existing): numbered badges confirmed back to their original 44×44px size / 1rem font after the class rename fix (computed style checked via `getComputedStyle`: width 44px, height 44px, font-size 16px = 1rem).
   - No horizontal scroll: `document.documentElement.scrollWidth === window.innerWidth === 1280`.
   - All 5 images: network requests returned `200`, `naturalWidth`/`naturalHeight` matched expected source dimensions (enroll images 780×1688 physical = 390×844 CSS @2x; home/rules/groups 1280×1300 physical = 1x).
   - Console: zero messages (checked via `list_console_messages`, only the harness's own "Emulating viewport" info lines, no page-origin errors/warnings).

2. **390×844 (mobile emulation, `emulate` tool with `390x844x2,mobile,touch` — the coarser `resize_page` tool would not hit exactly 390 CSS px, so I switched to viewport emulation for accuracy)**
   - Confirmed `window.innerWidth === 391` (accounts for the tool's 1px scrollbar-gutter convention) and `document.documentElement.scrollWidth === 391` — no horizontal scroll.
   - `#enroll` and `#peek` both stack to a single column (verified via `.enroll-grid` and `.shots` computed `grid-template-columns` collapsing under the `max-width:880px` media query, and visually in a targeted viewport screenshot of `#enroll`).
   - Enrollment step numbers confirmed correct ("1", "2", "3") post-fix in a mobile-viewport screenshot.
   - All 5 images loaded (`complete: true`, correct `naturalWidth`) after scrolling through the page to trigger `loading="lazy"`.
   - Console: zero page-origin messages.
   - Note: a full-page (`fullPage:true`) screenshot stitching artifact made the page appear to visually "repeat" content near the bottom on first inspection. I verified via DOM query (`document.querySelectorAll('#enroll').length` etc., and a full `getBoundingClientRect().top` listing of every `section`/`header`/`footer`/`nav`) that there is exactly one instance of every section, in the correct order, with a plausible total document height for a single-column mobile layout with 3 stacked gallery screenshots. This was a screenshot-tool artifact (related to the page's `scroll-behavior:smooth` interacting with the tool's tiled capture), not a real defect — confirmed by cropping the stitched image into strips and by independent viewport-sized screenshots, which show no duplication.

3. **Existing sections unchanged** — hero/goodnight switch, one-tap mock, schedules timelines, the untouched `#sturdy` card ("It keeps working…"), `#how` steps (after the class-collision fix restored original sizing), final CTA, and footer all visually match the pre-existing design. No other markup outside the specified insertion points and the one card replacement was touched.

4. **Zero console errors** confirmed at both viewport sizes across all checks above.

Server was stopped after verification (`lsof -ti:8090 | xargs kill`; confirmed connection refused on retry). Temporary screenshot files written during verification (`landing/.verify-tmp/`) were deleted; nothing was left behind in the working tree beyond the intended `index.html` edit.

## Self-review

- **Copy byte-exact per the brief:** Yes — headings, ledes, captions, the 3 enrollment steps, and the re-angled `#sturdy` card text all verified against the brief via an accessibility-tree snapshot (`take_snapshot`), matched verbatim.
- **All five images referenced correctly:** Yes — correct relative paths (`assets/<name>.png`), corrected dimensions per the task context (390×844 for enroll shots, 1280×1300 for the three gallery shots — not the brief's 1280×800 placeholder), `loading="lazy"`, `decoding="async"`, and descriptive alt text matching the brief verbatim on all five `<img>` tags.
- **Only the specified insertions + one card replacement:** Yes, with one additional necessary fix — the `step-n`→`enroll-step-n` rename described above. No other existing markup, copy, or CSS was touched. Confirmed via `grep` that `#devices`, `#sturdy`, `#how` sections retain their original structure except the one designated card swap.
- **Browser checkpoints all confirmed, zero console errors:** Yes, at both 1280×900 and 390×844.

## Concerns

1. **Brief defect (fixed):** The brief's own verbatim Step 1 markup + Step 4 CSS reuse the class `step-n`, which collides with the pre-existing `#how` section's counter-based `.step-n` styling. Left as-authored, this breaks the enrollment step numbers (renders "11"/"12"/"13") and shrinks the existing `#how` badges — a direct violation of the "existing sections untouched" constraint. I fixed this by renaming the enrollment-only class to `enroll-step-n` (markup line 595–597, CSS line 403), preserving every visible/behavioral detail the brief specified for both sections. Recommend this correction be folded back into the source plan/brief so future re-application doesn't reintroduce the bug.
2. The gallery screenshots (`home.png`, `rules.png`, `groups.png`) are 1280×1300 at 1x pixel density (not retina), per Task 1's actual capture — they will look slightly less crisp than the enroll shots (which are 2x) on high-DPI displays. This matches the task's explicit instruction to use the corrected dimensions, so it's expected, not a defect, but flagging for awareness.
