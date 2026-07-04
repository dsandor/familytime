# Landing Page: Screen Examples + Enrollment Section — Design

**Date:** 2026-07-03
**Status:** Approved

## Purpose

Show the real product on the landing page (`landing/index.html`): a gallery of actual app screens, and a dedicated section selling how easy device enrollment is, with real enroll-screen captures and copy.

## Approach (approved)

**Real screenshots as separate files** in `landing/assets/`, captured from the app's built-in client-side mock mode (`?preview=<view>`) via headless Chrome — authentic pixels, no gateway needed, easy to refresh. The landing page stops being single-file (HTML + assets folder); all CSS stays inline in the HTML.

Capture AFTER the "Give 30 more minutes" feature lands, so the Home shot shows the finished pause card.

## Screenshots

Server: the built binary on port 8080 with a scratch `-data` path; preview mode never touches the API.

| File | Source | Viewport | Notes |
|---|---|---|---|
| `landing/assets/home.png` | `/?preview=home` | 1280×800, DPR 2 | Quick-pause cards incl. `Starting` selector; kids card shows the Scheduled (pending) state via mock rule r9 |
| `landing/assets/rules.png` | `/?preview=rules` | 1280×800, DPR 2 | Rules list |
| `landing/assets/groups.png` | `/?preview=groups` | 1280×800, DPR 2 | Groups & Devices |
| `landing/assets/enroll-found.png` | `/?preview=enroll` | 390×844, DPR 2 | "We found your device" state (mock: iPhone 72:68) |
| `landing/assets/enroll-ready.png` | `/?preview=enroll` + click a group chip | 390×844, DPR 2 | Same screen with name filled and group selected |

Images get `loading="lazy"`, `decoding="async"`, explicit `width`/`height` (CLS-free), and descriptive alt text. Target ≤ ~400 KB each (dark flat UI compresses well; downscale if needed).

## Page changes (`landing/index.html`)

Existing sections stay untouched (hero, #onetap, #schedules, #devices, #how, final CTA, footer), with one exception: the `#sturdy` card "Add a device by opening a link on it." would sit right after the new enroll section and repeat it verbatim, so it is re-angled to the Private-Wi-Fi-Address robustness point (enrollment recognizes the device by its live connection; iOS's default "Fixed" private address is fully supported — per README). Download-CTA/GitHub-link follow-ups remain out of scope (no release URL yet).

1. **New `<section id="peek">` — "See it in action"** — placed between `#sturdy` and `#how`. Eyebrow + heading + one-line sub, then a 3-up grid (single column on mobile) of the desktop shots, each framed in a glassy browser-chrome card (traffic-light dots bar, rounded, luminous border — consistent with the page's dark sci-fi glass look). Captions:
   - Home — "Pause a group now — or in a little while."
   - Rules — "Every boundary, one calm list."
   - Groups & Devices — "Name a device once. Done."
2. **New `<section id="enroll">` — "Add a device in seconds"** — placed directly after `#devices` (it completes that section's story). Split layout: copy + a 3-step list on one side, the two phone-framed enroll shots on the other (phone frame: narrow rounded glass card). Stacks on mobile.
   - Copy (final): *"No MAC addresses. No router tables. Open one link on the device you're adding — Family Time recognizes it from its own connection, even when iPhones randomize their Wi-Fi address. Give it a name, pick its group, done."*
   - Steps: **1.** Open the link on their device · **2.** Pick a name & group · **3.** Done — rules apply instantly.
3. **CSS**: new rules appended to the existing inline `<style>` (frames, grid, phone frame, captions), reusing the page's existing custom properties/tokens; respects the page's existing `prefers-reduced-motion` discipline (no new animation requirements).

## Verification

- Images exist, are referenced correctly, and each is ≤ ~400 KB (downscale/re-encode if over).
- Browser check of the landing page (file:// or via a static server): gallery + enroll sections render, images sharp, layout holds at 1280w and 390w, no horizontal scroll, alt text present.
- No regressions to existing sections (visual skim).
