# Task 2 brief — ls plan (2026-07-03)

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
        <li><span class="step-n" aria-hidden="true">1</span>Open the link on their device</li>
        <li><span class="step-n" aria-hidden="true">2</span>Pick a name &amp; group</li>
        <li><span class="step-n" aria-hidden="true">3</span>Done — rules apply instantly</li>
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
.step-n{width:30px;height:30px;border-radius:50%;flex:none;display:inline-flex;align-items:center;justify-content:center;
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
