# Task 11 Report — Live E2E, README, cross-compile, final verification

## Step 1-2: Opt-in live E2E test

Created `internal/e2e/e2e_test.go` verbatim from the brief.

Baseline gateway listing before any test activity (only the user's own rule):
```
6a45406c47ab4d6708128d22 | kids apps | True
```

Ran live against the real UCG Max gateway:
```
$ set -a; source .env; set +a; BEDTIME_E2E=1 go test ./internal/e2e/ -v
=== RUN   TestLiveGatewayRuleLifecycle
--- PASS: TestLiveGatewayRuleLifecycle (0.38s)
PASS
ok  	bedtime/internal/e2e	0.622s
```

Post-test gateway listing — clean, no `[bedtime-e2e]` leftovers:
```
6a45406c47ab4d6708128d22 | kids apps | True
```

## Adapted walk-through (agent-executed via chrome-devtools MCP)

Built the binary into the scratchpad and ran it from the project root (so it
picks up `.env`) on port 8899 with a scratch data file:
```
go build -o <scratch>/bedtime-e2e-bin ./cmd/bedtime
<scratch>/bedtime-e2e-bin --port 8899 --data <scratch>/bedtime-e2e-data.json
```
(Note: found and killed an unrelated leftover process from an earlier session
already bound to :8899, with no data file ever written — confirmed harmless/
unconfigured before killing it, to free the port for this run.)

**Setup** — gateway address prefilled `192.168.0.1`; "✓ An API key was found
on the server" shown; left the API key field empty; "Test connection" →
"✓ Connected to your gateway"; set PIN `1234`; landed on Home.

**Profile** — device picker only lists real gateway clients, so created the
profile via a same-origin authenticated `fetch()` from the browser console
(equivalent to the brief's curl-with-cookie approach, but works around the
session cookie being `HttpOnly`):
```js
fetch('/api/profiles', {method:'POST', headers:{'Content-Type':'application/json'},
  body: JSON.stringify({name:'E2E Test', emoji:'🧪', color:'',
    devices:[{mac:'de:ad:be:ef:00:99', name:'E2E bogus'}]})})
```
→ `{"id":"24d0e807bb3be399","name":"E2E Test",...}`. Confirmed on Home/Family
tabs (1 device, bogus MAC only).

**Rule create** — in the browser, opened E2E Test → "Manage rules" → wizard:
picked YouTube preset, "School nights" (defaults Sun-Thu 20:00-07:00 already
matched), name auto-suggested exactly "No YouTube on school nights" →
"Create rule". Verified on the gateway via `GET trafficrules` with
`X-API-KEY`:
```json
{
  "description": "[bedtime] 491591d399f384b7 No YouTube on school nights",
  "matching_target": "DOMAIN",
  "domains": ["youtube.com","youtu.be","googlevideo.com","ytimg.com",
              "youtube-nocookie.com","youtubei.googleapis.com"],
  "schedule": {"mode":"EVERY_WEEK","repeat_on_days":["sun","mon","tue","wed","thu"],
               "time_range_start":"20:00","time_range_end":"07:00"},
  "target_devices": [{"client_mac":"de:ad:be:ef:00:99","type":"CLIENT"}],
  "enabled": true
}
```
Exact match to spec. The UI correctly showed "⛔ blocking right now" — it
was 02:22 AM Friday, which falls in Thursday's overnight window.

**Toggle off** — unchecked the rule's enable checkbox in the UI. Gateway
confirmed:
```
6a47550547ab4d670819e3ae | [bedtime] 491591d399f384b7 No YouTube on school nights | enabled: False
```

**Pause "until morning"** — tapped it for E2E Test on Home; UI showed
"📵 Internet paused until 7:00 AM". Gateway confirmed correct start-day
anchoring (today's date, not the crossed-midnight day):
```json
{
  "description": "[bedtime] 513195de4d987627 Internet pause",
  "matching_target": "INTERNET",
  "schedule": {"mode":"ONE_TIME_ONLY","date":"2026-07-03",
               "time_range_start":"02:23","time_range_end":"07:00"},
  "target_devices": [{"client_mac":"de:ad:be:ef:00:99","type":"CLIENT"}],
  "enabled": true
}
```
This confirms `translateSchedule`'s existing start-day anchoring logic (in
`internal/rules/translate.go`) is already correct — no code change needed.

**Unpause** — "Resume internet" in the UI. Gateway confirmed the pause rule
is gone, leaving only "kids apps" and the (disabled) YouTube rule:
```
6a45406c47ab4d6708128d22 | kids apps | enabled: True
6a47550547ab4d670819e3ae | [bedtime] 491591d399f384b7 No YouTube on school nights | enabled: False
```

**Settings / trust-cert** — Settings showed gateway `192.168.0.1` · site
`default` and the scratch data file path. Clicked "Trust the gateway's new
certificate" — no error banner, page data refreshed cleanly (no-op re-pin
since the cert hadn't actually changed).

**Screenshot** — re-enabled the YouTube rule via the UI to capture a
meaningful live-status screenshot of Home showing "⛔ YouTube — until
7:00 AM" under the E2E Test profile card, alongside the Everyone profile's
pause controls.

**Delete profile** — deleted "E2E Test" from its profile editor ("Delete
profile & its rules"). Note: the browser's native `confirm()` dialog was
unreliable through the chrome-devtools MCP tool's dialog-handling path (the
click, followed by `handle_dialog accept`, left the dialog re-appearing
without ever firing the DELETE call — confirmed via network log showing zero
DELETE requests across two attempts). Worked around it by monkey-patching
`window.confirm = () => true` before the click, which is a browser-side
workaround for the tool's dialog plumbing, not an application bug. After
that, `/api/profiles` returned `[]` and the gateway confirmed only
"kids apps" remained — both the profile's rules (the YouTube rule and any
pause) were removed from the gateway along with the profile.

## Final gateway cleanliness

```
$ curl -sk -H "X-API-KEY: $UNIFI_API_KEY" \
    https://192.168.0.1/proxy/network/v2/api/site/default/trafficrules
[only "kids apps", action ALLOW, enabled:true — untouched, exactly as at the start]
```

Server killed (`pkill`), scratch binary/data/log removed, port 8899
confirmed free.

## Step 4: README.md

Created `/Users/dsandor/Projects/bedtime/README.md` verbatim from the brief.

## Step 5: gofmt + cross-compile + full suite

```
$ gofmt -l . ; test -z "$(gofmt -l .)" && echo "gofmt: CLEAN"
gofmt: CLEAN

$ GOOS=darwin GOARCH=arm64 go build -o /dev/null ./cmd/bedtime   → OK
$ GOOS=darwin GOARCH=amd64 go build -o /dev/null ./cmd/bedtime   → OK
$ GOOS=linux  GOARCH=arm64 go build -o /dev/null ./cmd/bedtime   → OK
$ GOOS=linux  GOARCH=amd64 go build -o /dev/null ./cmd/bedtime   → OK

$ go build ./... && go vet ./... && go test ./...
ok  	bedtime/internal/e2e	   0.223s  (skipped: BEDTIME_E2E unset)
ok  	bedtime/internal/rules	   (cached)
ok  	bedtime/internal/server   (cached)
ok  	bedtime/internal/store	   (cached)
ok  	bedtime/internal/unifi	   (cached)
```

All green.

## Deferred items (require a human)

- **Brief Step 3 item 4** — category label visual confirmation (Social/
  Gaming/Streaming) in the UniFi web UI. Not agent-executable — requires a
  human to open the UniFi app and eyeball the category chip names against
  `internal/rules/presets.go`'s `UnifiID` values.
- **Brief Step 3 item 6** — real-device enforcement spot-check (block
  `example.com` for an actual phone, confirm it fails to load, then
  unpause/delete and confirm it loads again). Skipped per the gateway safety
  rules — this task only targets the bogus MAC `de:ad:be:ef:00:99`, never a
  real household device.
- **Brief Step 3 item 8 (janitor 5-min expiry observation)** — not run live.
  The pause-duration UI only offers 30 min / 1 hour / until morning /
  indefinite (no short "ends in 2 minutes" option), and fabricating a
  synthetic short-lived one-time rule via the API to force a ~5-minute wait
  for the next janitor tick felt like unjustified idle time for an optional,
  explicitly "don't block on it" item — especially since `CleanupOnce`'s
  expiry/reconciliation logic is already covered by
  `internal/server/janitor_test.go` in the unit suite. Flagging as available
  to run live if a maintainer wants direct observation.

## Concerns

- The chrome-devtools MCP's dialog handling (`confirm()` via
  `evaluate_script`'s `dialogAction` and the standalone `handle_dialog` tool)
  did not reliably resolve the native `confirm()` prompt from
  "Delete profile & its rules" — two attempts left zero DELETE requests in
  the network log despite "successfully accepted" responses. Worked around
  by monkey-patching `window.confirm` before the click; this is a tooling
  quirk in this environment, not a bug in Bedtime. Mentioning in case it
  recurs in future live/browser verification passes.
- Separately (and unrelated to gateway safety), the chrome-devtools MCP's
  `click` tool did not visibly change page state for this Alpine.js SPA
  (elements have UIDs but clicks didn't trigger the app's route changes);
  dispatching `.click()` via `evaluate_script` on freshly-queried, visibility
  -filtered elements worked reliably instead. No application code changes
  were needed — this is purely a note about how the walk-through had to be
  driven.
- No preset domain lists needed adjustment — the YouTube preset rule created
  during the walk-through round-tripped through the real gateway exactly as
  translated.
- Confirmed the one-time-rule start-day anchoring for overnight pauses (the
  brief's Step 3 item 5 concern) already works correctly in the current code
  — verified live rather than needing a fix.

Nothing was committed to git, per project instructions.
