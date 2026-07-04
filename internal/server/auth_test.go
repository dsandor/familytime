package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func postJSON(t *testing.T, c *http.Client, url, body string) *http.Response {
	t.Helper()
	resp, err := c.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func getState(t *testing.T, c *http.Client, base string) (configured, authed bool) {
	t.Helper()
	resp, err := c.Get(base + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct{ Configured, Authed bool }
	json.NewDecoder(resp.Body).Decode(&out)
	return out.Configured, out.Authed
}

func TestSetupFlowConfiguresAndAuthenticates(t *testing.T) {
	ts, _, st, _ := newTestServer(t)
	c := client(t)

	if conf, _ := getState(t, c, ts.URL); conf {
		t.Fatal("fresh store must not be configured")
	}
	resp := postJSON(t, c, ts.URL+"/api/setup", `{"host":"http://fake","apiKey":"k","pin":"1234"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("setup = %d", resp.StatusCode)
	}
	conf, authed := getState(t, c, ts.URL)
	if !conf || !authed {
		t.Errorf("after setup: configured=%v authed=%v, want true/true", conf, authed)
	}
	d := st.Snapshot()
	if d.Gateway.SiteID != "site-1" || d.Auth.PINHash == "" || d.Auth.PINHash == "1234" || d.Auth.SessionSecret == "" {
		t.Errorf("stored gateway/auth wrong: %+v", d)
	}
}

func TestSetupRejectedWhenAlreadyConfigured(t *testing.T) {
	ts, _, _, _ := newTestServer(t)
	doSetup(t, ts)
	resp := postJSON(t, client(t), ts.URL+"/api/setup", `{"host":"http://fake","apiKey":"k","pin":"9999"}`)
	if resp.StatusCode != 409 {
		t.Errorf("second setup = %d, want 409", resp.StatusCode)
	}
}

func TestSetupValidatesGatewayBeforeSaving(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	fake.failAll = http.ErrHandlerTimeout // any error
	resp := postJSON(t, client(t), ts.URL+"/api/setup", `{"host":"http://fake","apiKey":"bad","pin":"1234"}`)
	if resp.StatusCode == 200 {
		t.Error("setup must fail when the gateway check fails")
	}
	if st.IsConfigured() {
		t.Error("failed setup must not persist configuration")
	}
}

func TestSetupRejectsBadPIN(t *testing.T) {
	ts, _, _, _ := newTestServer(t)
	for _, pin := range []string{"12", "1234567", "abcd", ""} {
		resp := postJSON(t, client(t), ts.URL+"/api/setup", `{"host":"http://fake","apiKey":"k","pin":"`+pin+`"}`)
		if resp.StatusCode != 400 {
			t.Errorf("pin %q: setup = %d, want 400", pin, resp.StatusCode)
		}
	}
}

func TestLoginLogoutAndWrongPIN(t *testing.T) {
	ts, _, _, _ := newTestServer(t)
	doSetup(t, ts)

	c := client(t) // fresh browser
	if _, authed := getState(t, c, ts.URL); authed {
		t.Fatal("fresh client must not be authed")
	}
	if resp := postJSON(t, c, ts.URL+"/api/login", `{"pin":"9999"}`); resp.StatusCode != 401 {
		t.Errorf("wrong pin = %d, want 401", resp.StatusCode)
	}
	if resp := postJSON(t, c, ts.URL+"/api/login", `{"pin":"1234"}`); resp.StatusCode != 200 {
		t.Errorf("right pin = %d, want 200", resp.StatusCode)
	}
	if _, authed := getState(t, c, ts.URL); !authed {
		t.Error("must be authed after login")
	}
	postJSON(t, c, ts.URL+"/api/logout", `{}`)
	if _, authed := getState(t, c, ts.URL); authed {
		t.Error("must not be authed after logout")
	}
}

func TestLoginBackoffAfterFiveFailures(t *testing.T) {
	ts, _, _, _ := newTestServer(t)
	doSetup(t, ts)
	c := client(t)
	for i := 0; i < 5; i++ {
		postJSON(t, c, ts.URL+"/api/login", `{"pin":"0000"}`)
	}
	resp := postJSON(t, c, ts.URL+"/api/login", `{"pin":"1234"}`) // even the right pin
	if resp.StatusCode != 429 {
		t.Errorf("6th attempt = %d, want 429 (locked)", resp.StatusCode)
	}
}

func TestProtectedEndpointRequiresSession(t *testing.T) {
	ts, _, _, _ := newTestServer(t)
	doSetup(t, ts)
	resp, err := client(t).Get(ts.URL + "/api/status") // no cookie
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("status without session = %d, want 401", resp.StatusCode)
	}
}

func TestStateSetupHintsAndEnvKeyFallback(t *testing.T) {
	t.Setenv("UNIFI_API_KEY", "env-key")
	ts, _, st, srv := newTestServer(t)
	srv.detectGateway = func() string { return "http://fake" }
	c := client(t)
	var out struct {
		Configured bool `json:"configured"`
		EnvAPIKey  bool `json:"envApiKey"`
	}
	resp, _ := c.Get(ts.URL + "/api/state")
	json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if !out.EnvAPIKey {
		t.Error("state should advertise that the server holds an API key")
	}
	// Empty apiKey in setup, host matching the detected gateway → server uses the env key.
	r2 := postJSON(t, c, ts.URL+"/api/setup", `{"host":"http://fake","apiKey":"","pin":"1234"}`)
	if r2.StatusCode != 200 {
		t.Fatalf("setup with env key = %d", r2.StatusCode)
	}
	if got := st.Snapshot().Gateway.APIKey; got != "env-key" {
		t.Errorf("stored key = %q, want env-key", got)
	}
}

func TestEnvKeyFallbackOnlyForDetectedGateway(t *testing.T) {
	t.Setenv("UNIFI_API_KEY", "env-key")
	ts, _, st, srv := newTestServer(t)
	srv.detectGateway = func() string { return "http://fake-gateway" }
	c := client(t)

	// A host that doesn't match the detected gateway must not get the env key.
	resp := postJSON(t, c, ts.URL+"/api/setup", `{"host":"http://someone-elses-router","apiKey":"","pin":"1234"}`)
	if resp.StatusCode != 400 {
		t.Errorf("mismatched host + empty key = %d, want 400", resp.StatusCode)
	}
	if st.IsConfigured() {
		t.Error("must not configure without a real key")
	}

	// No detected gateway at all → the state hint must not advertise the env key either.
	ts2, _, _, srv2 := newTestServer(t)
	srv2.detectGateway = func() string { return "" }
	c2 := client(t)
	var out struct {
		EnvAPIKey bool `json:"envApiKey"`
	}
	resp2, _ := c2.Get(ts2.URL + "/api/state")
	json.NewDecoder(resp2.Body).Decode(&out)
	resp2.Body.Close()
	if out.EnvAPIKey {
		t.Error("state must not advertise an env key when no gateway was detected")
	}
}

func TestTestConnectionOnlyBeforeSetup(t *testing.T) {
	ts, _, _, _ := newTestServer(t)
	c := client(t)
	var out struct {
		Version string `json:"version"`
	}
	if code := doJSON(t, c, "POST", ts.URL+"/api/test-connection", `{"host":"http://fake","apiKey":"k"}`, &out); code != 200 || out.Version == "" {
		t.Errorf("test-connection = %d, version = %q", code, out.Version)
	}
	doSetup(t, ts)
	if code := doJSON(t, c, "POST", ts.URL+"/api/test-connection", `{"host":"http://fake","apiKey":"k"}`, nil); code != 409 {
		t.Errorf("test-connection after setup = %d, want 409", code)
	}
}

func TestForgedSessionRejected(t *testing.T) {
	ts, _, _, _ := newTestServer(t)
	doSetup(t, ts)
	req, _ := http.NewRequest("GET", ts.URL+"/api/status", nil)
	req.AddCookie(&http.Cookie{Name: "familytime_session", Value: "9999999999.deadbeef"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("forged cookie = %d, want 401", resp.StatusCode)
	}
}
