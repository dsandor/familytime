// Package e2e holds the opt-in live test against a real UniFi gateway.
// Run: FAMILYTIME_E2E=1 go test ./internal/e2e/ -v   (UNIFI_API_KEY required;
// FAMILYTIME_GATEWAY overrides the default 192.168.0.1)
package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"familytime/internal/unifi"
)

const e2ePrefix = "[family-time-e2e]"

func TestLiveGatewayRuleLifecycle(t *testing.T) {
	if os.Getenv("FAMILYTIME_E2E") != "1" {
		t.Skip("set FAMILYTIME_E2E=1 to run against the real gateway")
	}
	key := os.Getenv("UNIFI_API_KEY")
	if key == "" {
		t.Fatal("UNIFI_API_KEY not set (source .env)")
	}
	host := os.Getenv("FAMILYTIME_GATEWAY")
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

	// Leave the gateway clean no matter what happens above this line. Uses
	// its own fresh timeout rather than the test's ctx, which may already be
	// expired (e.g. the test itself timed out) — cleanup must still run.
	defer func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		rules, err := c.ListTrafficRules(cctx)
		if err != nil {
			t.Logf("cleanup list failed: %v", err)
			return
		}
		for _, r := range rules {
			if strings.HasPrefix(r.Description, e2ePrefix) {
				if err := c.DeleteTrafficRule(cctx, r.ID); err != nil {
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
