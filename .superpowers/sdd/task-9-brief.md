# Task 9 brief — Bedtime implementation plan

## Global Constraints

- **NEVER run any `git` command.** No `git init`, no commits, no pushes — the user's team handles version control. This overrides the usual commit-per-step workflow: where a normal plan says "commit", instead just re-run the verification commands.
- Project root: `/Users/dsandor/Projects/bedtime`. All commands run from there unless a step says otherwise.
- Module name: `bedtime`. Go `1.24`.
- Only external Go dependency allowed: `golang.org/x/crypto` (bcrypt). Everything else stdlib.
- Every UniFi rule this app creates has a description starting with exactly `[bedtime] `. Never modify or delete a gateway rule whose description lacks that prefix.
- Store file permissions 0600; writes are temp-file + rename (atomic).
- Defaults: port `8080`, data file `<os.UserConfigDir()>/bedtime/bedtime.json`.
- After every task: `go build ./... && go vet ./... && go test ./...` must pass.
- `ls` is aliased on this machine — use `/bin/ls` in shell commands.
- The live gateway is at `https://192.168.0.1` with the API key in `.env` (`UNIFI_API_KEY`). **Do not create/modify/delete anything on the gateway except in Task 10 (opt-in E2E), and never touch rules lacking the `[bedtime]`/`[bedtime-e2e]` prefix.** The user's real rule "kids apps" must survive untouched.



## Verified UniFi v2 API facts (probed live 2026-07-02 — treat as ground truth)

- Base: `https://192.168.0.1/proxy/network/v2/api/site/default/trafficrules`, header `X-API-KEY`, self-signed TLS.
- Writes return **200 or 201** interchangeably. No GET-by-id — list and filter. DELETE returns 200.
- `schedule.mode`: `ALWAYS`, `EVERY_DAY`, `EVERY_WEEK` (+ `repeat_on_days: ["sun".."sat"]`), `ONE_TIME_ONLY` (+ single `date: "YYYY-MM-DD"`).
- Time ranges crossing midnight (`21:00`→`07:00`) are **accepted natively** — no rule splitting.
- `target_devices`: `{"client_mac": "aa:bb:…", "type": "CLIENT"}` or `{"type": "ALL_CLIENTS"}`.
- `matching_target`: `DOMAIN` (with `domains: [{domain, ports: [], port_ranges: []}]`), `APP_CATEGORY` (with `app_category_ids` as **integers**), `INTERNET` (full block).
- Captured real payloads: `internal/unifi/testdata/trafficrules_probe.json` (already in the repo — 4 probe rules covering weekly/midnight/ALL_CLIENTS/one-time shapes).
- Official v1 API (`/proxy/network/integration/v1/…`) works for `info`, `sites`, `sites/{id}/clients` (paginated envelope `{offset,limit,count,totalCount,data}`) — used read-only for device inventory.



### Task 9: Web UI — Home dashboard + Family (profiles & devices)

> Invoke the **frontend-design skill** for this task's visuals (same contract rule as Task 8).

**Files:**
- Modify: `web/static/index.html`, `web/static/js/app.js`, `web/static/css/app.css`

**Interfaces:**
- Consumes: `/api/status`, `/api/profiles`, `/api/devices`, `/api/pause` from Tasks 5–6
- Produces: working `home`, `profiles`, `profile` views; `goRules(profileId)` navigation hook + `rules` placeholder view for Task 10; `?preview=<view>` client-side mock mode for design verification without a configured gateway

- [ ] **Step 1: Replace the home stub and add family views in `index.html`**

Replace the `<template x-if="view==='home'">…</template>` stub with:

```html
  <!-- Home dashboard -->
  <template x-if="view==='home'">
    <main>
      <header class="pagehead"><h1>Tonight</h1><p class="sub" x-text="clockLine()"></p></header>

      <template x-for="p in status" :key="p.id">
        <section class="card profile-card">
          <div class="profile-head">
            <span class="avatar" :style="p.color ? 'background:'+p.color : ''" x-text="p.emoji || '👤'"></span>
            <div>
              <h2 x-text="p.name"></h2>
              <p class="sub" x-text="p.id==='everyone' ? 'All devices on your network' : (p.deviceCount + (p.deviceCount===1 ? ' device' : ' devices'))"></p>
            </div>
          </div>

          <div class="pausebox" x-show="p.paused">
            <p>📵 Internet paused<span x-show="p.pausedUntil" x-text="' until ' + fmtTime(p.pausedUntil)"></span></p>
            <button class="primary" @click="unpause(p.id)">Resume internet</button>
          </div>

          <ul class="status-lines" x-show="p.lines.some(l => !l.pause)">
            <template x-for="l in p.lines.filter(l => !l.pause)" :key="l.ruleId">
              <li :class="{active: l.active}">
                <span x-text="(l.active ? '⛔ ' : '🕐 ') + l.label"></span>
                <span class="sub" x-text="l.active ? (l.until ? 'until ' + fmtTime(l.until) : 'blocking now') : 'scheduled'"></span>
              </li>
            </template>
          </ul>

          <div class="pause-actions" x-show="!p.paused">
            <span class="sub">Pause internet</span>
            <div class="chips">
              <button @click="pause(p.id,'30m')">30 min</button>
              <button @click="pause(p.id,'1h')">1 hour</button>
              <button @click="pause(p.id,'morning')">Until morning</button>
              <button @click="pause(p.id,'indefinite')">Until I resume</button>
            </div>
          </div>

          <button class="ghost" @click="goRules(p.id)">Manage rules →</button>
        </section>
      </template>

      <p class="center-note sub" x-show="profiles.length===0">
        Tip: add a profile for each kid in the <b>Family</b> tab, then set rules per kid.
      </p>
    </main>
  </template>

  <!-- Family: profile list -->
  <template x-if="view==='profiles'">
    <main>
      <header class="pagehead"><h1>Family</h1><p class="sub">Group devices by kid — or by anything ("Game consoles").</p></header>
      <template x-for="p in profiles" :key="p.id">
        <section class="card row clickable" @click="editProfile(p)">
          <span class="avatar" :style="p.color ? 'background:'+p.color : ''" x-text="p.emoji || '👤'"></span>
          <div class="grow">
            <h2 x-text="p.name"></h2>
            <p class="sub" x-text="p.devices.map(d => d.name || d.mac).join(', ') || 'No devices yet'"></p>
          </div>
          <span class="chev">›</span>
        </section>
      </template>
      <button class="primary" @click="newProfile()">＋ Add a profile</button>
    </main>
  </template>

  <!-- Family: profile editor -->
  <template x-if="view==='profile'">
    <main class="card">
      <h2 x-text="editing.id ? 'Edit profile' : 'New profile'"></h2>
      <label>Name <input type="text" x-model="editing.name" placeholder="Emma"></label>

      <label>Icon</label>
      <div class="chips">
        <template x-for="e in ['🧒','👧','👦','🧑','🦄','🐯','🐼','🚀','🎮','📱']">
          <button :class="{on: editing.emoji===e}" @click="editing.emoji=e" x-text="e"></button>
        </template>
      </div>

      <label>Color</label>
      <div class="chips">
        <template x-for="c in ['#f6d7e0','#d7e8f6','#d9f0dd','#f6ecd0','#e6dcf6','#f6ddd0']">
          <button class="swatch" :class="{on: editing.color===c}" :style="'background:'+c" @click="editing.color=c"></button>
        </template>
      </div>

      <label>Devices <span class="hint-inline">(from your gateway — online now or previously assigned)</span></label>
      <ul class="device-list">
        <template x-for="d in devices" :key="d.mac">
          <li :class="{disabled: d.profileId && d.profileId!==editing.id}" @click="toggleDevice(d)">
            <span class="check" x-text="deviceChecked(d) ? '☑' : '☐'"></span>
            <span class="grow">
              <span x-text="d.name"></span>
              <span class="sub" x-text="(d.connected ? (d.wireless ? ' 📶' : ' 🔌') : ' (offline)') + (d.profileId && d.profileId!==editing.id ? ' — in another profile' : '')"></span>
            </span>
          </li>
        </template>
      </ul>

      <p class="bad" x-text="editError"></p>
      <button class="primary" :disabled="!editing.name.trim()" @click="saveProfile()">Save</button>
      <button class="ghost" @click="goProfiles()">Cancel</button>
      <button class="ghost danger" x-show="editing.id" @click="removeProfile()">Delete profile & its rules</button>
    </main>
  </template>

  <!-- Rules view placeholder — implemented in Task 10 -->
  <template x-if="view==='rules'">
    <main class="card"><p>Rules — Task 10</p><button class="ghost" @click="goHome()">‹ Back</button></main>
  </template>
```

- [ ] **Step 2: Add the methods to `app.js`**

Replace the three stub methods (`goHome`, `goProfiles`, `goSettings` stays a stub until Task 10) and add the new state + methods. Add `editError: ''` and `rulesProfileId: ''` to the component state, then:

```js
    async goHome() {
      try {
        [this.status, this.profiles] = await Promise.all([
          this.api('GET', '/api/status'),
          this.api('GET', '/api/profiles'),
        ]);
      } catch (e) { /* banner already set for gateway errors */ }
      this.view = 'home';
    },

    async goProfiles() {
      try { this.profiles = await this.api('GET', '/api/profiles'); } catch (e) {}
      this.view = 'profiles';
    },

    goRules(profileId) {
      this.rulesProfileId = profileId;
      this.view = 'rules'; // real content in Task 10
    },

    clockLine() {
      return new Date().toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' });
    },

    fmtTime(rfc) {
      const d = new Date(rfc);
      const opts = { hour: 'numeric', minute: '2-digit' };
      const sameDay = d.toDateString() === new Date().toDateString();
      return sameDay ? d.toLocaleTimeString(undefined, opts)
                     : d.toLocaleDateString(undefined, { weekday: 'short' }) + ' ' + d.toLocaleTimeString(undefined, opts);
    },

    async pause(profileId, duration) {
      try { await this.api('POST', '/api/pause', { profileId, duration }); await this.goHome(); }
      catch (e) { this.banner = e.message; }
    },

    async unpause(profileId) {
      try { await this.api('DELETE', '/api/pause/' + profileId); await this.goHome(); }
      catch (e) { this.banner = e.message; }
    },

    async loadDevices() {
      this.devices = await this.api('GET', '/api/devices');
    },

    async newProfile() {
      this.editing = { id: null, name: '', emoji: '🧒', color: '#d7e8f6', devices: [] };
      this.editError = '';
      try { await this.loadDevices(); } catch (e) {}
      this.view = 'profile';
    },

    async editProfile(p) {
      this.editing = JSON.parse(JSON.stringify(p));
      this.editError = '';
      try { await this.loadDevices(); } catch (e) {}
      this.view = 'profile';
    },

    deviceChecked(d) {
      return this.editing.devices.some(x => x.mac === d.mac);
    },

    toggleDevice(d) {
      if (d.profileId && d.profileId !== this.editing.id) return; // belongs to another profile
      if (this.deviceChecked(d)) {
        this.editing.devices = this.editing.devices.filter(x => x.mac !== d.mac);
      } else {
        this.editing.devices.push({ mac: d.mac, name: d.name });
      }
    },

    async saveProfile() {
      const body = { name: this.editing.name, emoji: this.editing.emoji, color: this.editing.color, devices: this.editing.devices };
      try {
        if (this.editing.id) await this.api('PUT', '/api/profiles/' + this.editing.id, body);
        else await this.api('POST', '/api/profiles', body);
        await this.goProfiles();
      } catch (e) { this.editError = e.message; }
    },

    async removeProfile() {
      if (!confirm(`Delete ${this.editing.name} and all of their rules?`)) return;
      try {
        await this.api('DELETE', '/api/profiles/' + this.editing.id);
        await this.goProfiles();
      } catch (e) { this.editError = e.message; }
    },
```

Also add the **preview mode** at the top of `init()` (before `refreshState`), so any screen can be designed/verified without a configured gateway:

```js
    async init() {
      const preview = new URLSearchParams(location.search).get('preview');
      if (preview) { this.mockPreview(preview); return; }
      await this.refreshState();
      this.route();
    },

    // mockPreview renders a view with sample data, client-side only — the
    // server still rejects every API call without a real session.
    mockPreview(view) {
      this.state = { configured: true, authed: true };
      this.profiles = [
        { id: 'p1', name: 'Emma', emoji: '🦄', color: '#e6dcf6', devices: [{ mac: 'aa:1', name: "Emma's iPad" }, { mac: 'aa:2', name: 'Switch' }] },
        { id: 'p2', name: 'Jack', emoji: '🚀', color: '#d7e8f6', devices: [{ mac: 'bb:1', name: "Jack's phone" }] },
      ];
      this.status = [
        { id: 'everyone', name: 'Everyone', emoji: '🌍', deviceCount: 0, paused: false, lines: [] },
        { id: 'p1', name: 'Emma', emoji: '🦄', color: '#e6dcf6', deviceCount: 2, paused: false, lines: [
          { ruleId: 'r1', name: 'School nights', label: 'YouTube', active: true, until: new Date(Date.now() + 9 * 3600e3).toISOString(), pause: false },
          { ruleId: 'r2', name: 'Weekend mornings', label: 'Roblox', active: false, pause: false },
        ]},
        { id: 'p2', name: 'Jack', emoji: '🚀', color: '#d7e8f6', deviceCount: 1, paused: true, pausedUntil: new Date(Date.now() + 1800e3).toISOString(), lines: [
          { ruleId: 'r3', name: 'Internet pause', label: 'All internet', active: true, pause: true },
        ]},
      ];
      this.devices = [
        { mac: 'aa:1', name: "Emma's iPad", connected: true, wireless: true, profileId: 'p1' },
        { mac: 'bb:1', name: "Jack's phone", connected: true, wireless: true, profileId: 'p2' },
        { mac: 'cc:1', name: 'Living-room TV', connected: true, wireless: false },
        { mac: 'aa:2', name: 'Switch', connected: false, profileId: 'p1' },
      ];
      this.editing = { id: 'p1', ...this.profiles[0] };
      this.view = view;
    },
```

- [ ] **Step 3: Add the styles to `app.css`**

Append:

```css
/* Pages */
.pagehead { margin: 20px 4px 4px; }
.pagehead h1 { margin: 0; font-size: 26px; letter-spacing: -.02em; }
.center-note { text-align: center; margin-top: 32px; }
/* Profile cards */
.profile-card { padding: 20px; }
.profile-head { display: flex; align-items: center; gap: 14px; }
.avatar { width: 52px; height: 52px; border-radius: 50%; background: var(--accent-soft);
          display: inline-flex; align-items: center; justify-content: center; font-size: 26px; flex: none; }
.profile-head h2 { margin: 0; }
.row { display: flex; align-items: center; gap: 14px; }
.row h2 { margin: 0 0 2px; font-size: 18px; }
.grow { flex: 1; min-width: 0; }
.chev { color: var(--muted); font-size: 22px; }
.clickable { cursor: pointer; }
/* Status lines */
.status-lines { list-style: none; padding: 0; margin: 14px 0 4px; }
.status-lines li { display: flex; justify-content: space-between; gap: 10px; padding: 8px 2px; border-top: 1px solid var(--line); }
.status-lines li.active { font-weight: 600; }
/* Pause */
.pausebox { background: #fdf1ee; border: 1px solid #f2d5cd; border-radius: 12px; padding: 14px; margin-top: 14px; }
.pausebox p { margin: 0 0 10px; font-weight: 600; }
.pause-actions { margin-top: 14px; }
.chips { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
.chips button { padding: 9px 14px; border-radius: 999px; background: var(--accent-soft); font-size: 15px; }
.chips button.on { background: var(--accent); color: white; }
.chips .swatch { width: 36px; height: 36px; border-radius: 50%; border: 2px solid transparent; }
.chips .swatch.on { border-color: var(--ink); }
button.ghost { background: none; color: var(--accent); width: 100%; margin-top: 10px; }
button.ghost.danger { color: var(--bad); }
/* Device picker */
.device-list { list-style: none; padding: 0; margin: 8px 0; max-height: 320px; overflow-y: auto; }
.device-list li { display: flex; gap: 10px; padding: 10px 4px; border-top: 1px solid var(--line); cursor: pointer; }
.device-list li.disabled { opacity: .45; cursor: default; }
.device-list .check { font-size: 18px; }
```

- [ ] **Step 4: Verify**

```bash
go build ./... && go vet ./... && go test ./...
go build -o /tmp/bedtime-ui ./cmd/bedtime && /tmp/bedtime-ui --port 8899 --data /tmp/bedtime-ui.json &
```

Open `http://localhost:8899/?preview=home`, `?preview=profiles`, `?preview=profile` — screenshot each (chrome-devtools MCP). Verify: status lines and pause chips render, device picker shows online/offline/other-profile states, no console errors. Then kill the process and remove the temp files.

---

