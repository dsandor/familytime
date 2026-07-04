# Landing Page Screens + Enrollment Section Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add real app screenshots to the landing page — a 3-shot "See it in action" gallery and an "Add a device in seconds" enrollment section with phone-framed enroll captures and copy.

**Architecture:** Screenshots are captured from the app's client-side mock mode (`?preview=<view>`, no gateway needed) via the chrome-devtools MCP tools and saved as PNGs in `landing/assets/`. The landing page (`landing/index.html`, single file with inline CSS) gains two sections built from its existing tokens/classes (`.wrap`, `.eyebrow`, `.lede`, `.glass`). One existing card in `#sturdy` is re-angled to avoid repeating the new enroll section verbatim.

**Tech Stack:** Static HTML/CSS (inline styles, zero JS changes), chrome-devtools MCP for capture, `sips` for image inspection/downscaling.

**Spec:** `docs/superpowers/specs/2026-07-03-landing-screens-design.md`

**Prerequisite:** The "Give 30 More Minutes" plan must already be implemented — the Home capture must show the finished pause cards.

## Global Constraints

- **NEVER commit or push to git** (user rule — this plan intentionally has no commit steps; do not add any). The project is not a git repository; do not run git commands.
- Asset filenames exact: `landing/assets/home.png`, `rules.png`, `groups.png`, `enroll-found.png`, `enroll-ready.png`.
- Desktop shots 1280×800 viewport; enroll shots 390×844 viewport; DPR 2 if the emulation supports it, otherwise DPR 1.
- Each image ≤ ~400 KB (downscale/re-encode if over).
- All `<img>` tags: `loading="lazy"`, `decoding="async"`, explicit `width`/`height`, descriptive `alt`.
- Section copy verbatim from this plan (headings, lede, captions, steps).
- Existing sections untouched EXCEPT the `#sturdy` "Add a device by opening a link on it." card, which is replaced per Task 2 Step 3.
- Marketing claims must match the README: enrollment recognizes devices despite iOS Private Wi-Fi Address (default "Fixed" mode); do NOT claim it survives the non-default "Rotating" mode.
- `ls` is aliased on this machine — use `/bin/ls` when you need it.

---

### Task 1: Capture the five screenshots

**Files:**
- Create: `landing/assets/home.png`, `landing/assets/rules.png`, `landing/assets/groups.png`, `landing/assets/enroll-found.png`, `landing/assets/enroll-ready.png`

**Interfaces:**
- Consumes: the app binary (`go run ./cmd/familytime`) and its preview views `home`, `rules`, `groups`, `enroll` (client-side mock data; API calls are rejected — never "save" anything in the enroll flow).
- Produces: the five PNGs above at the exact paths; Task 2's markup references them by those names with the captured intrinsic dimensions.

- [ ] **Step 1: Start the app**

```bash
mkdir -p /Users/dsandor/Projects/bedtime/landing/assets
cd /Users/dsandor/Projects/bedtime
go run ./cmd/familytime -port 8080 -data /private/tmp/claude-501/-Users-dsandor-Projects-bedtime/18358d72-697b-4c57-9c0f-c8083c1d9c54/scratchpad/ft-shots.json
```
Run in the background; confirm `curl -s -o /dev/null -w '%{http_code}' http://localhost:8080/` prints 200.

- [ ] **Step 2: Capture the three desktop shots**

Using the chrome-devtools MCP tools: open a page, resize to 1280×800 (attempt DPR 2 via `emulate` with `deviceScaleFactor: 2`; if unsupported, continue at DPR 1), then for each of:
- `http://localhost:8080/?preview=home` → save as `landing/assets/home.png`. Before capturing, confirm via snapshot that the Kids card shows the amber "Scheduled" state and the Teens card shows the active "Paused" state with the "30 more min" button.
- `http://localhost:8080/?preview=rules` → `landing/assets/rules.png`
- `http://localhost:8080/?preview=groups` → `landing/assets/groups.png`

Capture the viewport (not full page). If `take_screenshot` can't write directly to the target path, save wherever it allows and `cp` into `landing/assets/`.

- [ ] **Step 3: Capture the two enroll shots**

Resize the page to 390×844:
- `http://localhost:8080/?preview=enroll` → `landing/assets/enroll-found.png` (the "we found your device" state, mock device "iPhone 72:68").
- Then interact before the second shot: the name field is prefilled ("iPhone 72:68") — clear it and type `Ava's iPhone`, then click the **Kids** group option. Screenshot → `landing/assets/enroll-ready.png`. Do NOT click any save/enroll button (the API will reject and show an error banner).

- [ ] **Step 4: Verify and right-size the images**

```bash
for f in /Users/dsandor/Projects/bedtime/landing/assets/*.png; do sips -g pixelWidth -g pixelHeight "$f"; /bin/ls -lh "$f"; done
```
Expected: five files; desktop shots 1280 or 2560 px wide, enroll shots 390 or 780 px wide. Any file over ~400 KB: downscale desktop shots to 1280 wide / enroll shots to 480 wide with `sips --resampleWidth <w> "$f"` and re-check. Record the final pixel dimensions of each file — Task 2 needs them for `width`/`height` attributes (use the CSS-pixel size, i.e. 1280×800 and 390×844, when the file is a 2× capture of those viewports).

- [ ] **Step 5: Stop the app**

Stop the background `go run` process and confirm port 8080 is free (`lsof -i :8080` shows nothing for it).

---

### Task 2: Landing page — gallery + enrollment sections

**Files:**
- Modify: `landing/index.html` — insert `#enroll` section after the `#devices` section (its `</section>` is at ~line 562); insert `#peek` section after the `#sturdy` section (`</section>` ~line 582, just before the `<!-- ============ HOW IT WORKS ============ -->` comment); replace one `#sturdy` card (~lines 575–579); append CSS immediately before the closing `</style>`.

**Interfaces:**
- Consumes: the five PNGs from Task 1 (`assets/<name>.png` relative to `landing/`); existing landing classes `.wrap`, `.eyebrow`, `.lede`, `.glass`, `.card`, `.cards2`, `.foot-note` and tokens `--dim`, `--edge`, `--text`, `--cyan-bright`, `--font-mono`.
- Produces: sections `#peek` and `#enroll`; final deliverable, nothing downstream.

- [ ] **Step 1: Insert the enrollment section**

Directly after the `#devices` section's closing `</section>` (before the `<!-- ============ QUIET DIFFERENTIATORS ============ -->` comment), insert:

```html
<!-- ============ ENROLLMENT ============ -->
<section id="enroll">
  <div class="wrap enroll-grid">
    <div>
      <p class="eyebrow">Adding a device</p>
      <h2>Add a device in seconds.</h2>
      <p class="lede">No MAC addresses. No router tables. Open one link on the device you're adding — Family Time recognizes it from its own connection, even when iPhones randomize their Wi-Fi address. Give it a name, pick its group, done.</p>
      <ol class="enroll-steps">
        <li><span class="enroll-step-n" aria-hidden="true">1</span>Open the link on their device</li>
        <li><span class="enroll-step-n" aria-hidden="true">2</span>Pick a name &amp; group</li>
        <li><span class="enroll-step-n" aria-hidden="true">3</span>Done — rules apply instantly</li>
      </ol>
    </div>
    <div class="enroll-shots">
      <figure class="phone glass">
        <img src="assets/enroll-found.png" width="390" height="844" loading="lazy" decoding="async"
             alt="Enrollment screen on a phone: Family Time found the visiting device and shows iPhone 72:68 with its network details.">
        <figcaption>It knows which device you're holding.</figcaption>
      </figure>
      <figure class="phone glass">
        <img src="assets/enroll-ready.png" width="390" height="844" loading="lazy" decoding="async"
             alt="Enrollment screen with the device renamed to Ava's iPhone and the Kids group selected, ready to finish.">
        <figcaption>Name it, pick a group, done.</figcaption>
      </figure>
    </div>
  </div>
</section>
```

(If Task 1 recorded different final CSS-pixel dimensions, use those in `width`/`height`.)

- [ ] **Step 2: Insert the gallery section**

Directly after the `#sturdy` section's closing `</section>` (before the `<!-- ============ HOW IT WORKS ============ -->` comment), insert:

```html
<!-- ============ SEE IT ============ -->
<section id="peek">
  <div class="wrap">
    <p class="eyebrow">A quick look</p>
    <h2>Calm control, one screen at a time.</h2>
    <p class="lede">The whole app is a handful of quiet screens. Here are the three you'll live in.</p>
    <div class="shots">
      <figure class="shot glass">
        <div class="shot-bar" aria-hidden="true"><span></span><span></span><span></span></div>
        <img src="assets/home.png" width="1280" height="800" loading="lazy" decoding="async"
             alt="Family Time home screen: quick-pause cards for each group with 15 minute, 30 minute, one hour, until-morning options, a Starting selector to schedule the pause for a little later, and a scheduled pause showing when it will begin.">
        <figcaption>Home — pause a group now, or in a little while.</figcaption>
      </figure>
      <figure class="shot glass">
        <div class="shot-bar" aria-hidden="true"><span></span><span></span><span></span></div>
        <img src="assets/rules.png" width="1280" height="800" loading="lazy" decoding="async"
             alt="Family Time rules screen: a searchable list of family rules like school-night YouTube blocks with on-off toggles.">
        <figcaption>Rules — every boundary, one calm list.</figcaption>
      </figure>
      <figure class="shot glass">
        <div class="shot-bar" aria-hidden="true"><span></span><span></span><span></span></div>
        <img src="assets/groups.png" width="1280" height="800" loading="lazy" decoding="async"
             alt="Family Time groups and devices screen: kids' devices tagged by name and organized into groups.">
        <figcaption>Groups &amp; Devices — name a device once. Done.</figcaption>
      </figure>
    </div>
  </div>
</section>
```

- [ ] **Step 3: Re-angle the duplicated `#sturdy` card**

Replace this card in `#sturdy`:

```html
      <div class="card glass">
        <h3>Add a device by opening a link on it.</h3>
        <p class="lede">No hunting through router lists of "iPhone 72:68." Open one link in the browser on your kid's device, give it a name, pick its group — done.</p>
        <p class="foot-note">Works with iPhones and iPads out of the box.</p>
      </div>
```

with:

```html
      <div class="card glass">
        <h3>Built for iPhone privacy features.</h3>
        <p class="lede">iPhones hide behind a randomized Wi-Fi address, which is why they show up in router lists as "iPhone 72:68." Family Time recognizes the device by its live connection instead, so enrollment just works.</p>
        <p class="foot-note">Private Wi-Fi Address on? Still works.</p>
      </div>
```

- [ ] **Step 4: Append the CSS**

Immediately before the closing `</style>`, append:

```css
/* ---------- screens: gallery + enrollment ---------- */
.shots{display:grid;grid-template-columns:repeat(3,1fr);gap:20px;margin-top:44px}
.shot{overflow:hidden;padding:0;margin:0}
.shot-bar{display:flex;gap:6px;padding:12px 14px;border-bottom:1px solid var(--edge)}
.shot-bar span{width:10px;height:10px;border-radius:50%;background:rgba(255,255,255,.14)}
.shot img{display:block;width:100%;height:auto}
.shot figcaption{padding:14px 16px;color:var(--dim);font-size:.9375rem}
.enroll-grid{display:grid;grid-template-columns:1fr 1fr;gap:48px;align-items:center}
.enroll-steps{list-style:none;margin-top:28px;display:grid;gap:14px}
.enroll-steps li{display:flex;align-items:center;gap:14px;color:var(--text)}
.enroll-step-n{width:30px;height:30px;border-radius:50%;flex:none;display:inline-flex;align-items:center;justify-content:center;
  font-family:var(--font-mono);font-size:.875rem;color:var(--cyan-bright);
  background:rgba(34,211,238,.10);border:1px solid rgba(34,211,238,.35)}
.enroll-shots{display:flex;gap:20px;justify-content:center}
.phone{overflow:hidden;padding:0;margin:0;max-width:230px;border-radius:28px}
.phone img{display:block;width:100%;height:auto}
.phone figcaption{padding:12px 14px;color:var(--dim);font-size:.875rem;text-align:center}
@media (max-width:880px){
  .shots{grid-template-columns:1fr}
  .enroll-grid{grid-template-columns:1fr;gap:36px}
  .enroll-shots{flex-wrap:wrap}
}
```

- [ ] **Step 5: Verify in the browser**

Serve the folder (`cd /Users/dsandor/Projects/bedtime/landing && python3 -m http.server 8090`) and open `http://localhost:8090/` via the chrome-devtools MCP tools:
- At 1280×900: `#enroll` renders copy left / two phone frames right; `#peek` shows the 3-up gallery with browser-chrome bars; images sharp and loaded (no broken images); the re-angled `#sturdy` card reads correctly.
- At 390×844: both new sections stack to one column; no horizontal scroll anywhere on the page.
- Existing sections visually unchanged (hero switch, timelines, how-it-works, footer).
- Zero console errors.
Stop the server afterward.

---

## Self-Review Notes

- Spec coverage: five captures with exact names/viewports (Task 1), gallery section with three captioned desktop shots (Task 2 Step 2), enrollment section with copy + steps + two phone shots (Task 2 Step 1), sturdy-card re-angle per amended spec (Task 2 Step 3), CSS with lazy/dimension/alt constraints in markup, size budget + downscale path (Task 1 Step 4), verification incl. mobile + no-horizontal-scroll (Task 2 Step 5).
- Copy accuracy: enrollment claims recognition despite randomized addresses (true — resolution is by live connection/IP); no claim about the non-default "Rotating" mode, per README caveat.
- No commit steps by design — the user's global rules forbid git; do not add them.
- Type consistency: asset filenames and dimensions match between Task 1 and Task 2 markup; class names used in markup (`shots`, `shot`, `shot-bar`, `enroll-grid`, `enroll-steps`, `enroll-step-n`, `enroll-shots`, `phone`) all defined in Step 4's CSS.
