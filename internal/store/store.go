// Package store persists Family Time's app-side state — everything UniFi can't
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
	if s.data.Version > 1 {
		return nil, fmt.Errorf("store: %s was created by a newer Family Time (version %d)", path, s.data.Version)
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
	tmp, err := os.CreateTemp(dir, ".familytime-*")
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
