# Task 8 brief — Bedtime implementation plan

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



### Task 8: Web UI foundation — shell, setup wizard, PIN login

> **UI tasks (8–10):** invoke the **frontend-design skill** before writing the HTML/CSS. The code below is the *functional contract* — views, Alpine state, bindings, API calls must stay as specified; visual design (colors, spacing, typography, polish) should be elevated by the skill. Target feel: warm, calm, family app — a bedside lamp, not a network console. Mobile-first (parents use phones), max-width card layout, large touch targets.

**Files:**
- Create: `web/static/js/app.js`, `web/static/css/app.css`; replace placeholder `web/static/index.html`; vendor `web/static/js/alpine.min.js`
- Modify: `internal/server/auth.go` (setup hints + env-key fallback), `cmd/bedtime/main.go` (.env loader)
- Test: extend `internal/server/auth_test.go`

**Interfaces:**
- Consumes: the JSON API from Tasks 4–6
- Produces: the SPA shell — Alpine `app` component with `view` routing (`loading|setup|login|home|profiles|profile|wizard|settings`), `api()` fetch wrapper with the global error banner, and the setup/login views. Tasks 9–10 add view templates and methods to these same files.
- Server additions: `GET /api/state` gains `suggestedGateway` + `envApiKey` (only while unconfigured); `POST /api/setup` with empty `apiKey` falls back to the server's `UNIFI_API_KEY` env var (the key never travels to an unauthenticated browser); `POST /api/test-connection` (unconfigured-only) validates gateway + key without saving.

- [ ] **Step 1: Vendor Alpine.js**

```bash
mkdir -p web/static/js web/static/css
curl -sL -o web/static/js/alpine.min.js https://cdn.jsdelivr.net/npm/alpinejs@3.14.9/dist/cdn.min.js
head -c 80 web/static/js/alpine.min.js
```

Expected: minified JS (non-empty, no HTML error page). This is the only network fetch in the whole build; the file is committed-in-place and embedded thereafter.

- [ ] **Step 2: Server-side setup hints (failing test first)**

Add to `internal/server/auth_test.go`:

```go
func TestStateSetupHintsAndEnvKeyFallback(t *testing.T) {
	t.Setenv("UNIFI_API_KEY", "env-key")
	ts, _, st, _ := newTestServer(t)
	c := client(t)
	var out struct {
		Configured  bool   `json:"configured"`
		EnvAPIKey   bool   `json:"envApiKey"`
	}
	resp, _ := c.Get(ts.URL + "/api/state")
	json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if !out.EnvAPIKey {
		t.Error("state should advertise that the server holds an API key")
	}
	// Empty apiKey in setup → server uses the env key.
	r2 := postJSON(t, c, ts.URL+"/api/setup", `{"host":"http://fake","apiKey":"","pin":"1234"}`)
	if r2.StatusCode != 200 {
		t.Fatalf("setup with env key = %d", r2.StatusCode)
	}
	if got := st.Snapshot().Gateway.APIKey; got != "env-key" {
		t.Errorf("stored key = %q, want env-key", got)
	}
}

func TestTestConnectionOnlyBeforeSetup(t *testing.T) {
	ts, _, _, _ := newTestServer(t)
	c := client(t)
	var out struct{ Version string `json:"version"` }
	if code := doJSON(t, c, "POST", ts.URL+"/api/test-connection", `{"host":"http://fake","apiKey":"k"}`, &out); code != 200 || out.Version == "" {
		t.Errorf("test-connection = %d, version = %q", code, out.Version)
	}
	doSetup(t, ts)
	if code := doJSON(t, c, "POST", ts.URL+"/api/test-connection", `{"host":"http://fake","apiKey":"k"}`, nil); code != 409 {
		t.Errorf("test-connection after setup = %d, want 409", code)
	}
}
```

Run: `go test ./internal/server/ -run 'TestStateSetupHints|TestTestConnection' -v` → FAIL.

Then in `internal/server/auth.go`:

1. Replace `handleState` with:

```go
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"configured": s.store.IsConfigured(),
		"authed":     s.validSession(r),
	}
	if !s.store.IsConfigured() {
		out["suggestedGateway"] = guessGateway()
		out["envApiKey"] = os.Getenv("UNIFI_API_KEY") != ""
	}
	writeJSON(w, 200, out)
}

// guessGateway assumes the common home layout: local /24 with the gateway
// at .1. A UDP "dial" sends no packets — it just resolves the local address.
func guessGateway() string {
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP.To4() == nil {
		return ""
	}
	ip := addr.IP.To4()
	return fmt.Sprintf("%d.%d.%d.1", ip[0], ip[1], ip[2])
}
```

(add `"net"` and `"os"` to the imports)

Also add the pre-flight test endpoint to `internal/server/auth.go`, and register it in `routes()` in `server.go` with `s.mux.HandleFunc("POST /api/test-connection", s.handleTestConnection)`:

```go
// handleTestConnection lets the setup wizard verify gateway + key before
// committing anything. Only available while unconfigured — afterwards,
// gateway changes live in Settings behind the PIN.
func (s *Server) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	if s.store.IsConfigured() {
		fail(w, http.StatusConflict, "Bedtime is already set up.")
		return
	}
	var in struct{ Host, APIKey string }
	if err := readJSON(r, &in); err != nil || in.Host == "" {
		fail(w, http.StatusBadRequest, "Gateway address is required.")
		return
	}
	if in.APIKey == "" {
		in.APIKey = os.Getenv("UNIFI_API_KEY")
	}
	fp, err := unifi.FetchCertFingerprint(r.Context(), in.Host)
	if err != nil {
		failErr(w, err)
		return
	}
	version, err := s.newAPI(in.Host, in.APIKey, fp).Version(r.Context())
	if err != nil {
		failErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"version": version})
}
```

2. In `handleSetup`, replace

```go
	if err := readJSON(r, &in); err != nil || in.Host == "" || in.APIKey == "" {
		fail(w, http.StatusBadRequest, "Gateway address and API key are required.")
		return
	}
```

with

```go
	if err := readJSON(r, &in); err != nil || in.Host == "" {
		fail(w, http.StatusBadRequest, "Gateway address is required.")
		return
	}
	if in.APIKey == "" {
		in.APIKey = os.Getenv("UNIFI_API_KEY") // key stays server-side
	}
	if in.APIKey == "" {
		fail(w, http.StatusBadRequest, "An API key is required.")
		return
	}
```

3. In `cmd/bedtime/main.go`, add as the first line of `main()`: `loadDotEnv()`, and the helper (plus `"strings"` import):

```go
// loadDotEnv applies KEY=VALUE lines from ./.env without overriding real
// environment variables. Lets `bedtime` pick up UNIFI_API_KEY for setup.
func loadDotEnv() {
	raw, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && os.Getenv(strings.TrimSpace(k)) == "" {
			os.Setenv(strings.TrimSpace(k), strings.TrimSpace(v))
		}
	}
}
```

Run: `go test ./internal/server/ -v` → PASS.

- [ ] **Step 3: Write the app shell HTML**

Replace `web/static/index.html`:

```html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Bedtime</title>
<link rel="stylesheet" href="/css/app.css">
<script defer src="/js/app.js"></script>
<script defer src="/js/alpine.min.js"></script>
</head>
<body x-data="app" x-cloak>

<div class="shell">
  <!-- Global error banner -->
  <div class="banner" x-show="banner" x-text="banner" @click="banner=''"></div>

  <!-- Loading -->
  <template x-if="view==='loading'">
    <main class="center"><div class="moon">🌙</div></main>
  </template>

  <!-- First-run setup wizard -->
  <template x-if="view==='setup'">
    <main class="card narrow">
      <div class="hero"><span class="moon">🌙</span><h1>Bedtime</h1>
        <p class="sub">Screen-time rules your whole family can live with.</p></div>

      <section x-show="setup.step===1">
        <h2>Connect to your UniFi gateway</h2>
        <label>Gateway address
          <input type="text" x-model="setup.host" placeholder="192.168.0.1">
        </label>
        <template x-if="state.envApiKey">
          <p class="hint">✓ An API key was found on the server — leave the field below empty to use it.</p>
        </template>
        <label>API key <span class="hint-inline">(UniFi console → Settings → Control Plane → Integrations)</span>
          <input type="password" x-model="setup.apiKey" :placeholder="state.envApiKey ? 'Using the server’s key' : 'Paste your API key'">
        </label>
        <button class="primary" :disabled="!setup.host || setup.testing" @click="testConnection()">
          <span x-show="!setup.testing">Test connection</span>
          <span x-show="setup.testing">Connecting…</span>
        </button>
        <p class="good" x-show="setup.testOK">✓ Connected to your gateway</p>
        <p class="bad" x-text="setup.testError"></p>
        <button class="primary" x-show="setup.testOK" @click="setup.step=2">Next: choose a PIN →</button>
      </section>

      <section x-show="setup.step===2">
        <h2>Choose a parent PIN</h2>
        <p class="sub">Kids on your WiFi shouldn't be able to undo bedtime. 4–6 digits.</p>
        <label>PIN <input type="password" inputmode="numeric" maxlength="6" x-model="setup.pin"></label>
        <label>Repeat PIN <input type="password" inputmode="numeric" maxlength="6" x-model="setup.pin2"></label>
        <p class="bad" x-show="setup.pin2 && setup.pin!==setup.pin2">PINs don't match</p>
        <button class="primary" :disabled="!/^[0-9]{4,6}$/.test(setup.pin) || setup.pin!==setup.pin2"
                @click="finishSetup()">Finish setup</button>
      </section>
    </main>
  </template>

  <!-- PIN login -->
  <template x-if="view==='login'">
    <main class="card narrow center">
      <div class="hero"><span class="moon">🌙</span><h1>Bedtime</h1></div>
      <div class="pin-dots">
        <template x-for="i in 6"><span class="dot" :class="{filled: pin.length>=i, ghost: i>6}"></span></template>
      </div>
      <div class="keypad">
        <template x-for="k in ['1','2','3','4','5','6','7','8','9','clr','0','ok']">
          <button :class="k==='ok' ? 'key primary' : 'key'"
                  @click="k==='clr' ? pin='' : (k==='ok' ? login() : (pin.length<6 && (pin+=k)))"
                  x-text="k==='clr' ? '⌫' : (k==='ok' ? '✓' : k)"></button>
        </template>
      </div>
      <p class="bad" x-text="loginError"></p>
    </main>
  </template>

  <!-- Authed views are added in Tasks 9 and 10 -->
  <template x-if="view==='home'">
    <main class="card"><p>Home — Task 9</p></main>
  </template>

  <!-- Bottom navigation (authed only) -->
  <nav class="tabbar" x-show="authedView()">
    <button :class="{on: view==='home'}" @click="goHome()"><span>🏠</span>Home</button>
    <button :class="{on: view==='profiles' || view==='profile'}" @click="goProfiles()"><span>👨‍👩‍👧</span>Family</button>
    <button :class="{on: view==='settings'}" @click="goSettings()"><span>⚙️</span>Settings</button>
  </nav>
</div>
</body>
</html>
```

- [ ] **Step 4: Write the Alpine app core**

Create `web/static/js/app.js`:

```js
document.addEventListener('alpine:init', () => {
  Alpine.data('app', () => ({
    view: 'loading',
    banner: '',
    state: { configured: false, authed: false, suggestedGateway: '', envApiKey: false },

    setup: { step: 1, host: '', apiKey: '', pin: '', pin2: '', testing: false, testOK: false, testError: '' },
    pin: '',
    loginError: '',

    // Populated by Tasks 9–10:
    status: [], profiles: [], devices: [], rules: [],
    presets: [], categories: [], editing: null, wizard: null, settings: {},

    async init() {
      await this.refreshState();
      this.route();
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
      return ['home', 'profiles', 'profile', 'wizard', 'settings'].includes(this.view);
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

    // Stubs — implemented in Tasks 9 and 10.
    async goHome() { this.view = 'home'; },
    async goProfiles() { this.view = 'profiles'; },
    async goSettings() { this.view = 'settings'; },
  }));
});
```

`testConnection` calls the real pre-flight endpoint added in Step 2 — the button genuinely reaches the gateway before the parent commits.

- [ ] **Step 5: Write the base stylesheet**

Create `web/static/css/app.css` (foundation — the frontend-design skill elevates it):

```css
/* Bedtime — warm, calm, family. Mobile-first. */
:root {
  --bg: #f7f4ee;
  --card: #ffffff;
  --ink: #2c2a33;
  --muted: #8b8496;
  --accent: #5b54d6;
  --accent-soft: #ecebfa;
  --good: #2e9e6b;
  --bad: #d64550;
  --line: #eee9e0;
  --radius: 18px;
  --shadow: 0 6px 24px rgba(44, 42, 51, .08);
  font-size: 17px;
}
* { box-sizing: border-box; }
[x-cloak] { display: none !important; }
html, body { margin: 0; min-height: 100vh; }
body {
  background: var(--bg);
  color: var(--ink);
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  -webkit-font-smoothing: antialiased;
}
.shell { max-width: 520px; margin: 0 auto; padding: 16px 16px 96px; }
.center { display: flex; flex-direction: column; align-items: center; justify-content: center; min-height: 70vh; text-align: center; }
.moon { font-size: 56px; display: block; }
.hero { text-align: center; margin-bottom: 20px; }
.hero h1 { margin: 8px 0 4px; font-size: 30px; letter-spacing: -.02em; }
.sub { color: var(--muted); margin: 4px 0 16px; }
.card { background: var(--card); border-radius: var(--radius); box-shadow: var(--shadow); padding: 24px; margin-top: 24px; }
.narrow { max-width: 400px; margin-left: auto; margin-right: auto; }
h2 { font-size: 20px; margin: 8px 0 12px; }
label { display: block; margin: 14px 0 6px; font-weight: 600; font-size: 15px; }
.hint, .hint-inline { color: var(--muted); font-weight: 400; font-size: 13px; }
input[type=text], input[type=password] {
  width: 100%; padding: 13px 14px; margin-top: 6px;
  border: 1.5px solid var(--line); border-radius: 12px;
  font-size: 17px; background: #fdfcfa;
}
input:focus { outline: none; border-color: var(--accent); }
button { font: inherit; cursor: pointer; border: none; border-radius: 12px; padding: 13px 18px; background: var(--accent-soft); color: var(--ink); }
button.primary { background: var(--accent); color: white; font-weight: 600; width: 100%; margin-top: 16px; }
button:disabled { opacity: .45; cursor: default; }
.good { color: var(--good); }
.bad { color: var(--bad); min-height: 1.2em; }
.banner {
  position: sticky; top: 8px; z-index: 10;
  background: #fff3f0; color: #a13c2f; border: 1px solid #f2c9c0;
  padding: 12px 16px; border-radius: 12px; cursor: pointer;
}
/* PIN pad */
.pin-dots { display: flex; gap: 12px; justify-content: center; margin: 24px 0; }
.dot { width: 14px; height: 14px; border-radius: 50%; background: var(--line); }
.dot.filled { background: var(--accent); }
.keypad { display: grid; grid-template-columns: repeat(3, 72px); gap: 12px; justify-content: center; }
.key { height: 64px; font-size: 22px; border-radius: 50%; background: var(--card); box-shadow: var(--shadow); }
.key.primary { background: var(--accent); color: white; }
/* Bottom tab bar */
.tabbar {
  position: fixed; bottom: 0; left: 0; right: 0;
  display: flex; justify-content: space-around;
  background: var(--card); border-top: 1px solid var(--line);
  padding: 8px max(12px, env(safe-area-inset-left)) max(10px, env(safe-area-inset-bottom));
}
.tabbar button { background: none; display: flex; flex-direction: column; align-items: center; gap: 2px; font-size: 12px; color: var(--muted); padding: 6px 18px; }
.tabbar button span { font-size: 22px; }
.tabbar button.on { color: var(--accent); }
```

- [ ] **Step 6: Build and verify in a browser**

```bash
go build ./... && go vet ./... && go test ./...
go build -o /tmp/bedtime-ui ./cmd/bedtime
/tmp/bedtime-ui --port 8899 --data /tmp/bedtime-ui.json &
```

Open `http://localhost:8899` in a browser (chrome-devtools MCP if available: navigate + screenshot). Verify: setup wizard renders with the suggested gateway prefilled; PIN step gates on matching 4–6 digit PINs; no console errors. Then `kill %1; rm -f /tmp/bedtime-ui /tmp/bedtime-ui.json`. **Do not complete real setup against the live gateway in this task** — that happens in Task 11.

---

