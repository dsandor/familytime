# Task 2 brief — gm plan (2026-07-03)

## Global Constraints

- **NEVER commit or push to git** (user rule — this plan intentionally has no commit steps; do not add any). The project is not a git repository; do not run git commands.
- All Go commands run from the repo root: `/Users/dsandor/Projects/bedtime`.
- No new dependencies — Go stdlib and the existing Alpine.js only.
- Wire contract is exact: route `POST /api/pause/{profileId}/delay`, body `{"delay":"15m"|"30m"|"1h"}`; unknown delay → 400 "Unknown pause delay."; no pause rule → 404 "Nothing is paused."; unknown profile → 404 "No such profile."; collapse response `{"removed":true}`; otherwise the stored `FamilyRule` JSON.
- Collapse rule is exact: remove instead of reschedule when `!until.After(newStart.Add(time.Minute))`.
- UI copy is exact: active pausebox button `30 more min`; pending pausebox button `+30 min`. UI always sends `"delay":"30m"`.
- `ls` is aliased on this machine — use `/bin/ls` when you need it.

### Task 2: Frontend — grant buttons in both pauseboxes, and full verification

**Files:**
- Modify: `web/static/js/app.js` (add `grantMore` after `unpause`, ~line 428)
- Modify: `web/static/index.html` (active pausebox ~lines 189–192; pending pausebox ~lines 195–199)
- Modify: `web/static/css/app.css` (after the `.pausebox.scheduled` rule, ~line 491)

**Interfaces:**
- Consumes: `POST /api/pause/{profileId}/delay` with body `{"delay":"30m"}` from Task 1; existing Alpine helpers `api`, `loadCore`, `banner`.
- Produces: Alpine method `grantMore(profileId)`; used only by index.html.

- [ ] **Step 1: Add `grantMore` to `app.js`**

Directly below the `unpause` method, add:

```js
    // grantMore lifts the current pause (or pushes a scheduled one) and
    // re-engages it 30 minutes later, keeping its original end.
    async grantMore(profileId) {
      try { await this.api('POST', '/api/pause/' + profileId + '/delay', { delay: '30m' }); await this.loadCore(); }
      catch (e) { this.banner = e.message; }
    },
```

- [ ] **Step 2: Add the buttons in `index.html`**

Replace the active pausebox block:

```html
            <template x-if="groupPause(p.id)">
              <div class="pausebox">
                <p class="sub">Paused<span x-show="groupPause(p.id).until" x-text="' until ' + fmtTime(groupPause(p.id).until)"></span></p>
                <button class="primary" @click="unpause(p.id)">Resume</button>
              </div>
            </template>
```

with:

```html
            <template x-if="groupPause(p.id)">
              <div class="pausebox">
                <p class="sub">Paused<span x-show="groupPause(p.id).until" x-text="' until ' + fmtTime(groupPause(p.id).until)"></span></p>
                <div class="pausebox-actions">
                  <button class="more" @click="grantMore(p.id)">30 more min</button>
                  <button class="primary" @click="unpause(p.id)">Resume</button>
                </div>
              </div>
            </template>
```

Replace the pending pausebox block:

```html
            <template x-if="groupPending(p.id)">
              <div class="pausebox scheduled">
                <p class="sub" x-text="'Pauses at ' + fmtTime(groupPending(p.id).startsAt)
                  + (groupPending(p.id).until ? ' · until ' + fmtTime(groupPending(p.id).until) : '')"></p>
                <button class="primary" @click="unpause(p.id)">Cancel</button>
              </div>
            </template>
```

with:

```html
            <template x-if="groupPending(p.id)">
              <div class="pausebox scheduled">
                <p class="sub" x-text="'Pauses at ' + fmtTime(groupPending(p.id).startsAt)
                  + (groupPending(p.id).until ? ' · until ' + fmtTime(groupPending(p.id).until) : '')"></p>
                <div class="pausebox-actions">
                  <button class="more" @click="grantMore(p.id)">+30 min</button>
                  <button class="primary" @click="unpause(p.id)">Cancel</button>
                </div>
              </div>
            </template>
```

- [ ] **Step 3: Style the secondary button in `app.css`**

Insert after the `.pausebox.scheduled` rule:

```css
.pausebox-actions { display: flex; align-items: center; gap: 8px; flex: none; }
.pausebox button.more { padding: 10px 14px; font-size: 13px; font-weight: 600; color: var(--text-dim); background: none; border: 1px solid var(--glass-border); border-radius: var(--radius-full); white-space: nowrap; }
.pausebox button.more:hover:not(:disabled) { color: var(--cyan-bright); background: var(--glass-fill); }
```

(The existing `.pausebox button { margin: 0; width: auto; ... }` rule already normalizes both buttons inside the box.)

- [ ] **Step 4: Build and run the full test suite**

Run: `gofmt -l cmd internal && go build ./... && go test ./...`
Expected: `gofmt` prints nothing; build succeeds; all packages PASS.

- [ ] **Step 5: Verify the UI in the browser**

Run: `go run ./cmd/familytime -port 8080 -data /private/tmp/claude-501/-Users-dsandor-Projects-bedtime/18358d72-697b-4c57-9c0f-c8083c1d9c54/scratchpad/ft-preview2.json`
Open `http://localhost:8080/?preview=home` (client-side mock data; API calls are rejected, so verify presence/layout, not clicks):
- The Teens card (active pause) shows **30 more min** beside **Resume**, right-aligned as one group, no wrapping at desktop width.
- The Kids card (scheduled pause) shows **+30 min** beside **Cancel**.
- Zero console errors.
Then stop the server. Use the chrome-devtools MCP tools if no visible browser is available.
