package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"familytime/internal/store"
	"familytime/internal/unifi"
)

// fakeAPI is an in-memory UnifiAPI. Set failAll to make every call error, or
// failUpdateID to make UpdateTrafficRule fail only for that one rule id.
type fakeAPI struct {
	mu           sync.Mutex
	rules        []unifi.TrafficRule
	clients      []unifi.NetClient
	nextID       int
	failAll      error
	failUpdateID string
	renames      []struct{ MAC, Name string }
	failRename   error
}

func (f *fakeAPI) err() error { f.mu.Lock(); defer f.mu.Unlock(); return f.failAll }

func (f *fakeAPI) Version(ctx context.Context) (string, error) {
	if e := f.err(); e != nil {
		return "", e
	}
	return "10.4.57", nil
}

func (f *fakeAPI) FirstSite(ctx context.Context) (unifi.Site, error) {
	if e := f.err(); e != nil {
		return unifi.Site{}, e
	}
	return unifi.Site{ID: "site-1", InternalReference: "default", Name: "Default"}, nil
}

func (f *fakeAPI) ListClients(ctx context.Context, siteID string) ([]unifi.NetClient, error) {
	if e := f.err(); e != nil {
		return nil, e
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]unifi.NetClient{}, f.clients...), nil
}

func (f *fakeAPI) ListTrafficRules(ctx context.Context) ([]unifi.TrafficRule, error) {
	if e := f.err(); e != nil {
		return nil, e
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]unifi.TrafficRule{}, f.rules...), nil
}

func (f *fakeAPI) CreateTrafficRule(ctx context.Context, r unifi.TrafficRule) (unifi.TrafficRule, error) {
	if e := f.err(); e != nil {
		return unifi.TrafficRule{}, e
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	r.ID = fmt.Sprintf("u%d", f.nextID)
	f.rules = append(f.rules, r)
	return r, nil
}

func (f *fakeAPI) UpdateTrafficRule(ctx context.Context, r unifi.TrafficRule) error {
	if e := f.err(); e != nil {
		return e
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failUpdateID != "" && r.ID == f.failUpdateID {
		return fmt.Errorf("simulated gateway failure updating rule %s", r.ID)
	}
	for i := range f.rules {
		if f.rules[i].ID == r.ID {
			f.rules[i] = r
			return nil
		}
	}
	return unifi.ErrNotFound
}

func (f *fakeAPI) DeleteTrafficRule(ctx context.Context, id string) error {
	if e := f.err(); e != nil {
		return e
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.rules {
		if f.rules[i].ID == id {
			f.rules = append(f.rules[:i], f.rules[i+1:]...)
			return nil
		}
	}
	return unifi.ErrNotFound
}

func (f *fakeAPI) RenameClient(ctx context.Context, mac, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failRename != nil {
		return f.failRename
	}
	f.renames = append(f.renames, struct{ MAC, Name string }{mac, name})
	return nil
}

func (f *fakeAPI) ruleByDesc(substr string) (unifi.TrafficRule, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rules {
		if strings.Contains(r.Description, substr) {
			return r, true
		}
	}
	return unifi.TrafficRule{}, false
}

var testStatic = fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>familytime</html>")}}

// newTestServer returns an httptest server wrapping a fresh Server, the
// shared fake gateway, the backing store, and the Server itself (so tests
// can override srv.now for schedule-dependent assertions).
func newTestServer(t *testing.T) (*httptest.Server, *fakeAPI, *store.Store, *Server) {
	t.Helper()
	st, err := store.Load(t.TempDir() + "/familytime.json")
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeAPI{}
	srv := New(st, func(host, apiKey, fp string) UnifiAPI { return fake }, testStatic)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, fake, st, srv
}

// client returns an http.Client with a cookie jar so sessions persist.
func client(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

// doSetup runs the first-run setup and returns an authed client.
func doSetup(t *testing.T, ts *httptest.Server) *http.Client {
	t.Helper()
	c := client(t)
	resp, err := c.Post(ts.URL+"/api/setup", "application/json",
		strings.NewReader(`{"host":"http://fake-gateway","apiKey":"k","pin":"1234"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("setup returned %d", resp.StatusCode)
	}
	return c
}
