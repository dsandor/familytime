package server

import (
	"context"
	"testing"
	"time"

	"familytime/internal/store"
	"familytime/internal/unifi"
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
	fake.rules = []unifi.TrafficRule{{ID: "u1", Description: "[family-time] fr1 Internet pause"}}
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
		What:         store.What{Type: store.WhatEverything},
		When:         store.When{Kind: store.WhenAlways},
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

func TestCleanupDeletesOrphanedFamilyTimeRulesAfterTwoPasses(t *testing.T) {
	ts, fake, _, srv := newTestServer(t)
	doSetup(t, ts)
	fixedNow(t, srv, "2026-07-03 12:00")
	fake.rules = []unifi.TrafficRule{{ID: "u9", Description: "[family-time] frX leftover"}}
	srv.CleanupOnce(context.Background())
	if len(fake.rules) != 1 {
		t.Fatal("first sighting must not delete")
	}
	srv.CleanupOnce(context.Background())
	if len(fake.rules) != 0 {
		t.Error("orphaned [family-time] gateway rule should be deleted on second pass")
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
		t.Fatal("rules without the [family-time] prefix must NEVER be touched")
	}
}
