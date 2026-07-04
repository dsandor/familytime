# Task 10 brief — Bedtime implementation plan

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



### Task 10: Web UI — rules list, add-rule wizard, settings

> Invoke the **frontend-design skill** for this task's visuals (same contract rule as Task 8).

**Files:**
- Modify: `web/static/index.html`, `web/static/js/app.js`, `web/static/css/app.css`

**Interfaces:**
- Consumes: `/api/rules`, `/api/presets`, `/api/settings/*` from Task 6
- Produces: working `rules`, `wizard`, `settings` views — completing the UI

- [ ] **Step 1: Replace the rules placeholder in `index.html`**

```html
  <!-- Rules for one profile -->
  <template x-if="view==='rules'">
    <main>
      <header class="pagehead withback">
        <button class="back" @click="goHome()">‹</button>
        <h1 x-text="'Rules · ' + profileName(rulesProfileId)"></h1>
      </header>

      <template x-for="r in rules" :key="r.id">
        <section class="card row">
          <div class="grow">
            <h2 x-text="r.name"></h2>
            <p class="sub" x-text="ruleSummary(r)"></p>
            <p class="sub blocking" x-show="r.active">⛔ blocking right now</p>
          </div>
          <label class="switch">
            <input type="checkbox" :checked="r.enabled" @change="toggleRule(r)">
            <span class="slider"></span>
          </label>
          <button class="iconbtn danger" title="Delete rule" @click="deleteRule(r)">🗑</button>
        </section>
      </template>

      <p class="center-note sub" x-show="rules.length===0">No rules yet — add the first one.</p>
      <button class="primary" @click="startWizard()">＋ Add a rule</button>
    </main>
  </template>

  <!-- Add-rule wizard -->
  <template x-if="view==='wizard'">
    <main class="card">
      <header class="pagehead withback">
        <button class="back" @click="view='rules'">‹</button>
        <h1 x-text="['What to block?','When?','Name it'][wizard.step-1]"></h1>
      </header>

      <!-- Step 1: what -->
      <section x-show="wizard.step===1">
        <div class="preset-grid">
          <template x-for="p in presets" :key="p.id">
            <button :class="{on: wizard.whatType==='preset' && wizard.presetId===p.id}"
                    @click="wizard.whatType='preset'; wizard.presetId=p.id">
              <span class="pemoji" x-text="p.emoji"></span><span x-text="p.name"></span>
            </button>
          </template>
        </div>
        <label>Whole categories</label>
        <div class="chips">
          <template x-for="c in categories" :key="c.id">
            <button :class="{on: wizard.whatType==='category' && wizard.categoryId===c.id}"
                    @click="wizard.whatType='category'; wizard.categoryId=c.id"
                    x-text="c.emoji + ' ' + c.name"></button>
          </template>
        </div>
        <label>Or something specific</label>
        <div class="chips">
          <button :class="{on: wizard.whatType==='domains'}" @click="wizard.whatType='domains'">🌐 A website</button>
          <button :class="{on: wizard.whatType==='everything'}" @click="wizard.whatType='everything'">📵 All internet</button>
        </div>
        <label x-show="wizard.whatType==='domains'">Website addresses <span class="hint-inline">(one per line)</span>
          <textarea x-model="wizard.domainsText" rows="3" placeholder="coolmathgames.com"></textarea>
        </label>
        <button class="primary" :disabled="!wizardWhatValid()" @click="wizard.step=2">Next →</button>
      </section>

      <!-- Step 2: when -->
      <section x-show="wizard.step===2">
        <div class="chips">
          <template x-for="w in whenChoices" :key="w.id">
            <button :class="{on: wizard.whenChoice===w.id}" @click="pickWhen(w.id)" x-text="w.label"></button>
          </template>
        </div>
        <div x-show="wizard.whenChoice!=='always'">
          <label>Days</label>
          <div class="chips">
            <template x-for="d in ['sun','mon','tue','wed','thu','fri','sat']" :key="d">
              <button :class="{on: wizard.days.includes(d)}" @click="toggleDay(d)"
                      x-text="d[0].toUpperCase()+d.slice(1)"></button>
            </template>
          </div>
          <div class="times">
            <label>From <input type="time" x-model="wizard.start"></label>
            <label>Until <input type="time" x-model="wizard.end"></label>
          </div>
          <p class="hint" x-show="wizard.start > wizard.end">🌙 Overnight — blocks into the next morning. That's fine.</p>
        </div>
        <button class="primary" :disabled="!wizardWhenValid()" @click="wizard.name=suggestName(); wizard.step=3">Next →</button>
      </section>

      <!-- Step 3: name + confirm -->
      <section x-show="wizard.step===3">
        <label>Rule name <input type="text" x-model="wizard.name"></label>
        <p class="sub" x-text="wizardRecap()"></p>
        <p class="bad" x-text="wizard.error"></p>
        <button class="primary" :disabled="!wizard.name.trim()" @click="saveRule()">Create rule</button>
      </section>
    </main>
  </template>

  <!-- Settings -->
  <template x-if="view==='settings'">
    <main>
      <header class="pagehead"><h1>Settings</h1></header>

      <section class="card">
        <h2>Gateway</h2>
        <p class="sub"><span x-text="settings.host"></span> · site <span x-text="settings.siteName"></span></p>
        <label>Gateway address <input type="text" x-model="gwForm.host"></label>
        <label>API key <input type="password" x-model="gwForm.apiKey" placeholder="Paste to change"></label>
        <p class="bad" x-text="gwForm.error"></p>
        <button class="primary" :disabled="!gwForm.host || !gwForm.apiKey" @click="saveGateway()">Update gateway</button>
        <button class="ghost" @click="trustCert()">Trust the gateway's new certificate</button>
        <p class="hint">Use "trust" after a UniFi update changes the gateway's certificate and Bedtime reports a certificate error.</p>
      </section>

      <section class="card">
        <h2>Parent PIN</h2>
        <label>Current PIN <input type="password" inputmode="numeric" maxlength="6" x-model="pinForm.current"></label>
        <label>New PIN <input type="password" inputmode="numeric" maxlength="6" x-model="pinForm.next"></label>
        <p class="bad" x-text="pinForm.error"></p>
        <p class="good" x-text="pinForm.ok"></p>
        <button class="primary" :disabled="!pinForm.current || !/^[0-9]{4,6}$/.test(pinForm.next)" @click="savePin()">Change PIN</button>
      </section>

      <section class="card">
        <p class="sub">Data file: <code x-text="settings.dataPath"></code></p>
        <button class="ghost" @click="logout()">Lock (sign out)</button>
      </section>
    </main>
  </template>
```

- [ ] **Step 2: Add the methods and state to `app.js`**

Add state: `gwForm: { host: '', apiKey: '', error: '' }`, `pinForm: { current: '', next: '', error: '', ok: '' }`, and the `whenChoices` constant + methods:

```js
    whenChoices: [
      { id: 'always',   label: 'Always' },
      { id: 'everyday', label: 'Every day' },
      { id: 'school',   label: 'School nights' },
      { id: 'weekend',  label: 'Weekends' },
      { id: 'custom',   label: 'Custom' },
    ],

    profileName(id) {
      if (id === 'everyone') return 'Everyone';
      const p = this.profiles.find(p => p.id === id);
      return p ? p.name : '';
    },

    async goRules(profileId) {
      this.rulesProfileId = profileId;
      try {
        const all = await this.api('GET', '/api/rules');
        this.rules = all.filter(r => r.profileId === profileId && !r.pause);
        if (!this.presets.length) {
          const cat = await this.api('GET', '/api/presets');
          this.presets = cat.presets; this.categories = cat.categories;
        }
        if (!this.profiles.length) this.profiles = await this.api('GET', '/api/profiles');
      } catch (e) {}
      this.view = 'rules';
    },

    ruleSummary(r) {
      const what = r.what.type === 'preset' ? (this.presets.find(p => p.id === r.what.presetId) || {}).name
        : r.what.type === 'category' ? (this.categories.find(c => c.id === r.what.categoryId) || {}).name
        : r.what.type === 'domains' ? (r.what.domains || []).join(', ')
        : 'All internet';
      const w = r.when;
      const when = w.kind === 'always' ? 'always'
        : w.kind === 'onetime' ? ('until ' + this.fmtTime(w.until))
        : (w.days.length === 7 ? 'every day' : w.days.map(d => d[0].toUpperCase() + d.slice(1)).join(' ')) +
          (w.start ? ` · ${w.start}–${w.end}` : ' · all day');
      return `${what} — ${when}`;
    },

    async toggleRule(r) {
      try {
        await this.api('PUT', '/api/rules/' + r.id,
          { name: r.name, what: r.what, when: r.when, enabled: !r.enabled });
        await this.goRules(this.rulesProfileId);
      } catch (e) { this.banner = e.message; }
    },

    async deleteRule(r) {
      if (!confirm(`Delete "${r.name}"?`)) return;
      try {
        await this.api('DELETE', '/api/rules/' + r.id);
        await this.goRules(this.rulesProfileId);
      } catch (e) { this.banner = e.message; }
    },

    startWizard() {
      this.wizard = {
        step: 1, whatType: '', presetId: '', categoryId: '', domainsText: '',
        whenChoice: 'school', days: ['sun','mon','tue','wed','thu'], start: '20:00', end: '07:00',
        name: '', error: '',
      };
      this.view = 'wizard';
    },

    wizardWhatValid() {
      const w = this.wizard;
      return (w.whatType === 'preset' && w.presetId)
        || (w.whatType === 'category' && w.categoryId)
        || (w.whatType === 'domains' && w.domainsText.trim())
        || w.whatType === 'everything';
    },

    pickWhen(id) {
      const w = this.wizard;
      w.whenChoice = id;
      if (id === 'everyday') w.days = ['sun','mon','tue','wed','thu','fri','sat'];
      if (id === 'school')   w.days = ['sun','mon','tue','wed','thu'];
      if (id === 'weekend')  w.days = ['fri','sat'];
    },

    toggleDay(d) {
      const i = this.wizard.days.indexOf(d);
      if (i >= 0) this.wizard.days.splice(i, 1); else this.wizard.days.push(d);
      this.wizard.whenChoice = 'custom';
    },

    wizardWhenValid() {
      const w = this.wizard;
      return w.whenChoice === 'always' || (w.days.length > 0 && w.start && w.end);
    },

    wizardWhat() {
      const w = this.wizard;
      if (w.whatType === 'domains') {
        return { type: 'domains', domains: w.domainsText.split('\n').map(s => s.trim()).filter(Boolean) };
      }
      return { type: w.whatType, presetId: w.presetId || undefined, categoryId: w.categoryId || undefined };
    },

    wizardWhen() {
      const w = this.wizard;
      if (w.whenChoice === 'always') return { kind: 'always' };
      return { kind: 'recurring', days: [...w.days], start: w.start, end: w.end };
    },

    whatText() {
      const w = this.wizard;
      return w.whatType === 'preset' ? (this.presets.find(p => p.id === w.presetId) || {}).name
        : w.whatType === 'category' ? (this.categories.find(c => c.id === w.categoryId) || {}).name
        : w.whatType === 'domains' ? 'those websites'
        : 'the internet';
    },

    suggestName() {
      const whenText = { always: 'always', everyday: 'every day', school: 'on school nights', weekend: 'on weekends', custom: '' }[this.wizard.whenChoice] || '';
      return `No ${this.whatText()} ${whenText}`.trim();
    },

    wizardRecap() {
      const w = this.wizard;
      const when = w.whenChoice === 'always' ? 'at all times'
        : `${w.days.map(d => d[0].toUpperCase() + d.slice(1)).join(', ')} from ${w.start} to ${w.end}`;
      return `Blocks ${this.whatText()} for ${this.profileName(this.rulesProfileId)}, ${when}.`;
    },

    async saveRule() {
      try {
        await this.api('POST', '/api/rules', {
          profileId: this.rulesProfileId,
          name: this.wizard.name,
          what: this.wizardWhat(),
          when: this.wizardWhen(),
        });
        await this.goRules(this.rulesProfileId);
      } catch (e) { this.wizard.error = e.message; }
    },

    async goSettings() {
      try {
        this.settings = await this.api('GET', '/api/settings');
        this.gwForm = { host: this.settings.host, apiKey: '', error: '' };
        this.pinForm = { current: '', next: '', error: '', ok: '' };
      } catch (e) {}
      this.view = 'settings';
    },

    async saveGateway() {
      this.gwForm.error = '';
      try {
        await this.api('PUT', '/api/settings/gateway', { host: this.gwForm.host, apiKey: this.gwForm.apiKey });
        this.banner = '';
        await this.goSettings();
      } catch (e) { this.gwForm.error = e.message; }
    },

    async trustCert() {
      try { await this.api('POST', '/api/settings/trust-cert'); this.banner = ''; await this.goSettings(); }
      catch (e) { this.gwForm.error = e.message; }
    },

    async savePin() {
      this.pinForm.error = ''; this.pinForm.ok = '';
      try {
        await this.api('PUT', '/api/settings/pin', { currentPin: this.pinForm.current, newPin: this.pinForm.next });
        this.pinForm = { current: '', next: '', error: '', ok: 'PIN changed ✓' };
      } catch (e) { this.pinForm.error = e.message; }
    },

    async logout() {
      await this.api('POST', '/api/logout', {});
      this.state.authed = false;
      this.view = 'login';
    },
```

(Remove the Task 8 `goSettings` stub — this replaces it. `goRules` here replaces the Task 9 stub.) Extend `mockPreview` so `?preview=rules`, `?preview=wizard`, and `?preview=settings` work: seed `this.rulesProfileId='p1'`, `this.rules` with two sample `ruleView` objects, `this.presets`/`this.categories` from sample data, `this.settings = { host: '192.168.0.1', siteName: 'default', dataPath: '/example/bedtime.json' }`, and call `startWizard()` for the wizard preview (then set `view` last).

- [ ] **Step 3: Add the styles to `app.css`**

Append:

```css
/* Rules + wizard */
.pagehead.withback { display: flex; align-items: center; gap: 8px; }
.back { background: none; font-size: 26px; padding: 4px 10px; color: var(--accent); }
.blocking { color: var(--bad); font-weight: 600; }
.iconbtn { background: none; font-size: 18px; padding: 8px; }
.preset-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; margin: 10px 0; }
.preset-grid button { display: flex; flex-direction: column; align-items: center; gap: 6px; padding: 14px 6px; background: var(--card); border: 1.5px solid var(--line); border-radius: 14px; font-size: 14px; }
.preset-grid button.on { border-color: var(--accent); background: var(--accent-soft); }
.pemoji { font-size: 26px; }
textarea { width: 100%; padding: 12px; border: 1.5px solid var(--line); border-radius: 12px; font: inherit; margin-top: 6px; }
.times { display: flex; gap: 14px; }
.times label { flex: 1; }
input[type=time] { width: 100%; padding: 11px; border: 1.5px solid var(--line); border-radius: 12px; font: inherit; margin-top: 6px; }
/* Toggle switch */
.switch { position: relative; display: inline-block; width: 52px; height: 30px; flex: none; }
.switch input { display: none; }
.slider { position: absolute; inset: 0; background: var(--line); border-radius: 999px; transition: .15s; }
.slider::before { content: ""; position: absolute; width: 24px; height: 24px; border-radius: 50%; background: white; top: 3px; left: 3px; transition: .15s; box-shadow: 0 1px 4px rgba(0,0,0,.2); }
.switch input:checked + .slider { background: var(--good); }
.switch input:checked + .slider::before { transform: translateX(22px); }
code { font-size: 13px; word-break: break-all; }
```

- [ ] **Step 4: Verify**

```bash
go build ./... && go vet ./... && go test ./...
go build -o /tmp/bedtime-ui ./cmd/bedtime && /tmp/bedtime-ui --port 8899 --data /tmp/bedtime-ui.json &
```

Open `?preview=rules`, `?preview=wizard`, `?preview=settings` — screenshot each; walk the wizard's three steps in the browser (preset select → school nights → suggested name appears). Kill the process, remove temp files.

---

