package server

import (
	"bytes"
	"errors"
	"log"
	"os"
	"strings"
	"testing"
)

// TestFailErrLogsUnknownGatewayErrors verifies that failErr's default
// branch (anything that isn't one of the recognized sentinel errors) logs
// the underlying error server-side, even though the HTTP response only
// carries a friendly generic message.
func TestFailErrLogsUnknownGatewayErrors(t *testing.T) {
	ts, fake, st, _ := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	fake.failAll = errors.New("boom: connection reset by peer")

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	code := doJSON(t, c, "POST", ts.URL+"/api/rules",
		`{"profileId":"p1","name":"x","what":{"type":"everything"},"when":{"kind":"always"}}`, nil)
	if code != 502 {
		t.Fatalf("code = %d, want 502", code)
	}
	if !strings.Contains(buf.String(), "boom: connection reset by peer") {
		t.Errorf("expected the underlying gateway error to be logged, got %q", buf.String())
	}
}
