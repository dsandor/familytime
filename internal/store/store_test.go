package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCreatesEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "familytime.json")
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
	path := filepath.Join(t.TempDir(), "familytime.json")
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

func TestLoadRejectsNewerVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.json")
	os.WriteFile(path, []byte(`{"version":2}`), 0o600)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load should reject a file from a newer Family Time version")
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "newer Family Time") {
		t.Errorf("error = %q, want it to name the file and mention a newer Family Time", err.Error())
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
