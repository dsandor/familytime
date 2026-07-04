# Task 1 brief — Bedtime implementation plan

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



### Task 1: Project scaffold + store package

**Files:**
- Create: `go.mod`, `internal/store/store.go`
- Test: `internal/store/store_test.go`

**Interfaces:**
- Consumes: nothing (first task)
- Produces (later tasks import `bedtime/internal/store`):
  - Types `Data`, `Gateway`, `Auth`, `Profile`, `Device`, `FamilyRule`, `What`, `When`
  - Constants `EveryoneProfileID = "everyone"`; `WhatPreset/WhatCategory/WhatDomains/WhatEverything = "preset"/"category"/"domains"/"everything"`; `WhenAlways/WhenRecurring/WhenOneTime = "always"/"recurring"/"onetime"`
  - `Load(path string) (*Store, error)`, `(*Store).Snapshot() Data`, `(*Store).Update(fn func(*Data) error) error`, `(*Store).IsConfigured() bool`, `(*Store).Path() string`, `NewID() string`

- [ ] **Step 1: Initialize the module**

```bash
cd /Users/dsandor/Projects/bedtime
go mod init bedtime
go get golang.org/x/crypto@latest
```

Expected: `go.mod` created with `module bedtime`, `go 1.24`.

- [ ] **Step 2: Write the failing store test**

Create `internal/store/store_test.go`:

```go
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCreatesEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "bedtime.json")
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	d := s.Snapshot()
	if d.Version != 1 {
		t.Errorf("Version = %d, want 1", d.Version)
	}
	if s.IsConfigured() {
		t.Error("empty store should not be configured")
	}
}

func TestUpdatePersistsAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bedtime.json")
	s, _ := Load(path)
	err := s.Update(func(d *Data) error {
		d.Gateway = Gateway{Host: "192.168.0.1", APIKey: "k", SiteID: "site1", SiteName: "default"}
		d.Auth = Auth{PINHash: "hash", SessionSecret: "secret"}
		d.Profiles = append(d.Profiles, Profile{
			ID: "p1", Name: "Emma", Emoji: "🦄", Color: "#b57edc",
			Devices: []Device{{MAC: "aa:bb:cc:dd:ee:ff", Name: "Emma's iPad"}},
		})
		return nil
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !s.IsConfigured() {
		t.Error("store should be configured after setup fields set")
	}

	// Reload from disk and verify round-trip.
	s2, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	d := s2.Snapshot()
	if len(d.Profiles) != 1 || d.Profiles[0].Name != "Emma" || d.Profiles[0].Devices[0].Name != "Emma's iPad" {
		t.Errorf("round-trip mismatch: %+v", d.Profiles)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode = %o, want 600", perm)
	}
}

func TestSnapshotIsDeepCopy(t *testing.T) {
	s, _ := Load(filepath.Join(t.TempDir(), "b.json"))
	s.Update(func(d *Data) error {
		d.Profiles = []Profile{{ID: "p1", Devices: []Device{{MAC: "m1"}}}}
		return nil
	})
	snap := s.Snapshot()
	snap.Profiles[0].Devices[0].MAC = "mutated"
	if s.Snapshot().Profiles[0].Devices[0].MAC != "m1" {
		t.Error("Snapshot leaked internal state")
	}
}

func TestLoadRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(path, []byte("{not json"), 0o600)
	if _, err := Load(path); err == nil {
		t.Fatal("Load should fail on corrupt JSON, not reinitialize")
	}
}

func TestUpdateRollsBackOnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.json")
	s, _ := Load(path)
	s.Update(func(d *Data) error { d.Profiles = []Profile{{ID: "keep"}}; return nil })
	errBoom := json.Unmarshal([]byte("x"), &struct{}{}) // any non-nil error
	err := s.Update(func(d *Data) error {
		d.Profiles = nil
		return errBoom
	})
	if err == nil {
		t.Fatal("Update should propagate fn error")
	}
	if got := s.Snapshot().Profiles; len(got) != 1 || got[0].ID != "keep" {
		t.Errorf("failed Update must not mutate state, got %+v", got)
	}
}

func TestNewIDUnique(t *testing.T) {
	if NewID() == NewID() {
		t.Error("NewID should produce unique ids")
	}
	if len(NewID()) != 16 {
		t.Errorf("NewID length = %d, want 16", len(NewID()))
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/store/ -v`
Expected: FAIL — compile errors (`Load`, `Data`, etc. undefined).

- [ ] **Step 4: Implement the store**

Create `internal/store/store.go`:

```go
// Package store persists Bedtime's app-side state — everything UniFi can't
// hold for us: profiles, auth, and the mapping from family rules to gateway
// rule ids. One JSON file, atomic writes, safe for concurrent use.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const EveryoneProfileID = "everyone"

// What.Type values.
const (
	WhatPreset     = "preset"
	WhatCategory   = "category"
	WhatDomains    = "domains"
	WhatEverything = "everything"
)

// When.Kind values.
const (
	WhenAlways    = "always"
	WhenRecurring = "recurring"
	WhenOneTime   = "onetime"
)

type Data struct {
	Version  int          `json:"version"`
	Gateway  Gateway      `json:"gateway"`
	Auth     Auth         `json:"auth"`
	Profiles []Profile    `json:"profiles"`
	Rules    []FamilyRule `json:"rules"`
}

type Gateway struct {
	Host            string `json:"host"`
	APIKey          string `json:"apiKey"`
	SiteID          string `json:"siteId"`
	SiteName        string `json:"siteName"`
	CertFingerprint string `json:"certFingerprint"` // SHA-256 hex of gateway leaf cert, pinned at setup
}

type Auth struct {
	PINHash       string `json:"pinHash"`
	SessionSecret string `json:"sessionSecret"`
}

type Device struct {
	MAC  string `json:"mac"`
	Name string `json:"name"`
}

type Profile struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Emoji   string   `json:"emoji"`
	Color   string   `json:"color"`
	Devices []Device `json:"devices"`
}

type What struct {
	Type       string   `json:"type"`
	PresetID   string   `json:"presetId,omitempty"`
	CategoryID string   `json:"categoryId,omitempty"`
	Domains    []string `json:"domains,omitempty"`
}

type When struct {
	Kind  string   `json:"kind"`
	Days  []string `json:"days,omitempty"`  // "sun".."sat"
	Start string   `json:"start,omitempty"` // "20:00"
	End   string   `json:"end,omitempty"`   // "07:00" (may cross midnight)
	Until string   `json:"until,omitempty"` // RFC3339, one-time rules only
}

type FamilyRule struct {
	ID           string   `json:"id"`
	ProfileID    string   `json:"profileId"`
	Name         string   `json:"name"`
	What         What     `json:"what"`
	When         When     `json:"when"`
	Enabled      bool     `json:"enabled"`
	Pause        bool     `json:"pause,omitempty"` // created by the Pause button
	UnifiRuleIDs []string `json:"unifiRuleIds"`
}

type Store struct {
	mu   sync.Mutex
	path string
	data Data
}

// Load reads the store at path, creating an empty one (and parent dirs) if
// the file does not exist. Corrupt JSON is an error — never silently reset.
func Load(path string) (*Store, error) {
	s := &Store{path: path, data: Data{Version: 1}}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("store: create dir: %w", err)
		}
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("store: %s is not valid JSON (refusing to overwrite): %w", path, err)
	}
	return s, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) IsConfigured() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Gateway.Host != "" && s.data.Auth.PINHash != ""
}

// Snapshot returns a deep copy of the current data.
func (s *Store) Snapshot() Data {
	s.mu.Lock()
	defer s.mu.Unlock()
	return deepCopy(s.data)
}

// Update mutates the data under lock and persists atomically. If fn returns
// an error, no change is kept and nothing is written.
func (s *Store) Update(fn func(*Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	work := deepCopy(s.data)
	if err := fn(&work); err != nil {
		return err
	}
	if err := s.save(work); err != nil {
		return err
	}
	s.data = work
	return nil
}

func (s *Store) save(d Data) error {
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("store: marshal: %w", err)
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".bedtime-*")
	if err != nil {
		return fmt.Errorf("store: temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

func deepCopy(d Data) Data {
	raw, err := json.Marshal(d)
	if err != nil {
		panic(fmt.Sprintf("store: deep copy marshal: %v", err))
	}
	var out Data
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(fmt.Sprintf("store: deep copy unmarshal: %v", err))
	}
	return out
}

// NewID returns a 16-hex-char random id for profiles and rules.
func NewID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("store: rand: %v", err))
	}
	return hex.EncodeToString(b)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/store/ -v`
Expected: PASS (6 tests).

- [ ] **Step 6: Full verification**

Run: `go build ./... && go vet ./...`
Expected: no output, exit 0. (Note: `go.sum` may not exist until a package actually imports x/crypto in Task 4 — if `go build` complains about an unused dependency, run `go mod tidy`.)

---

