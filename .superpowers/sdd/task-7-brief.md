# Task 7 brief — Bedtime implementation plan

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



### Task 7: Janitor + embedded UI wiring + main.go (runnable binary)

**Files:**
- Create: `internal/server/janitor.go`, `web/embed.go`, `web/static/index.html` (placeholder, replaced in Task 8), `cmd/bedtime/main.go`
- Test: `internal/server/janitor_test.go`

**Interfaces:**
- Consumes: everything so far
- Produces: `(*Server).CleanupOnce(ctx) error`, `(*Server).RunJanitor(ctx, interval)`; `web.Static() fs.FS`; the `bedtime` binary (`--port`, `--data` flags)

- [ ] **Step 1: Write the failing janitor tests**

Create `internal/server/janitor_test.go`:

```go
package server

import (
	"context"
	"testing"
	"time"

	"bedtime/internal/store"
	"bedtime/internal/unifi"
)

func seedRule(t *testing.T, st *store.Store, fr store.FamilyRule) {
	t.Helper()
	if err := st.Update(func(d *store.Data) error { d.Rules = append(d.Rules, fr); return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupRemovesExpiredOneTimeRules(t *testing.T) {
	ts, fake, st, srv := newTestServer(t)
	doSetup(t, ts)
	now := fixedNow(t, srv, "2026-07-03 23:00")
	fake.rules = []unifi.TrafficRule{{ID: "u1", Description: "[bedtime] fr1 Internet pause"}}
	seedRule(t, st, store.FamilyRule{
		ID: "fr1", ProfileID: "everyone", Name: "Internet pause", Pause: true, Enabled: true,
		What: store.What{Type: store.WhatEverything},
		When: store.When{Kind: store.WhenOneTime, Start: "21:00",
			Until: now.Add(-time.Hour).Format(time.RFC3339)}, // window ended an hour ago
		UnifiRuleIDs: []string{"u1"},
	})
	if err := srv.CleanupOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(st.Snapshot().Rules) != 0 {
		t.Error("expired rule should be dropped from store")
	}
	if len(fake.rules) != 0 {
		t.Error("expired rule should be deleted from gateway")
	}
}

func TestCleanupForgetsVanishedRulesAfterTwoPasses(t *testing.T) {
	ts, _, st, srv := newTestServer(t)
	doSetup(t, ts)
	fixedNow(t, srv, "2026-07-03 12:00")
	seedRule(t, st, store.FamilyRule{
		ID: "fr1", ProfileID: "everyone", Name: "x", Enabled: true,
		What: store.What{Type: store.WhatEverything},
		When: store.When{Kind: store.WhenAlways},
		UnifiRuleIDs: []string{"gone-id"}, // not on the gateway
	})
	srv.CleanupOnce(context.Background())
	if len(st.Snapshot().Rules) != 1 {
		t.Fatal("first sighting must not drop the rule (write may be in flight)")
	}
	srv.CleanupOnce(context.Background())
	if len(st.Snapshot().Rules) != 0 {
		t.Error("second consecutive sighting should drop the metadata")
	}
}

func TestCleanupDeletesOrphanedBedtimeRulesAfterTwoPasses(t *testing.T) {
	ts, fake, _, srv := newTestServer(t)
	doSetup(t, ts)
	fixedNow(t, srv, "2026-07-03 12:00")
	fake.rules = []unifi.TrafficRule{{ID: "u9", Description: "[bedtime] frX leftover"}}
	srv.CleanupOnce(context.Background())
	if len(fake.rules) != 1 {
		t.Fatal("first sighting must not delete")
	}
	srv.CleanupOnce(context.Background())
	if len(fake.rules) != 0 {
		t.Error("orphaned [bedtime] gateway rule should be deleted on second pass")
	}
}

func TestCleanupNeverTouchesForeignRules(t *testing.T) {
	ts, fake, _, srv := newTestServer(t)
	doSetup(t, ts)
	fixedNow(t, srv, "2026-07-03 12:00")
	fake.rules = []unifi.TrafficRule{{ID: "u5", Description: "kids apps"}} // the user's own rule
	srv.CleanupOnce(context.Background())
	srv.CleanupOnce(context.Background())
	srv.CleanupOnce(context.Background())
	if len(fake.rules) != 1 {
		t.Fatal("rules without the [bedtime] prefix must NEVER be touched")
	}
}
```

Use the `mustParse`-style helper form for the `Until` field (shown in the comment) so the test is timezone-proof — copy `fixedNow`'s parse pattern.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -v -run TestCleanup`
Expected: FAIL — `CleanupOnce` undefined.

- [ ] **Step 3: Implement the janitor**

Create `internal/server/janitor.go`:

```go
package server

import (
	"context"
	"log"
	"time"

	"bedtime/internal/rules"
	"bedtime/internal/store"
)

// CleanupOnce reconciles app state with the gateway:
//
//  1. expired one-time rules (spent pauses) are deleted from the gateway
//     and the store — enforcement already ended, this is cosmetic tidying;
//  2. store metadata whose gateway rule vanished (deleted in the UniFi app)
//     is forgotten;
//  3. [bedtime]-tagged gateway rules the store doesn't know (e.g. left by a
//     crashed compensation) are deleted.
//
// Cases 2 and 3 act only on the second consecutive sighting (s.suspects) so
// an in-flight write can't be mistaken for drift. Foreign rules — anything
// without the [bedtime] prefix — are never touched.
func (s *Server) CleanupOnce(ctx context.Context) error {
	if !s.store.IsConfigured() {
		return nil
	}
	gw, err := s.api().ListTrafficRules(ctx)
	if err != nil {
		return err
	}
	onGateway := map[string]bool{}
	bedtimeGw := map[string]bool{}
	for _, r := range gw {
		onGateway[r.ID] = true
		if rules.IsBedtime(r.Description) {
			bedtimeGw[r.ID] = true
		}
	}
	now := s.now()
	d := s.store.Snapshot()

	suspectedNow := map[string]bool{}
	var deleteIDs []string
	drop := map[string]bool{}
	tracked := map[string]bool{}

	for _, fr := range d.Rules {
		for _, id := range fr.UnifiRuleIDs {
			tracked[id] = true
		}
		if rules.Expired(fr.When, now) {
			drop[fr.ID] = true
			deleteIDs = append(deleteIDs, fr.UnifiRuleIDs...)
			continue
		}
		alive := false
		for _, id := range fr.UnifiRuleIDs {
			if onGateway[id] {
				alive = true
			}
		}
		if !alive {
			suspectedNow["meta:"+fr.ID] = true
			if s.suspects["meta:"+fr.ID] {
				drop[fr.ID] = true
			}
		}
	}
	for id := range bedtimeGw {
		if !tracked[id] {
			suspectedNow["gw:"+id] = true
			if s.suspects["gw:"+id] {
				deleteIDs = append(deleteIDs, id)
			}
		}
	}
	s.suspects = suspectedNow

	if err := s.deleteGatewayRules(ctx, deleteIDs); err != nil {
		return err
	}
	if len(drop) == 0 {
		return nil
	}
	return s.store.Update(func(d *store.Data) error {
		kept := d.Rules[:0]
		for _, fr := range d.Rules {
			if !drop[fr.ID] {
				kept = append(kept, fr)
			}
		}
		d.Rules = kept
		return nil
	})
}

// RunJanitor cleans immediately, then on every tick until ctx ends. Errors
// are logged and retried next tick — the gateway may be briefly offline.
func (s *Server) RunJanitor(ctx context.Context, interval time.Duration) {
	if err := s.CleanupOnce(ctx); err != nil {
		log.Printf("janitor: %v", err)
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.CleanupOnce(ctx); err != nil {
				log.Printf("janitor: %v", err)
			}
		}
	}
}
```

- [ ] **Step 4: Run janitor tests**

Run: `go test ./internal/server/ -v -run TestCleanup`
Expected: PASS (4 tests).

- [ ] **Step 5: Create the embed package and placeholder UI**

Create `web/embed.go`:

```go
// Package web embeds Bedtime's built-in browser UI into the binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:static
var static embed.FS

// Static returns the UI file tree with index.html at its root.
func Static() fs.FS {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
```

Create `web/static/index.html` (placeholder until Task 8):

```html
<!doctype html>
<meta charset="utf-8">
<title>Bedtime</title>
<p>Bedtime UI coming in Task 8.</p>
```

- [ ] **Step 6: Create main.go**

Create `cmd/bedtime/main.go`:

```go
// Bedtime — family-friendly screen-time rules for UniFi gateways.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"bedtime/internal/server"
	"bedtime/internal/store"
	"bedtime/internal/unifi"
	"bedtime/web"
)

func main() {
	defaultData := ""
	if dir, err := os.UserConfigDir(); err == nil {
		defaultData = filepath.Join(dir, "bedtime", "bedtime.json")
	}
	port := flag.Int("port", 8080, "port for the web UI")
	data := flag.String("data", defaultData, "path to the bedtime data file")
	flag.Parse()
	if *data == "" {
		log.Fatal("bedtime: could not determine a config directory; pass --data")
	}

	st, err := store.Load(*data)
	if err != nil {
		log.Fatal(err)
	}
	srv := server.New(st, func(host, apiKey, fp string) server.UnifiAPI {
		return unifi.New(host, apiKey, fp)
	}, web.Static())

	go srv.RunJanitor(context.Background(), 5*time.Minute)

	fmt.Printf("🌙 Bedtime is running — open http://localhost:%d\n", *port)
	fmt.Printf("   data file: %s\n", *data)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), srv.Handler()))
}
```

- [ ] **Step 7: Build and smoke-test the binary**

```bash
go build ./... && go vet ./... && go test ./...
go build -o /tmp/bedtime-smoke ./cmd/bedtime
/tmp/bedtime-smoke --port 8899 --data /tmp/bedtime-smoke.json &
sleep 1
curl -s http://localhost:8899/api/state
curl -s http://localhost:8899/ | head -c 200
kill %1
rm -f /tmp/bedtime-smoke /tmp/bedtime-smoke.json
```

Expected: `/api/state` returns `{"authed":false,"configured":false}`; `/` returns the placeholder HTML. (If your session provides a scratchpad directory, use it instead of `/tmp` for the smoke binary and data file.)

---

