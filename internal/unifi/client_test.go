package unifi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func loadProbeFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/trafficrules_probe.json")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return raw
}

func TestListTrafficRulesParsesProbeFixture(t *testing.T) {
	fixture := loadProbeFixture(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/network/v2/api/site/default/trafficrules" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("X-API-KEY") != "test-key" {
			t.Errorf("missing api key header")
		}
		w.Write(fixture)
	}))
	defer ts.Close()

	c := New(ts.URL, "test-key", "")
	rules, err := c.ListTrafficRules(context.Background())
	if err != nil {
		t.Fatalf("ListTrafficRules: %v", err)
	}
	if len(rules) != 4 {
		t.Fatalf("got %d rules, want 4 (probe fixture)", len(rules))
	}
	byDesc := map[string]TrafficRule{}
	for _, r := range rules {
		byDesc[r.Description] = r
	}
	weekly := byDesc["[family-time-probe] weekly"]
	if weekly.Schedule.Mode != ModeEveryWeek || len(weekly.Schedule.RepeatOnDays) != 2 {
		t.Errorf("weekly schedule parsed wrong: %+v", weekly.Schedule)
	}
	everyone := byDesc["[family-time-probe] everyone"]
	if len(everyone.TargetDevices) != 1 || everyone.TargetDevices[0].Type != TargetTypeAllClients {
		t.Errorf("ALL_CLIENTS target parsed wrong: %+v", everyone.TargetDevices)
	}
	onetime := byDesc["[family-time-probe] onetime2"]
	if onetime.Schedule.Mode != ModeOneTime || onetime.Schedule.Date == "" {
		t.Errorf("one-time schedule parsed wrong: %+v", onetime.Schedule)
	}
	if onetime.MatchingTarget != MatchInternet {
		t.Errorf("matching_target = %s, want INTERNET", onetime.MatchingTarget)
	}
}

func TestCreateTrafficRuleAccepts200And201(t *testing.T) {
	for _, code := range []int{200, 201} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var in TrafficRule
			json.NewDecoder(r.Body).Decode(&in)
			in.ID = "new-id"
			w.WriteHeader(code)
			json.NewEncoder(w).Encode(in)
		}))
		c := New(ts.URL, "k", "")
		rule := NewBlockRule()
		rule.Description = "[family-time] x test"
		out, err := c.CreateTrafficRule(context.Background(), rule)
		ts.Close()
		if err != nil {
			t.Fatalf("code %d: %v", code, err)
		}
		if out.ID != "new-id" {
			t.Errorf("code %d: ID = %q", code, out.ID)
		}
	}
}

func TestCreateSendsEmptyArraysNotNull(t *testing.T) {
	var body map[string]json.RawMessage
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.Write([]byte(`{"_id":"x"}`))
	}))
	defer ts.Close()
	c := New(ts.URL, "k", "")
	c.CreateTrafficRule(context.Background(), NewBlockRule())
	for _, field := range []string{"domains", "app_category_ids", "app_ids", "ip_addresses", "network_ids", "target_devices"} {
		if string(body[field]) == "null" {
			t.Errorf("field %s marshaled as null, want []", field)
		}
	}
}

func TestUpdateTrafficRulePutsToIDPath(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/proxy/network/v2/api/site/default/trafficrules/abc" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(201) // the documented PUT-returns-201 quirk
		w.Write([]byte(`{}`))
	}))
	defer ts.Close()
	c := New(ts.URL, "k", "")
	r := NewBlockRule()
	r.ID = "abc"
	if err := c.UpdateTrafficRule(context.Background(), r); err != nil {
		t.Fatalf("UpdateTrafficRule: %v", err)
	}
}

func TestErrorMapping(t *testing.T) {
	for code, want := range map[int]error{401: ErrUnauthorized, 403: ErrUnauthorized, 404: ErrNotFound} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))
		c := New(ts.URL, "k", "")
		_, err := c.ListTrafficRules(context.Background())
		ts.Close()
		if !errors.Is(err, want) {
			t.Errorf("code %d: err = %v, want %v", code, err, want)
		}
	}
}

func TestListClientsPaginates(t *testing.T) {
	page := func(offset, total int, items []string) string {
		type cl struct {
			Name string `json:"name"`
		}
		var data []cl
		for _, n := range items {
			data = append(data, cl{n})
		}
		raw, _ := json.Marshal(map[string]any{
			"offset": offset, "limit": 200, "count": len(items), "totalCount": total, "data": data,
		})
		return string(raw)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/network/integration/v1/sites/s1/clients" {
			t.Errorf("path = %s", r.URL.Path)
		}
		switch r.URL.Query().Get("offset") {
		case "", "0":
			fmt.Fprint(w, page(0, 3, []string{"a", "b"}))
		case "2":
			fmt.Fprint(w, page(2, 3, []string{"c"}))
		default:
			t.Errorf("unexpected offset %s", r.URL.Query().Get("offset"))
		}
	}))
	defer ts.Close()
	c := New(ts.URL, "k", "")
	clients, err := c.ListClients(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if len(clients) != 3 {
		t.Errorf("got %d clients, want 3", len(clients))
	}
}

func TestPinnedCertVerification(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()
	sum := sha256.Sum256(ts.Certificate().Raw)
	goodPin := hex.EncodeToString(sum[:])

	c := New(ts.URL, "k", goodPin)
	if _, err := c.ListTrafficRules(context.Background()); err != nil {
		t.Fatalf("correct pin should connect: %v", err)
	}

	bad := New(ts.URL, "k", strings.Repeat("0", 64))
	if _, err := bad.ListTrafficRules(context.Background()); !errors.Is(err, ErrCertChanged) {
		t.Errorf("wrong pin: err = %v, want ErrCertChanged", err)
	}

	fp, err := FetchCertFingerprint(context.Background(), ts.URL)
	if err != nil || fp != goodPin {
		t.Errorf("FetchCertFingerprint = %q, %v; want %q", fp, err, goodPin)
	}
}

func TestRenameClientSuccess(t *testing.T) {
	var putPath, putBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path != "/proxy/network/api/s/default/rest/user" {
				t.Errorf("GET path = %s", r.URL.Path)
			}
			fmt.Fprint(w, `{"meta":{"rc":"ok"},"data":[
				{"_id":"u1","mac":"00:05:cd:3a:00:e9"},
				{"_id":"u2","mac":"aa:bb:cc:dd:ee:99","name":"old name"}
			]}`)
		case http.MethodPut:
			putPath = r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			putBody = string(raw)
			fmt.Fprint(w, `{"meta":{"rc":"ok"}}`)
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	defer ts.Close()

	c := New(ts.URL, "test-key", "")
	if err := c.RenameClient(context.Background(), "AA:BB:CC:DD:EE:99", "Ava's iPhone"); err != nil {
		t.Fatalf("RenameClient: %v", err)
	}
	if putPath != "/proxy/network/api/s/default/rest/user/u2" {
		t.Errorf("PUT path = %q", putPath)
	}
	if putBody != `{"name":"Ava's iPhone"}` {
		t.Errorf("PUT body = %q", putBody)
	}
}

func TestRenameClientMACNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"meta":{"rc":"ok"},"data":[{"_id":"u1","mac":"00:05:cd:3a:00:e9"}]}`)
	}))
	defer ts.Close()

	c := New(ts.URL, "test-key", "")
	err := c.RenameClient(context.Background(), "aa:bb:cc:dd:ee:99", "Ava's iPhone")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRenameClientRCNotOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			fmt.Fprint(w, `{"meta":{"rc":"ok"},"data":[{"_id":"u2","mac":"aa:bb:cc:dd:ee:99"}]}`)
		case http.MethodPut:
			fmt.Fprint(w, `{"meta":{"rc":"error","msg":"api.err.NoPermission"}}`)
		}
	}))
	defer ts.Close()

	c := New(ts.URL, "test-key", "")
	err := c.RenameClient(context.Background(), "aa:bb:cc:dd:ee:99", "Ava's iPhone")
	if err == nil || !strings.Contains(err.Error(), "api.err.NoPermission") {
		t.Errorf("err = %v, want it to mention the meta.msg", err)
	}
}

func TestFirstSite(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"offset":0,"limit":25,"count":1,"totalCount":1,"data":[{"id":"s1","internalReference":"default","name":"Default"}]}`)
	}))
	defer ts.Close()
	c := New(ts.URL, "k", "")
	site, err := c.FirstSite(context.Background())
	if err != nil {
		t.Fatalf("FirstSite: %v", err)
	}
	if site.ID != "s1" || site.InternalReference != "default" {
		t.Errorf("site = %+v", site)
	}
}
