# Task 11 brief — Bedtime implementation plan

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



### Task 11: Live E2E, README, cross-compile, final verification

**Files:**
- Create: `internal/e2e/e2e_test.go`, `README.md`

**Interfaces:**
- Consumes: everything. This task touches the **real gateway** — the only one allowed to (opt-in via `BEDTIME_E2E=1`). All test rules use the `[bedtime-e2e]` prefix, are created `enabled:false`, target a bogus MAC, and are deleted in a deferred cleanup even on failure. The user's own rules are never touched.

- [ ] **Step 1: Write the opt-in live E2E test**

Create `internal/e2e/e2e_test.go`:

```go
// Package e2e holds the opt-in live test against a real UniFi gateway.
// Run: BEDTIME_E2E=1 go test ./internal/e2e/ -v   (UNIFI_API_KEY required;
// BEDTIME_GATEWAY overrides the default 192.168.0.1)
package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"bedtime/internal/unifi"
)

const e2ePrefix = "[bedtime-e2e]"

func TestLiveGatewayRuleLifecycle(t *testing.T) {
	if os.Getenv("BEDTIME_E2E") != "1" {
		t.Skip("set BEDTIME_E2E=1 to run against the real gateway")
	}
	key := os.Getenv("UNIFI_API_KEY")
	if key == "" {
		t.Fatal("UNIFI_API_KEY not set (source .env)")
	}
	host := os.Getenv("BEDTIME_GATEWAY")
	if host == "" {
		host = "192.168.0.1"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fp, err := unifi.FetchCertFingerprint(ctx, host)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	c := unifi.New(host, key, fp)

	// Leave the gateway clean no matter what happens above this line.
	defer func() {
		rules, err := c.ListTrafficRules(ctx)
		if err != nil {
			t.Logf("cleanup list failed: %v", err)
			return
		}
		for _, r := range rules {
			if strings.HasPrefix(r.Description, e2ePrefix) {
				if err := c.DeleteTrafficRule(ctx, r.ID); err != nil {
					t.Errorf("cleanup delete %s failed: %v — REMOVE IT MANUALLY in the UniFi app", r.ID, err)
				}
			}
		}
	}()

	if _, err := c.Version(ctx); err != nil {
		t.Fatalf("version: %v", err)
	}

	r := unifi.NewBlockRule()
	r.Description = e2ePrefix + " lifecycle"
	r.MatchingTarget = unifi.MatchDomain
	r.Domains = []unifi.Domain{{Domain: "example.com", Ports: []int{}, PortRanges: []any{}}}
	r.TargetDevices = []unifi.TargetDevice{{ClientMAC: "de:ad:be:ef:00:99", Type: unifi.TargetTypeClient}}
	r.Schedule = unifi.Schedule{Mode: unifi.ModeEveryWeek, RepeatOnDays: []string{"mon", "tue"},
		TimeRangeStart: "20:00", TimeRangeEnd: "07:00"} // crosses midnight on purpose
	r.Enabled = false // never enforce anything from a test

	created, err := c.CreateTrafficRule(ctx, r)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("create returned no _id")
	}

	list, err := c.ListTrafficRules(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var got *unifi.TrafficRule
	for i := range list {
		if list[i].ID == created.ID {
			got = &list[i]
		}
	}
	if got == nil {
		t.Fatal("created rule not in list")
	}
	if got.Schedule.Mode != unifi.ModeEveryWeek || len(got.Schedule.RepeatOnDays) != 2 ||
		got.Schedule.TimeRangeStart != "20:00" || got.Schedule.TimeRangeEnd != "07:00" {
		t.Errorf("schedule did not round-trip: %+v", got.Schedule)
	}
	if got.Enabled {
		t.Error("rule must stay disabled")
	}

	got.Domains = append(got.Domains, unifi.Domain{Domain: "example.org", Ports: []int{}, PortRanges: []any{}})
	if err := c.UpdateTrafficRule(ctx, *got); err != nil {
		t.Fatalf("update (expects 200/201): %v", err)
	}

	if err := c.DeleteTrafficRule(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = c.ListTrafficRules(ctx)
	for _, x := range list {
		if x.ID == created.ID {
			t.Error("rule still present after delete")
		}
	}
}
```

- [ ] **Step 2: Run it against the real gateway**

```bash
cd /Users/dsandor/Projects/bedtime
set -a; source .env; set +a
BEDTIME_E2E=1 go test ./internal/e2e/ -v
```

Expected: PASS. Afterwards verify cleanliness: the trafficrules list contains only the user's own rules (e.g. "kids apps") — no `[bedtime-e2e]` leftovers.

- [ ] **Step 3: Manual verification checklist (real gateway + browser)**

Run the real binary (`go run ./cmd/bedtime --data <scratch>/bedtime-real.json --port 8080`) and walk through as a parent would. Confirm each item; fix before proceeding if any fails:

1. **Setup**: suggested gateway prefilled; "API key found on the server ✓" appears (`.env` present); Test connection succeeds; PIN set; lands on Home.
2. **Profile**: create "Test Kid" and assign one real-but-expendable device (or any device — the rules below stay short-lived).
3. **Rule**: add "No YouTube on school nights". Open the **UniFi app** → Traffic & Firewall Rules: the rule appears as `[bedtime] <id> No YouTube on school nights`, Sun–Thu 20:00–07:00, correct device. Delete it from Bedtime; it disappears from UniFi.
4. **Category labels**: create one category rule per category (Social/Gaming/Streaming) targeting Test Kid; in the UniFi UI confirm each shows the expected category name (Social Networks / Games / Streaming-media-like label). **If a label mismatches, fix the `UnifiID` in `internal/rules/presets.go`** and re-verify. Delete the rules.
5. **Pause semantics**: tap "Pause internet — until morning" in the evening; in the UniFi UI open the created rule and confirm the schedule reads as tonight→07:00 tomorrow. **If the gateway interprets the one-time `date` + overnight range as already-past/ended**, change `translateSchedule`'s one-time branch to use the *start* day's date (`s.Date = now`… requires threading `now` — take the date from `w.Until` minus the overnight day-shift) or fall back to `EVERY_DAY` + janitor deletion; re-verify. Resume; rule disappears.
6. **Enforcement spot-check** (recommended): with a rule blocking `example.com` for your own phone active now, confirm the site fails to load; then unpause/delete and confirm it loads.
7. **Cert re-trust**: Settings shows gateway host + site; "Trust the gateway's new certificate" succeeds (no-op re-pin).
8. **Janitor**: let a 30-min pause expire (or create one ending in ~2 min), wait for the next janitor tick (≤5 min) — the spent rule disappears from the UniFi UI and from Bedtime's data file.
9. Delete the Test Kid profile — its rules vanish from UniFi; the user's own "kids apps" rule is still there, untouched.

- [ ] **Step 4: Write the README**

Create `README.md`:

```markdown
# 🌙 Bedtime

Family-friendly screen-time rules for UniFi networks. One small binary with a
built-in web app: parents group devices by kid, then block apps ("YouTube",
"Roblox"), whole categories, specific websites, or *all* internet — on a
schedule ("school nights, 8pm–7am") or right now ("pause for 30 minutes").

Enforcement runs **on your UniFi gateway** as native traffic rules with
built-in schedules, so blocking keeps working even when Bedtime isn't running.

## Requirements

- A UniFi Cloud Gateway (tested on UCG Max, UniFi Network 10.4.x)
- An API key: UniFi console → **Settings → Control Plane → Integrations**
- Go 1.24+ to build

## Quick start

    go build -o bedtime ./cmd/bedtime
    ./bedtime
    # open http://localhost:8080 and follow the setup wizard

Flags: `--port` (default 8080), `--data` (default: your OS config dir +
`/bedtime/bedtime.json`). If a `.env` file with `UNIFI_API_KEY=...` sits in
the working directory, setup offers to use it automatically.

## How it works

Every Bedtime rule is a real UniFi traffic rule tagged `[bedtime] ` in its
description, with the schedule enforced by the gateway itself. Bedtime never
modifies or deletes rules it didn't create. Deleting a rule/profile in
Bedtime removes its gateway rules; deleting a Bedtime-made rule in the UniFi
app is tolerated (Bedtime forgets it on the next cleanup pass).

## Security notes

- The web UI is protected by a parent PIN (bcrypt-hashed; login backs off
  after repeated failures). Sessions last 30 days per browser.
- The gateway's self-signed TLS certificate is pinned on first use
  (SHA-256). If UniFi regenerates it (e.g. firmware update), Bedtime shows a
  banner and Settings offers one-tap re-trust.
- The API key and PIN hash live in the data file (created mode 0600). Treat
  that file like a password.
- Bedtime binds to all interfaces on your LAN. Don't port-forward it.

## Development

    go test ./...                       # unit tests (no gateway needed)
    BEDTIME_E2E=1 go test ./internal/e2e/ -v   # opt-in live gateway test

The UI is embedded via go:embed (`web/static/`) — no Node toolchain; Alpine
is vendored. `go build` is the whole pipeline.

## Troubleshooting

- **"Can't reach your UniFi gateway"** — check the gateway address in
  Settings, and that you're on the same network.
- **"The gateway rejected the API key"** — regenerate a key in the UniFi
  console and update it in Settings.
- **Certificate banner** — expected after UniFi updates; re-trust in
  Settings.
- **An app preset doesn't fully block the app** — domain lists live in
  `internal/rules/presets.go`; add the missing domains and rebuild.
```

- [ ] **Step 5: Cross-compile + format + full suite**

```bash
gofmt -l . ; test -z "$(gofmt -l .)"
GOOS=darwin GOARCH=arm64 go build -o /dev/null ./cmd/bedtime
GOOS=darwin GOARCH=amd64 go build -o /dev/null ./cmd/bedtime
GOOS=linux  GOARCH=arm64 go build -o /dev/null ./cmd/bedtime
GOOS=linux  GOARCH=amd64 go build -o /dev/null ./cmd/bedtime
go build ./... && go vet ./... && go test ./...
```

Expected: gofmt lists nothing; all four cross-compiles succeed (pure-Go deps only); full suite green.

- [ ] **Step 6: Done — report**

Summarize for the user: what was built, the manual-checklist outcomes (especially items 4 and 5 — category labels and one-time overnight semantics, the two things probing couldn't fully verify), and any preset domain lists that needed adjustment. **Do not commit anything to git.**

---

