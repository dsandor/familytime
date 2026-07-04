# Task 3 brief — Delayed Quick Pause plan (2026-07-03)

## Global Constraints

- **NEVER commit or push to git** (user rule — overrides this plan template's usual commit steps; none are included, and you must not add any).
- All Go commands run from the repo root: `/Users/dsandor/Projects/bedtime`.
- No new dependencies — Go stdlib and the existing Alpine.js only. The web UI has no build step; `go build ./...` embeds `web/static`.
- API wire values are exact: `delay` accepts `"15m" | "30m" | "1h"` (or absent); `duration` values are unchanged (`15m|30m|1h|morning|indefinite`). The pending JSON field is `startsAt` (RFC3339, `omitempty`).
- UI copy is exact: chip row label `Starting`, chips `Now / in 15 min / in 30 min / in 1 hr`, badge `Scheduled`, pending line `Pauses at <time> · until <time>`, button `Cancel`.
- `ls` is aliased on this machine — use `/bin/ls` when you need it.

### Task 3: Frontend — `Starting` selector, Scheduled state, upcoming list, and final verification

**Files:**
- Modify: `web/static/js/app.js` — state (after line 18), choices (after `whenChoices`, line 35), mock data (`mockPreview` rules array, lines 68–80), home helpers (`groupPause` area, lines 339–342), `upcomingTodayList` (lines 363–378), `pause` (lines 380–383)
- Modify: `web/static/index.html` — Quick pause section (lines 151–189)
- Modify: `web/static/css/app.css` — after the `.pausebox` block (line 481)

**Interfaces:**
- Consumes: `POST /api/pause` body `{profileId, duration, delay?}` and rules-list `startsAt` from Task 2; existing Alpine helpers `pauseRules()`, `fmtTime(rfc)`, `whatLabel(w)`, `profileName(id)`, `api(method, path, body)`, `loadCore()`, `unpause(profileId)`.
- Produces: Alpine members `pauseDelay` (object keyed by profile id), `pauseDelayChoices`, `groupDelay(profileId)`, `setGroupDelay(profileId, id)`, `groupPending(profileId)` — used only within these files.

- [ ] **Step 1: Add state and delay choices in `app.js`**

After `renameDraft: '',` (line 18) add:

```js
    pauseDelay: {},
```

After the `whenChoices` array's closing `],` (line 35) add:

```js
    pauseDelayChoices: [
      { id: 'now', label: 'Now' },
      { id: '15m', label: 'in 15 min' },
      { id: '30m', label: 'in 30 min' },
      { id: '1h',  label: 'in 1 hr' },
    ],
```

- [ ] **Step 2: Add pending helpers and send the delay**

In `app.js`, directly below `groupPause` (after line 342) add:

```js
    // groupPending returns the scheduled-but-not-started pause rule for a profile, or null.
    groupPending(profileId) {
      return this.pauseRules().find(r => r.profileId === profileId && !r.active && r.startsAt) || null;
    },

    groupDelay(profileId) { return this.pauseDelay[profileId] || 'now'; },
    setGroupDelay(profileId, id) { this.pauseDelay[profileId] = id; },
```

Replace `pause` (lines 380–383):

```js
    async pause(profileId, duration) {
      const delay = this.groupDelay(profileId);
      const body = { profileId, duration };
      if (delay !== 'now') body.delay = delay;
      try {
        await this.api('POST', '/api/pause', body);
        this.pauseDelay[profileId] = 'now';
        await this.loadCore();
      }
      catch (e) { this.banner = e.message; }
    },
```

- [ ] **Step 3: Surface pending pauses in the Today strip**

Replace `upcomingTodayList` (lines 363–378) — the recurring mapping is unchanged; scheduled one-time pauses starting later today are merged in:

```js
    upcomingTodayList() {
      const now = new Date();
      const nowM = now.getHours() * 60 + now.getMinutes();
      const today = ['sun','mon','tue','wed','thu','fri','sat'][now.getDay()];
      const recurring = this.nonPauseRules()
        .filter(r => r.enabled && !r.active && r.when.kind === 'recurring' && r.when.start
          && (r.when.days || []).includes(today))
        .map(r => {
          const [h, m] = r.when.start.split(':').map(Number);
          return { id: r.id, name: r.name, groupName: this.profileName(r.profileId) || 'Everyone',
                   label: this.whatLabel(r.what), startM: h * 60 + m,
                   startLabel: new Date(0, 0, 0, h, m).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' }) };
        });
      const pending = this.rules
        .filter(r => r.enabled && !r.active && r.startsAt
          && new Date(r.startsAt).toDateString() === now.toDateString())
        .map(r => {
          const d = new Date(r.startsAt);
          return { id: r.id, name: r.name, groupName: this.profileName(r.profileId) || 'Everyone',
                   label: this.whatLabel(r.what), startM: d.getHours() * 60 + d.getMinutes(),
                   startLabel: d.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' }) };
        });
      return recurring.concat(pending)
        .filter(x => x.startM > nowM)
        .sort((a, b) => a.startM - b.startM);
    },
```

- [ ] **Step 4: Add a pending pause to the preview mock**

In `mockPreview`, append one entry to the `this.rules` array (after the last existing rule object, keeping all existing entries):

```js
        { id: 'r9', profileId: 'teens', name: 'Internet pause',
          what: { type: 'everything' },
          when: { kind: 'onetime', start: hhmm(now + 30 * 60e3), until: new Date(now + 90 * 60e3).toISOString() },
          enabled: true, pause: true, active: false,
          startsAt: new Date(now + 30 * 60e3).toISOString(),
          until: new Date(now + 90 * 60e3).toISOString() },
```

- [ ] **Step 5: Rework the Quick pause card in `index.html`**

Replace the section body (lines 151–189) with:

```html
      <section class="section">
        <div class="section-head">
          <p class="eyebrow">Quick pause</p>
          <p class="sub">Give a group a break — right away, or in a little while.</p>
        </div>
        <template x-for="p in profiles" :key="p.id">
          <div class="card pause-card">
            <div class="pause-card-head">
              <span class="avatar" :style="'--accent:'+(p.color||'#22d3ee')" x-text="p.emoji || '🧒'"></span>
              <div class="grow">
                <h3 x-text="p.name"></h3>
                <p class="sub" x-text="(p.devices?p.devices.length:0) + ((p.devices&&p.devices.length===1)?' device':' devices')"></p>
              </div>
              <span class="badge badge-active" x-show="groupPause(p.id)">Active</span>
              <span class="badge badge-scheduled" x-show="groupPending(p.id)">Scheduled</span>
            </div>

            <template x-if="!groupPause(p.id) && !groupPending(p.id)">
              <div>
                <div class="pause-start">
                  <span class="label">Starting</span>
                  <template x-for="c in pauseDelayChoices" :key="c.id">
                    <button class="chip" :class="{ on: groupDelay(p.id) === c.id }"
                            @click="setGroupDelay(p.id, c.id)" x-text="c.label"></button>
                  </template>
                </div>
                <div class="pause-actions">
                  <button class="chip" @click="pause(p.id,'15m')">15m</button>
                  <button class="chip" @click="pause(p.id,'30m')">30m</button>
                  <button class="chip" @click="pause(p.id,'1h')">1h</button>
                  <button class="chip secondary" @click="pause(p.id,'morning')">Until morning</button>
                  <button class="chip secondary" @click="pause(p.id,'indefinite')"
                          :disabled="groupDelay(p.id) !== 'now'"
                          :title="groupDelay(p.id) !== 'now' ? 'Scheduled pauses need an end time — pick a timed duration.' : ''">Until I resume</button>
                </div>
              </div>
            </template>
            <template x-if="groupPause(p.id)">
              <div class="pausebox">
                <p class="sub">Paused<span x-show="groupPause(p.id).until" x-text="' until ' + fmtTime(groupPause(p.id).until)"></span></p>
                <button class="primary" @click="unpause(p.id)">Resume</button>
              </div>
            </template>
            <template x-if="groupPending(p.id)">
              <div class="pausebox scheduled">
                <p class="sub" x-text="'Pauses at ' + fmtTime(groupPending(p.id).startsAt)
                  + (groupPending(p.id).until ? ' · until ' + fmtTime(groupPending(p.id).until) : '')"></p>
                <button class="primary" @click="unpause(p.id)">Cancel</button>
              </div>
            </template>

            <button class="ghost" @click="goRules(p.id)">Manage rules →</button>
          </div>
        </template>
        <p class="center-note sub" x-show="profiles.length===0">
          Add your first group in <b>Groups &amp; Devices</b> to start pausing internet access.
        </p>
      </section>
```

(The card head, active pausebox, Manage rules button, and empty-state note are byte-identical to today; the changes are the subtitle, the Scheduled badge, the wrapping `x-if` gaining `&& !groupPending(p.id)` plus the `pause-start` row, the disabled state on "Until I resume", and the new scheduled pausebox.)

- [ ] **Step 6: Style the new pieces in `app.css`**

Insert after the `.pausebox button` rule (line 481):

```css
.pause-start { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 16px; }
.pause-start .label { font-size: 11px; font-weight: 700; letter-spacing: .06em; text-transform: uppercase; color: var(--text-dim); margin-right: 2px; }
.pause-start .chip { padding: 6px 12px; font-size: 13px; border-radius: var(--radius-full); }
.pause-start .chip.on { border-color: var(--cyan); background: rgba(34, 211, 238, .12); color: var(--cyan-bright); box-shadow: var(--shadow-glow-cyan); }

.badge.badge-scheduled { background: rgba(251, 191, 36, .15); color: #fcd34d; }
.badge.badge-scheduled::before { content: ""; width: 6px; height: 6px; border-radius: 50%; background: #fbbf24; box-shadow: 0 0 6px rgba(251, 191, 36, .8); }

.pausebox.scheduled { background: rgba(251, 191, 36, .07); border-color: rgba(251, 191, 36, .22); }
```

- [ ] **Step 7: Build and run the full test suite**

Run: `gofmt -l cmd internal && go build ./... && go test ./...`
Expected: `gofmt` prints nothing; build succeeds (embeds the updated static files); all packages PASS, including `internal/e2e`.

- [ ] **Step 8: Verify the UI in the browser**

Run: `go run ./cmd/familytime -port 8080 -data /private/tmp/claude-501/-Users-dsandor-Projects-bedtime/18358d72-697b-4c57-9c0f-c8083c1d9c54/scratchpad/ft-preview.json`
Open `http://localhost:8080/?preview=home` (the preview mode is client-side mock data; no gateway or setup needed) and confirm:
- Each Quick Pause card shows the `Starting` row (`Now` selected by default) above the duration chips.
- Selecting `in 30 min` disables **Until I resume** (dimmed via the global `button:disabled` style).
- The Teens card (mock `r9`) shows the amber **Scheduled** badge and the scheduled pausebox — "Pauses at \<time\> · until \<time\>" with a **Cancel** button — instead of duration chips.
- The "Upcoming today" strip lists the pending pause with its start time.

Then stop the server (Ctrl-C). If a browser isn't available, use the chrome-devtools MCP tools to load the page and screenshot it.
