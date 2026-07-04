# Bedtime — Design Spec

**Date:** 2026-07-02
**Status:** Approved design, pre-implementation

## What it is

A single Go binary with an embedded web UI that lets parents set time-based app and website blocking for kids' devices on a UniFi Cloud Gateway. Parents think in terms of *kids* and *apps* ("block YouTube for Emma on school nights"); under the covers the app manages UniFi traffic rules with native gateway schedules. Built for non-technical users: beautiful, simple, no jargon.

## Goals

- One-tap blocking of popular apps (curated presets), broad categories, custom websites, and full internet pause.
- Rules target a profile (a kid or a device group), or everyone.
- Schedules: always, every day, school nights, weekends, custom days + time window, and one-time pauses ("30 minutes", "until morning").
- Enforcement lives on the gateway — blocking works even when this app is not running.
- Works both always-on (home server) and launch-on-demand.
- Parent PIN protects the UI from kids on the same network.

## Non-goals

- Daily usage quotas ("1 hour of YouTube per day") — requires an always-on scheduler; excluded by design.
- Usage statistics or monitoring.
- Access from outside the LAN, HTTPS for the app itself, multi-gateway support, multi-user accounts.
- Managing UniFi rules not created by this app.

## Verified facts (probed 2026-07-02 against the real gateway)

- Gateway: UCG Max at `192.168.0.1`, Network app **10.4.57**, firmware 5.1.19, site id `88f7af54-98f8-306a-a1c7-c9349722b1f6` (`internalReference: "default"`), 82 clients.
- **The internal v2 API accepts `X-API-KEY` header auth** on this firmware — no username/password session or CSRF needed. `GET /proxy/network/v2/api/site/default/trafficrules` returns rule JSON with the key from `.env`.
- The official v1 integration API works for `sites`, `clients`, `devices`, but has **no traffic-rule endpoint**, and its `firewall/policies` endpoint returns "Zone Based Firewall is not configured". v1 is therefore used only for reads (client/device inventory); all rule CRUD goes through v2.
- Traffic rules carry a **native schedule object** enforced by the gateway itself:

```json
{
  "action": "ALLOW|BLOCK",
  "description": "kids apps",
  "matching_target": "DOMAIN",
  "domains": [{"domain": "roblox.com", "port_ranges": [], "ports": []}],
  "app_ids": [], "app_category_ids": [],
  "target_devices": [{"network_id": "…", "type": "NETWORK"}],
  "schedule": {
    "mode": "ALWAYS", "date_start": "2026-07-01", "date_end": "2026-07-08",
    "time_all_day": false, "time_range_start": "09:00", "time_range_end": "12:00",
    "repeat_on_days": []
  },
  "enabled": true, "traffic_direction": "TO"
}
```

- Known v2 quirks: successful writes return **200 or 201** interchangeably (treat both as success); there is no GET-by-id (list all, filter client-side).
- **Probe results (2026-07-02, disabled rules created via API and deleted):** `schedule.mode` enums confirmed as `ALWAYS`, `EVERY_DAY`, `EVERY_WEEK` (with `repeat_on_days: ["sun"…"sat"]` lowercase), `ONE_TIME_ONLY` (takes a single `date: "YYYY-MM-DD"` field, not date_start/date_end). Target shapes: `{"client_mac": "…", "type": "CLIENT"}` and `{"type": "ALL_CLIENTS"}`. `matching_target: INTERNET` (full pause) and `APP_CATEGORY` (integer DPI category ids) both accepted. **Time windows crossing midnight (21:00→07:00) are accepted natively.** Captured payloads frozen at `internal/unifi/testdata/trafficrules_probe.json`.

## Architecture

One Go module, one binary, stdlib-first. The only external Go dependency is `golang.org/x/crypto` (bcrypt for the PIN). The UI is embedded with `go:embed`; `go build` is the entire build pipeline — no Node toolchain.

```
cmd/bedtime/main.go     entry point: flags, store load, server start
internal/unifi/         typed UniFi client (v2 trafficrules CRUD, v1 clients/devices reads)
internal/store/         local state: one JSON file, atomic writes, mutex-serialized
internal/rules/         translator: family rule ⇆ UniFi trafficrule payload(s); preset catalog
internal/server/        net/http handlers, PIN sessions, JSON API, janitor
web/                    embedded UI: hand-written HTML/CSS + vendored Alpine.js (~15 KB)
```

Separation of concerns: `unifi` knows HTTP and payload shapes but nothing about families. `rules` knows how a family rule maps to gateway payloads but does no I/O. `store` persists app-only state. `server` orchestrates. Each is independently unit-testable.

### Source of truth

The **gateway** owns enforcement: every family rule exists as one or more real UniFi traffic rules with native schedules. The local JSON store holds only what UniFi cannot: profiles, device assignments, PIN hash, session secret, gateway address, API key, and the mapping from each family rule to its UniFi rule `_id`s.

Every rule this app creates has a description prefixed `[bedtime] `. The app never modifies or deletes a gateway rule without that prefix — rules made by hand in the UniFi UI are invisible to it and safe from it.

## Data model (local store)

Single JSON file, default `<os.UserConfigDir()>/bedtime/bedtime.json`, override with `--data`. Written via temp-file + rename (atomic); file mode 0600 (it contains the API key and PIN hash). One `sync.Mutex` serializes writes.

```jsonc
{
  "version": 1,
  "gateway": { "host": "192.168.0.1", "apiKey": "…", "siteId": "…", "siteName": "default",
               "certFingerprint": "sha256-hex-of-gateway-leaf-cert" },
  "auth": { "pinHash": "$2a$…", "sessionSecret": "base64…" },
  "profiles": [
    { "id": "uuid", "name": "Emma", "emoji": "🦄", "color": "#b57edc",
      "devices": [{ "mac": "aa:bb:cc:dd:ee:ff", "name": "Emma's iPad" }] }
  ],
  "rules": [
    { "id": "uuid", "profileId": "uuid",
      "name": "No YouTube on school nights",
      "what": { "type": "preset", "presetId": "youtube" },   // preset | category | domains | everything
      "when": { "kind": "recurring", "days": ["sun","mon","tue","wed","thu"],
                "start": "20:00", "end": "07:00" },           // or {"kind":"always"} or one-time
      "enabled": true,
      "unifiRuleIds": ["6a45…", "6a46…"] }                    // 1..N gateway rules
  ]
}
```

- **Profile** is the single grouping concept: a kid ("Emma") or a device group ("Game Consoles") — same object. A built-in virtual **Everyone** profile (fixed id `everyone`, not stored) targets all devices.
- **Device inventory is never stored** — the picker lists clients live from the gateway. The only snapshot is the device *name* captured at assignment time (refreshed on profile edits), so an assigned device that's currently offline still renders with a friendly name. MACs are the stable device identity.

## Rule translation (`internal/rules`)

A family rule is *{profile, what, when}*. The translator converts it to 1..N UniFi trafficrule payloads:

- **what → matching target**
  - `preset` → `matching_target: DOMAIN` with the preset's curated domain list (e.g. YouTube → youtube.com, googlevideo.com, ytimg.com, youtu.be, …). Presets live in a Go data file (`presets.go`) so adding one is a small diff. Initial catalog: YouTube, TikTok, Instagram, Snapchat, Roblox, Fortnite, Minecraft, Discord, Twitch, Netflix, Disney+. If domain blocking proves insufficient for a given app, that preset switches to UniFi `app_ids` (DPI signatures) — decided per-preset during the schema-verification pass.
  - `category` → `matching_target: APP_CATEGORY` with `app_category_ids` (**integers**, verified — classic DPI ids: Social Networks 8, Games 10, Streaming Media 4; labels get one visual confirmation in the UniFi UI during E2E). UI offers the curated short list only.
  - `domains` → `matching_target: DOMAIN` with parent-entered domains (normalized: strip scheme/path, lowercase).
  - `everything` → `matching_target: INTERNET` (verified) — the full internet pause.
- **profile → target_devices**: profile device MACs → `[{client_mac, type: "CLIENT"}, …]`. Everyone → the all-clients form.
- **when → schedule**
  - `always` → `mode: ALWAYS`, `time_all_day: true`.
  - Recurring days + window → `EVERY_DAY` / `EVERY_WEEK` + `repeat_on_days` + time range.
  - **Windows crossing midnight** (20:00–07:00): verified — the gateway accepts start > end natively, no splitting needed. `unifiRuleIds` stays a list for future flexibility but currently always holds one id.
  - One-time pause (“30 min”, “until morning”) → `ONE_TIME_ONLY` with a single `date: "YYYY-MM-DD"` field (verified) plus a time range. The **gateway** expires it; no daemon required. Until-morning = ends 07:00 (next day via midnight-crossing range). “Until I turn it back on” → `mode: ALWAYS`, removed on unpause.
- `action` is always `BLOCK`, `enabled` mirrors the family rule's toggle, description is `[bedtime] <rule id> <name>`.

**Schema-verification pass (first implementation step):** create each shape once in the UniFi UI (weekly schedule, one-time, all-devices target, client target, category rule), GET the list back, and freeze the exact JSON as test fixtures. This pins the enum spellings (`EVERY_WEEK`, `ONE_TIME_ONLY`, all-clients targeting, category ids, midnight behavior) to observed reality instead of community folklore.

## UniFi client (`internal/unifi`)

- Base `https://<host>`; header `X-API-KEY`. TLS: the gateway's self-signed cert can't pass CA verification, so it is **pinned trust-on-first-use** — setup captures the leaf cert's SHA-256 fingerprint and every later connection verifies against the pin (never blanket `InsecureSkipVerify` after setup). If the gateway regenerates its cert (e.g. firmware update), requests fail with `ErrCertChanged` and the UI offers a one-tap "trust new certificate" in Settings.
- v2: `GET/POST /proxy/network/v2/api/site/{site}/trafficrules`, `PUT/DELETE …/trafficrules/{id}`. Treat both 200 and 201 as success (PUT-returns-201 quirk).
- v1: `GET /proxy/network/integration/v1/sites`, `…/sites/{id}/clients` (paginated) for device inventory. If v1 only returns currently-connected clients, fall back to v2 client-history endpoints so offline kid devices still appear in the picker — resolved during the verification pass.
- Timeouts (10 s), no retries on writes (parent taps again), one retry on reads. Errors surface as typed values: `ErrUnauthorized` (bad key), `ErrUnreachable`, `ErrNotFound`.

## HTTP server & API (`internal/server`)

Stdlib `net/http` with Go 1.22+ `ServeMux` patterns. Listens on `:8080` by default (`--port` to change), binds all interfaces — LAN use, protected by the PIN.

- `POST /api/setup` — first run only (rejected once configured): gateway host, API key, PIN. Validates by calling the gateway before saving.
- `POST /api/login` {pin} → HMAC-signed session cookie, 30-day expiry. bcrypt-hashed PIN; after 5 consecutive failures, exponential backoff (in-memory).
- `GET /api/status` — profiles with computed live status. The gateway doesn't report "currently enforcing", so status ("YouTube blocked until 07:00") is computed from each rule's schedule vs. the current time.
- `GET /api/devices` — live inventory from gateway, annotated with which profile each device belongs to.
- `GET/POST/PUT/DELETE /api/profiles[/{id}]`
- `GET/POST/PUT/DELETE /api/rules[/{id}]` — writes go: translate → gateway CRUD → store metadata. If the gateway call fails, nothing is stored (gateway-first ordering keeps store ⊆ gateway).
- `POST /api/pause` {profileId, duration: "30m" | "1h" | "morning" | "indefinite"}; `DELETE /api/pause/{profileId}` — sugar over an `everything` rule.
- `GET /api/presets`, `POST /api/test-connection`.
- Everything except `/api/setup`, `/api/login`, and static assets requires a valid session.

All state-changing operations are also guarded server-side by re-checking the `[bedtime]` prefix before PUT/DELETE on the gateway.

### Janitor

At startup and every 5 minutes while running: list gateway `[bedtime]` rules; delete expired one-time rules (cosmetic — enforcement already ended); drop store metadata for rules that vanished from the gateway (someone deleted them in the UniFi UI). Startup-time run means launch-on-demand users get the same tidying.

## Web UI (`web/`)

Hand-written HTML + modern CSS + **Alpine.js vendored** into the repo and embedded — no build step, no CDN (works offline on the LAN). Visual design executed with the frontend-design skill during implementation; target feel: warm, calm, family app — not a network admin tool.

Screens:

1. **Setup wizard** (first run): gateway address prefilled with the detected default gateway, API key prefilled from `UNIFI_API_KEY`/`.env` if present, "Test connection", choose PIN.
2. **PIN pad login** — remembered per browser for 30 days.
3. **Home** — a card per profile: avatar, devices count, live status lines ("📵 Internet paused until 7:00 AM", "YouTube blocked on school nights"), and a prominent **Pause Internet** button (30 min / 1 hour / until morning / until I turn it back on). Unpause is one tap.
4. **Profiles** — create/edit profile, assign devices from the live gateway list (searchable, shows device names and last-seen).
5. **Rules** — per profile, 3-step wizard: *What* (app-icon grid, categories, custom site, everything) → *When* (chips: Always / Every day / School nights / Weekends / Custom) → confirm with auto-suggested name. Existing rules listed with enable/disable toggle and delete.
6. **Settings** — change PIN, gateway connection, view port/data-file location.

## Error handling

- Gateway unreachable → persistent, friendly banner ("Can't reach your UniFi gateway"); UI stays usable read-only from store metadata; writes fail with a clear message.
- 401 from gateway → banner prompting to update the API key in Settings.
- Gateway TLS cert changed (pin mismatch) → banner explaining it ("If you recently updated your UniFi gateway, this is expected") with a one-tap re-trust action in Settings.
- Drift (rule edited/deleted in UniFi UI): deletions are absorbed by the janitor; edits to `[bedtime]` rules are tolerated (the gateway copy wins for enforcement; editing the family rule in Bedtime rewrites it).
- Store corruption (bad JSON) → refuse to start with a clear message naming the file; never silently reinitialize (it holds the PIN and rule mappings).
- Partial multi-rule writes (midnight split, rule 1 of 2 fails): delete the succeeded rule(s), report failure — family rules apply atomically or not at all.

## Testing

- **Unit — translator:** family rule → expected payload JSON for every what/when combination, midnight split, domain normalization, `[bedtime]` tagging. Fixtures are the frozen real-gateway captures.
- **Unit — store:** round-trip, atomic write (no partial file on simulated crash), 0600 perms.
- **Unit — unifi client:** against `httptest.Server` replaying captured responses, including PUT→201 and pagination.
- **Handler tests:** setup-once enforcement, PIN auth + backoff, session cookies, gateway-first write ordering.
- **Live E2E (opt-in, `BEDTIME_E2E=1`):** against the real gateway — create one rule, read it back, delete it. Never run in CI by default.
- **Build verification:** `go build ./...` and `go vet ./...` must pass (embed compiles the UI in); manual browser pass on the real UI before completion.

## Configuration & running

- `bedtime [--port 8080] [--data <path>]`. Everything else is configured in the setup wizard and stored in the JSON file.
- `UNIFI_API_KEY` env var (or `.env` in the working directory) is read only to prefill the setup wizard.
- Cross-compiles for darwin/linux, amd64/arm64 (pure-Go dependencies only).
- Not a git repo yet; per user instructions, no commits/pushes — files are just written to disk.

## Schema verification — status

Resolved by live probing on 2026-07-02 (see Verified facts): schedule enums and one-time shape ✔, all-devices targeting ✔ (`ALL_CLIENTS`), midnight crossing ✔ (native, no splitting), category ids ✔ (integers; no catalog endpoint found at the guessed paths — curated constants used instead), offline devices ✔ (names snapshotted at assignment).

Remaining, deferred to the E2E task:

1. Visual confirmation in the UniFi UI that category ids 8/10/4 carry the expected labels (Social Networks / Games / Streaming Media).
2. Per-preset check that domain blocking actually blocks each app on a test device; escalate a preset to `app_ids` where domains prove insufficient.
