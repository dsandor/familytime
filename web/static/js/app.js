document.addEventListener('alpine:init', () => {
  Alpine.data('app', () => ({
    view: 'loading',
    banner: '',
    state: { configured: false, authed: false, suggestedGateway: '', envApiKey: false },

    setup: { step: 1, host: '', apiKey: '', pin: '', pin2: '', testing: false, testOK: false, testError: '' },
    pin: '',
    loginError: '',

    profiles: [], devices: [], rules: [],
    presets: [], categories: [], editing: null, wizard: null, settings: {},
    editError: '', deviceQuery: '', editorDeviceQuery: '', rulesSearch: '', rulesGroupFilter: '',
    gwForm: { host: '', apiKey: '', error: '' },
    pinForm: { current: '', next: '', error: '', ok: '' },
    enroll: null,
    renamingDeviceMac: null,
    renameDraft: '',
    pauseDelay: {},

    accentSwatches: [
      { id: 'cyan', hex: '#22d3ee' },
      { id: 'teal', hex: '#2dd4bf' },
      { id: 'green', hex: '#34d399' },
      { id: 'amber', hex: '#fbbf24' },
      { id: 'pink', hex: '#f472b6' },
      { id: 'blue', hex: '#60a5fa' },
    ],

    whenChoices: [
      { id: 'always',   label: 'Always' },
      { id: 'everyday', label: 'Every day' },
      { id: 'school',   label: 'School nights' },
      { id: 'weekend',  label: 'Weekends' },
      { id: 'custom',   label: 'Custom' },
    ],

    pauseDelayChoices: [
      { id: 'now', label: 'Now' },
      { id: '15m', label: 'in 15 min' },
      { id: '30m', label: 'in 30 min' },
      { id: '1h',  label: 'in 1 hr' },
    ],

    async init() {
      const preview = new URLSearchParams(location.search).get('preview');
      if (preview) { this.mockPreview(preview); return; }
      // The enroll page is visited by the device itself (no session, no
      // normal routing) — a phone opening this address identifies itself
      // to the server by its own connection, so it skips setup/login
      // entirely even on a totally fresh browser.
      if (location.pathname === '/enroll') { await this.initEnroll(); return; }
      await this.refreshState();
      this.route();
    },

    // mockPreview renders a view with sample data, client-side only — the
    // server still rejects every API call without a real session.
    mockPreview(view) {
      this.state = { configured: true, authed: true };
      const now = Math.floor(Date.now() / 1800e3) * 1800e3; // half-hour boundary: preview times (and marketing screenshots) stay clean
      const hhmm = d => new Date(d).toTimeString().slice(0, 5);

      this.profiles = [
        { id: 'kids', name: 'Kids', emoji: '🧒', color: '#22d3ee',
          devices: [{ mac: 'a1:b2:c3:00:00:01', name: "Ava's iPad" }, { mac: 'a1:b2:c3:00:00:02', name: "Noah's Switch" }] },
        { id: 'teens', name: 'Teens', emoji: '🎧', color: '#3b82f6',
          devices: [{ mac: 'a1:b2:c3:00:00:03', name: "Mia's Phone" }] },
      ];
      this.devices = [
        { mac: 'a1:b2:c3:00:00:01', name: "Ava's iPad", connected: true, wireless: true, profileId: 'kids' },
        { mac: 'a1:b2:c3:00:00:02', name: "Noah's Switch", connected: true, wireless: true, profileId: 'kids' },
        { mac: 'a1:b2:c3:00:00:03', name: "Mia's Phone", connected: true, wireless: true, profileId: 'teens' },
        { mac: 'a1:b2:c3:00:00:04', name: "Living Room TV", connected: true, wireless: false, profileId: '' },
      ];
      this.rules = [
        { id: 'r1', profileId: 'kids', name: 'School nights',
          what: { type: 'everything' },
          when: { kind: 'recurring', days: ['sun','mon','tue','wed','thu'], start: hhmm(now - 3600e3), end: hhmm(now + 9 * 3600e3) },
          enabled: true, pause: false, active: true, until: new Date(now + 9 * 3600e3).toISOString() },
        { id: 'r2', profileId: 'kids', name: 'No Roblox during homework',
          what: { type: 'preset', presetId: 'roblox' },
          when: { kind: 'recurring', days: ['mon','tue','wed','thu','fri'], start: hhmm(now + 3600e3), end: hhmm(now + 3 * 3600e3) },
          enabled: true, pause: false, active: false },
        { id: 'r3', profileId: 'teens', name: 'Teens: social media curfew',
          what: { type: 'category', categoryId: 'social' },
          when: { kind: 'recurring', days: ['sun','mon','tue','wed','thu','fri','sat'], start: '22:00', end: '06:00' },
          enabled: true, pause: false, active: false },
        { id: 'r4', profileId: 'teens', name: 'Internet pause',
          what: { type: 'everything' },
          when: { kind: 'onetime', start: hhmm(now), until: new Date(now + 30 * 60e3).toISOString() },
          enabled: true, pause: true, active: true, until: new Date(now + 30 * 60e3).toISOString() },
        // Pending (scheduled) pause — with real data a profile has at most one pause rule.
        { id: 'r9', profileId: 'kids', name: 'Internet pause',
          what: { type: 'everything' },
          when: { kind: 'onetime', start: hhmm(now + 30 * 60e3), until: new Date(now + 90 * 60e3).toISOString() },
          enabled: true, pause: true, active: false,
          startsAt: new Date(now + 30 * 60e3).toISOString(),
          until: new Date(now + 90 * 60e3).toISOString() },
      ];
      this.presets = [
        { id: 'youtube', name: 'YouTube', emoji: '📺', domains: [] },
        { id: 'tiktok', name: 'TikTok', emoji: '🎵', domains: [] },
        { id: 'roblox', name: 'Roblox', emoji: '🧱', domains: [] },
        { id: 'discord', name: 'Discord', emoji: '💬', domains: [] },
        { id: 'netflix', name: 'Netflix', emoji: '🍿', domains: [] },
        { id: 'minecraft', name: 'Minecraft', emoji: '⛏️', domains: [] },
      ];
      this.categories = [
        { id: 'social', name: 'Social Media', emoji: '💬' },
        { id: 'games', name: 'Gaming', emoji: '🎮' },
        { id: 'streaming', name: 'Video Streaming', emoji: '📺' },
      ];
      this.settings = { host: '192.168.0.1', siteName: 'default', dataPath: '/example/familytime.json', enrollUrl: 'http://192.168.0.42:8080/enroll' };
      this.editing = JSON.parse(JSON.stringify(this.profiles[0]));
      this.editorDeviceQuery = '';

      if (view === 'wizard') this.startWizard('kids');
      // "group" is the friendly alias; "groupEditor" is the real view name
      // and already falls through to the generic branch below, but this
      // makes the sample-data reset (query, staged devices) explicit.
      else if (view === 'group' || view === 'groupEditor') {
        this.editorDeviceQuery = '';
        this.view = 'groupEditor';
      }
      else if (view === 'enroll') {
        this.enroll = {
          status: 'found',
          device: { mac: 'a1:b2:c3:9e:7f:21', name: 'iPhone 72:68', ip: '192.168.0.87' },
          groups: this.profiles.map(p => ({ id: p.id, name: p.name, emoji: p.emoji, color: p.color })),
          currentProfileId: '',
          name: 'iPhone 72:68',
          profileId: '',
          error: '',
          saving: false,
          result: null,
        };
        this.view = 'enroll';
      }
      else this.view = view;
    },

    async refreshState() {
      const r = await fetch('/api/state');
      this.state = await r.json();
    },

    route() {
      if (!this.state.configured) {
        this.setup.host = this.state.suggestedGateway || '';
        this.view = 'setup';
      } else if (!this.state.authed) {
        this.view = 'login';
      } else {
        this.goHome();
      }
    },

    authedView() {
      return ['home', 'groups', 'groupEditor', 'rules', 'wizard', 'settings'].includes(this.view);
    },

    // api wraps fetch: JSON in/out, throws {message}, surfaces gateway
    // problems in the global banner, bounces to login on session expiry.
    async api(method, path, body) {
      const opts = { method, headers: {} };
      if (body !== undefined) {
        opts.headers['Content-Type'] = 'application/json';
        opts.body = JSON.stringify(body);
      }
      const r = await fetch(path, opts);
      const data = r.status === 204 ? {} : await r.json().catch(() => ({}));
      if (r.status === 401 && this.view !== 'login') {
        this.state.authed = false;
        this.view = 'login';
        throw new Error(data.error || 'Please enter your PIN.');
      }
      if (!r.ok) {
        if (data.code === 'unreachable' || data.code === 'unauthorized' || data.code === 'cert_changed') {
          this.banner = data.error;
        }
        const e = new Error(data.error || 'Something went wrong.');
        e.code = data.code;
        throw e;
      }
      return data;
    },

    async testConnection() {
      this.setup.testing = true;
      this.setup.testOK = false;
      this.setup.testError = '';
      try {
        await this.api('POST', '/api/test-connection', {
          host: this.setup.host.trim(),
          apiKey: this.setup.apiKey.trim(),
        });
        this.setup.testOK = true;
      } catch (e) {
        this.setup.testError = e.message;
      } finally {
        this.setup.testing = false;
      }
    },

    async finishSetup() {
      try {
        await this.api('POST', '/api/setup', {
          host: this.setup.host.trim(),
          apiKey: this.setup.apiKey.trim(),
          pin: this.setup.pin,
        });
        this.state.configured = true;
        this.state.authed = true;
        this.goHome();
      } catch (e) {
        this.setup.testError = e.message;
        this.setup.step = 1; // most failures are connection/key problems
        this.setup.testOK = false;
      }
    },

    async login() {
      try {
        await this.api('POST', '/api/login', { pin: this.pin });
        this.loginError = '';
        this.pin = '';
        this.state.authed = true;
        this.goHome();
      } catch (e) {
        this.loginError = e.message;
        this.pin = '';
      }
    },

    async logout() {
      await this.api('POST', '/api/logout', {});
      this.state.authed = false;
      this.view = 'login';
    },

    // ---------- data loading ----------

    async loadCore() {
      // Groups + all rules together drive Home, Groups & Devices, and Rules —
      // fetched together so every view can be computed client-side without
      // extra round trips.
      const [profiles, rules] = await Promise.all([
        this.api('GET', '/api/profiles'),
        this.api('GET', '/api/rules'),
      ]);
      this.profiles = profiles;
      this.rules = rules;
    },

    async loadDevices() {
      this.devices = await this.api('GET', '/api/devices');
    },

    async loadPresets() {
      if (this.presets.length) return;
      const cat = await this.api('GET', '/api/presets');
      this.presets = cat.presets;
      this.categories = cat.categories;
    },

    // ---------- navigation ----------

    async goHome() {
      try { await this.loadCore(); } catch (e) { /* banner already set for gateway errors */ }
      this.view = 'home';
    },

    async goGroups() {
      try { await Promise.all([this.loadCore(), this.loadDevices()]); } catch (e) {}
      this.deviceQuery = '';
      this.view = 'groups';
    },

    async goRules(filterProfileId) {
      this.rulesGroupFilter = filterProfileId || '';
      this.rulesSearch = '';
      try {
        await this.loadCore();
        await this.loadPresets();
      } catch (e) { this.banner = e.message; }
      this.view = 'rules';
    },

    async goSettings() {
      try {
        this.settings = await this.api('GET', '/api/settings');
        this.gwForm = { host: this.settings.host, apiKey: '', error: '' };
        this.pinForm = { current: '', next: '', error: '', ok: '' };
      } catch (e) {}
      this.view = 'settings';
    },

    profileName(id) {
      if (id === 'everyone') return 'Everyone';
      const p = this.profiles.find(p => p.id === id);
      return p ? p.name : '';
    },

    groupAccent(id) {
      if (id === 'everyone') return '#67e8f9';
      const p = this.profiles.find(p => p.id === id);
      return (p && p.color) || '#3b82f6';
    },

    groupEmoji(id) {
      if (id === 'everyone') return '🌐';
      const p = this.profiles.find(p => p.id === id);
      return (p && p.emoji) || '🧑';
    },

    fmtTime(rfc) {
      const d = new Date(rfc);
      const opts = { hour: 'numeric', minute: '2-digit' };
      const sameDay = d.toDateString() === new Date().toDateString();
      return sameDay ? d.toLocaleTimeString(undefined, opts)
                     : d.toLocaleDateString(undefined, { weekday: 'short' }) + ' ' + d.toLocaleTimeString(undefined, opts);
    },

    // ---------- Home ----------

    greeting() {
      const h = new Date().getHours();
      const line = h < 5 ? 'Good night.' : h < 12 ? 'Good morning.' : h < 18 ? 'Good afternoon.' : 'Good evening.';
      return line;
    },

    homeSubtitle() {
      return "A quiet dashboard for your family's rules. Pause anything with one tap.";
    },

    nonPauseRules() { return this.rules.filter(r => !r.pause); },
    pauseRules() { return this.rules.filter(r => r.pause); },

    statGroups() {
      const devices = this.profiles.reduce((n, p) => n + (p.devices ? p.devices.length : 0), 0);
      return { count: this.profiles.length, devices };
    },

    statRules() {
      const rs = this.nonPauseRules();
      return { count: rs.length, active: rs.filter(r => r.active).length };
    },

    statPauses() {
      return { active: this.pauseRules().filter(r => r.active).length };
    },

    // groupPause returns the active pause rule for a profile, or null.
    groupPause(profileId) {
      return this.pauseRules().find(r => r.profileId === profileId && r.active) || null;
    },

    // groupPending returns the scheduled-but-not-started pause rule for a profile, or null.
    groupPending(profileId) {
      return this.pauseRules().find(r => r.profileId === profileId && !r.active && r.startsAt) || null;
    },

    groupDelay(profileId) { return this.pauseDelay[profileId] || 'now'; },
    setGroupDelay(profileId, id) { this.pauseDelay[profileId] = id; },

    whatLabel(w) {
      if (w.type === 'preset') return (this.presets.find(p => p.id === w.presetId) || {}).name || w.presetId;
      if (w.type === 'category') return (this.categories.find(c => c.id === w.categoryId) || {}).name || w.categoryId;
      if (w.type === 'domains') return (w.domains || []).join(', ');
      return 'All internet';
    },

    activeNowList() {
      return this.rules
        .filter(r => r.enabled && r.active)
        .map(r => ({
          id: r.id,
          label: r.pause ? 'Internet pause' : ('Blocks ' + this.whatLabel(r.what)),
          groupName: this.profileName(r.profileId) || 'Everyone',
          until: r.until,
        }))
        .sort((a, b) => (a.until || '').localeCompare(b.until || ''));
    },

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

    async unpause(profileId) {
      try { await this.api('DELETE', '/api/pause/' + profileId); await this.loadCore(); }
      catch (e) { this.banner = e.message; }
    },

    // grantMore lifts the current pause (or pushes a scheduled one) and
    // re-engages it 30 minutes later, keeping its original end.
    async grantMore(profileId) {
      try { await this.api('POST', '/api/pause/' + profileId + '/delay', { delay: '30m' }); await this.loadCore(); }
      catch (e) { this.banner = e.message; }
    },

    // ---------- Groups & Devices ----------

    filteredDevices() {
      const q = this.deviceQuery.trim().toLowerCase();
      if (!q) return this.devices;
      return this.devices.filter(d => (d.name || '').toLowerCase().includes(q) || (d.mac || '').toLowerCase().includes(q));
    },

    // memberSummary is the Groups list's secondary line — the point is to
    // make membership visible without opening the editor, so it lists
    // names rather than just a count.
    memberSummary(p) {
      const devices = (p && p.devices) || [];
      return devices.length ? devices.map(d => d.name).join(', ') : 'No devices';
    },

    newGroup() {
      this.editing = { id: null, name: '', emoji: '🧒', color: '#22d3ee', devices: [] };
      this.editError = '';
      this.editorDeviceQuery = '';
      this.view = 'groupEditor';
      this.loadDevices().catch(() => {}); // needed for the "add a device" picker
    },

    editGroup(p) {
      this.editing = JSON.parse(JSON.stringify(p));
      this.editError = '';
      this.editorDeviceQuery = '';
      this.view = 'groupEditor';
      this.loadDevices().catch(() => {}); // refresh online status + who's unassigned
    },

    // ---------- Group editor: device membership (staged, committed on Save) ----------

    // editorDeviceOnline looks up connection status from the live devices[]
    // list — `editing.devices` itself only carries {mac, name}.
    editorDeviceOnline(mac) {
      const d = this.devices.find(d => d.mac === mac);
      return !!(d && d.connected);
    },

    removeEditorDevice(mac) {
      this.editing.devices = (this.editing.devices || []).filter(d => d.mac !== mac);
      if (this.renamingDeviceMac === mac) this.renamingDeviceMac = null;
    },

    // ---------- Group editor: inline device rename (staged, committed on Save) ----------
    // Only one row is ever in rename mode at a time. The new name is staged
    // straight onto editing.devices[i].name — nothing is sent to the server
    // until saveGroup() persists the whole profile, at which point the
    // backend's own name-diff pushes the rename to UniFi (best-effort).

    startRenameDevice(d) {
      this.renamingDeviceMac = d.mac;
      this.renameDraft = d.name;
    },

    // commitRenameDevice applies the staged draft (trimmed; a blank draft
    // leaves the name unchanged rather than clearing it) and exits rename
    // mode. Guarded on renamingDeviceMac still matching d.mac so a stray
    // blur event fired by the input's own removal from the DOM (e.g. right
    // after Escape already canceled) can't re-apply a commit.
    commitRenameDevice(d) {
      if (this.renamingDeviceMac !== d.mac) return;
      const val = this.renameDraft.trim();
      if (val) d.name = val;
      this.renamingDeviceMac = null;
    },

    cancelRenameDevice() {
      this.renamingDeviceMac = null;
    },

    // editorDeviceInOtherGroup: true only for devices assigned to a
    // *different* profile than the one being edited (unassigned, or
    // already staged into this group, is fine). Always coerced to a real
    // boolean — Alpine's :disabled binding sets the attribute for any
    // non-`false` value, so returning '' (the natural result of
    // `d.profileId && ...` when profileId is '') left every row disabled,
    // including unassigned ones. Caught via the a11y snapshot, not the
    // screenshot, since the CSS looks the same either way.
    editorDeviceInOtherGroup(d) {
      return !!(d.profileId && d.profileId !== this.editing.id);
    },

    // addEditorDevice stages a device into the group being edited. Devices
    // that belong to another group are refused here too, mirroring the
    // :disabled state in the template — the backend would reject them on
    // Save anyway ("already belongs to X"), so this is just belt-and-braces
    // against a stale click.
    addEditorDevice(d) {
      if (this.editorDeviceInOtherGroup(d)) return;
      if ((this.editing.devices || []).some(x => x.mac === d.mac)) return;
      this.editing.devices.push({ mac: d.mac, name: d.name });
    },

    // editorAddableDevices is the live devices[] list filtered down to the
    // "add a device" search: excludes devices already staged into this
    // group; devices belonging to another group are still listed (so
    // parents can see where a device went) but rendered disabled.
    editorAddableDevices() {
      const q = this.editorDeviceQuery.trim().toLowerCase();
      const inGroup = new Set((this.editing.devices || []).map(d => d.mac));
      return this.devices
        .filter(d => !inGroup.has(d.mac))
        .filter(d => !q || (d.name || '').toLowerCase().includes(q) || (d.mac || '').toLowerCase().includes(q));
    },

    async saveGroup() {
      // The editor no longer shows a device picker (that now lives on the
      // Devices list below) — devices travel through unchanged.
      const body = { name: this.editing.name, emoji: this.editing.emoji, color: this.editing.color, devices: this.editing.devices || [] };
      try {
        if (this.editing.id) await this.api('PUT', '/api/profiles/' + this.editing.id, body);
        else await this.api('POST', '/api/profiles', body);
        await this.goGroups();
      } catch (e) { this.editError = e.message; }
    },

    async removeGroup() {
      if (!confirm(`Delete ${this.editing.name} and all of its rules?`)) return;
      try {
        await this.api('DELETE', '/api/profiles/' + this.editing.id);
        await this.goGroups();
      } catch (e) { this.editError = e.message; }
    },

    async removeGroupFromList(p) {
      if (!confirm(`Delete ${p.name} and all of its rules?`)) return;
      try {
        await this.api('DELETE', '/api/profiles/' + p.id);
        await this.goGroups();
      } catch (e) { this.banner = e.message; }
    },

    // reassignDevice moves a device between groups: remove it from its old
    // group's device array (if any), then add it to the new one (if any),
    // as two PUTs — remove-before-add so the backend's "already belongs to
    // another profile" guard never trips.
    //
    // Partial-failure handling: if the first PUT (remove from old) fails,
    // nothing changed on the backend — surface the error as before. If the
    // second PUT (add to new) fails, the device has *already* been removed
    // from `old` on the backend, so we attempt a compensating PUT to put it
    // back where it was, rather than leaving it stranded unassigned.
    async reassignDevice(device, newProfileId) {
      const oldProfileId = device.profileId || '';
      newProfileId = newProfileId || '';
      if (newProfileId === oldProfileId) return;
      const old = oldProfileId ? this.profiles.find(p => p.id === oldProfileId) : null;
      const next = newProfileId ? this.profiles.find(p => p.id === newProfileId) : null;

      try {
        if (old) {
          await this.api('PUT', '/api/profiles/' + old.id, {
            name: old.name, emoji: old.emoji, color: old.color,
            devices: old.devices.filter(d => d.mac !== device.mac),
          });
        }
      } catch (e) {
        this.banner = e.message;
        await this.loadDevices(); // reset the dropdown to the real state
        return;
      }

      if (next) {
        try {
          await this.api('PUT', '/api/profiles/' + next.id, {
            name: next.name, emoji: next.emoji, color: next.color,
            devices: [...next.devices, { mac: device.mac, name: device.name }],
          });
        } catch (addErr) {
          // remove-before-add already pulled the device out of `old` (if
          // any) above; the add to `next` failed, so the device is
          // currently unassigned on the backend unless we can roll it back.
          if (old) {
            try {
              await this.api('PUT', '/api/profiles/' + old.id, {
                name: old.name, emoji: old.emoji, color: old.color,
                devices: [...old.devices.filter(d => d.mac !== device.mac), { mac: device.mac, name: device.name }],
              });
              this.banner = `Couldn't move ${device.name} — it's back in ${old.name}. Check your connection and try again.`;
            } catch (rollbackErr) {
              this.banner = `${device.name} was removed from ${old.name} but couldn't be added to ${next.name} — it's currently unassigned. Pick its group again.`;
            }
          } else {
            // Unassigned as source: there was no first PUT, so there's
            // nothing to roll back — just surface the add failure.
            this.banner = addErr.message;
          }
        }
      }

      // Always refresh, regardless of which branch above ran, so the UI
      // reflects the real backend state (happy path, rolled-back, or
      // stranded-unassigned).
      await Promise.all([this.loadCore(), this.loadDevices()]);
    },

    // ---------- Rules ----------

    filteredRules() {
      const q = this.rulesSearch.trim().toLowerCase();
      return this.nonPauseRules().filter(r => {
        if (this.rulesGroupFilter && r.profileId !== this.rulesGroupFilter) return false;
        if (!q) return true;
        const hay = [r.name, this.whatLabel(r.what), this.profileName(r.profileId)].join(' ').toLowerCase();
        return hay.includes(q);
      });
    },

    ruleSummary(r) {
      const what = this.whatLabel(r.what);
      const w = r.when;
      const when = w.kind === 'always' ? 'always'
        : w.kind === 'onetime' ? ('until ' + this.fmtTime(w.until))
        : (w.days.length === 7 ? 'every day' : w.days.map(d => d[0].toUpperCase() + d.slice(1)).join(' ')) +
          (w.start ? ` · ${w.start}–${w.end}` : ' · all day');
      return `Blocks ${what} · ${when}`;
    },

    async toggleRule(r) {
      try {
        await this.api('PUT', '/api/rules/' + r.id,
          { name: r.name, what: r.what, when: r.when, enabled: !r.enabled });
        await this.loadCore();
      } catch (e) { this.banner = e.message; }
    },

    async deleteRule(r) {
      if (!confirm(`Delete "${r.name}"?`)) return;
      try {
        await this.api('DELETE', '/api/rules/' + r.id);
        await this.loadCore();
      } catch (e) { this.banner = e.message; }
    },

    // ---------- Rule wizard (Who → What → When → Name) ----------
    // "Who" only appears when creating a rule — the backend never lets an
    // edit change which profile a rule targets, so editing jumps straight
    // to "What" instead of showing a picker that would silently do nothing.

    wizardSteps() {
      return this.wizard && this.wizard.mode === 'edit' ? ['what', 'when', 'name'] : ['who', 'what', 'when', 'name'];
    },

    currentWizardStepKey() {
      return this.wizardSteps()[this.wizard.step - 1];
    },

    wizardStepTitle() {
      const titles = { who: 'Who is this for?', what: 'What to block?', when: 'When?', name: 'Name it' };
      return titles[this.currentWizardStepKey()] || '';
    },

    wizardBack() {
      if (this.wizard.step > 1) { this.wizard.step--; return; }
      this.view = 'rules';
    },

    startWizard(prefillProfileId) {
      this.wizard = {
        mode: 'create', editingId: null, step: 1,
        profileId: prefillProfileId || '',
        whatType: '', presetId: '', categoryId: '', domainsText: '',
        whenChoice: 'school', days: ['sun','mon','tue','wed','thu'], start: '20:00', end: '07:00',
        name: '', error: '', saving: false,
      };
      this.loadPresets().catch(() => {});
      this.view = 'wizard';
    },

    editRule(r) {
      const w = r.what, whn = r.when;
      let whenChoice = 'custom';
      if (whn.kind === 'always') whenChoice = 'always';
      else if (whn.kind === 'recurring' && whn.days.length === 7) whenChoice = 'everyday';
      else if (whn.kind === 'recurring' && whn.days.join(',') === ['sun','mon','tue','wed','thu'].join(',')) whenChoice = 'school';
      else if (whn.kind === 'recurring' && whn.days.join(',') === ['fri','sat'].join(',')) whenChoice = 'weekend';

      this.wizard = {
        mode: 'edit', editingId: r.id, step: 1,
        profileId: r.profileId,
        whatType: w.type, presetId: w.presetId || '', categoryId: w.categoryId || '',
        domainsText: (w.domains || []).join('\n'),
        whenChoice, days: whn.days ? [...whn.days] : [], start: whn.start || '20:00', end: whn.end || '07:00',
        name: r.name, error: '', saving: false,
      };
      this.loadPresets().catch(() => {});
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
      return `Blocks ${this.whatText()} for ${this.profileName(w.profileId)}, ${when}.`;
    },

    async saveRule() {
      if (this.wizard.saving) return; // guard against double-taps duplicating the rule
      this.wizard.saving = true;
      try {
        if (this.wizard.mode === 'edit') {
          await this.api('PUT', '/api/rules/' + this.wizard.editingId, {
            name: this.wizard.name, what: this.wizardWhat(), when: this.wizardWhen(),
          });
        } else {
          await this.api('POST', '/api/rules', {
            profileId: this.wizard.profileId, name: this.wizard.name,
            what: this.wizardWhat(), when: this.wizardWhen(),
          });
        }
        await this.loadCore();
        this.view = 'rules';
      } catch (e) {
        this.wizard.error = e.message;
        this.wizard.saving = false;
      }
    },

    // ---------- Settings ----------

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

    // ---------- Enroll (device self-service, no session) ----------
    // Visited by the device being enrolled, not a parent's browser — the
    // server identifies "this device" from the connection itself, so
    // there's nothing here that needs login state.

    async initEnroll() {
      this.view = 'enroll';
      this.enroll = { status: 'loading', device: null, groups: [], currentProfileId: '', name: '', profileId: '', error: '', saving: false, result: null };
      await this.loadEnrollWhoami();
    },

    async loadEnrollWhoami() {
      this.enroll.status = 'loading';
      this.enroll.error = '';
      try {
        const r = await fetch('/api/enroll/whoami');
        if (r.status === 409) { this.enroll.status = 'unconfigured'; return; }
        const data = await r.json();
        this.enroll.groups = data.groups || [];
        if (data.found) {
          this.enroll.device = { mac: data.mac, name: data.name, ip: data.ip };
          this.enroll.currentProfileId = data.currentProfileId || '';
          this.enroll.profileId = data.currentProfileId || '';
          this.enroll.name = data.currentDeviceName || data.name || '';
          this.enroll.status = 'found';
        } else {
          this.enroll.status = 'notfound';
        }
      } catch (e) {
        this.enroll.status = 'notfound';
      }
    },

    enrollGroupName(id) {
      const g = (this.enroll.groups || []).find(g => g.id === id);
      return g ? g.name : '';
    },

    async submitEnroll() {
      if (this.enroll.saving) return; // guard against double-taps
      this.enroll.saving = true;
      this.enroll.error = '';
      try {
        const r = await fetch('/api/enroll', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name: this.enroll.name.trim(), profileId: this.enroll.profileId }),
        });
        const data = await r.json().catch(() => ({}));
        if (!r.ok) throw new Error(data.error || 'Something went wrong.');
        this.enroll.result = data;
        this.enroll.status = 'done';
      } catch (e) {
        this.enroll.error = e.message;
      } finally {
        this.enroll.saving = false;
      }
    },
  }));
});
