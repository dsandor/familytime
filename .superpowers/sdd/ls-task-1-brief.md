# Task 1 brief — ls plan (2026-07-03)

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
