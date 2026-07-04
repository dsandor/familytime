package rules

import (
	"strings"
	"testing"
	"time"

	"familytime/internal/store"
	"familytime/internal/unifi"
)

var emma = store.Profile{
	ID: "p1", Name: "Emma",
	Devices: []store.Device{{MAC: "aa:aa:aa:aa:aa:01", Name: "iPad"}, {MAC: "aa:aa:aa:aa:aa:02", Name: "Switch"}},
}

func baseRule() store.FamilyRule {
	return store.FamilyRule{
		ID: "fr1", ProfileID: "p1", Name: "No YouTube",
		What:    store.What{Type: store.WhatPreset, PresetID: "youtube"},
		When:    store.When{Kind: store.WhenAlways},
		Enabled: true,
	}
}

func TestTranslatePresetForProfile(t *testing.T) {
	tr, err := Translate(baseRule(), emma)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if tr.Action != unifi.ActionBlock || tr.MatchingTarget != unifi.MatchDomain {
		t.Errorf("action/target = %s/%s", tr.Action, tr.MatchingTarget)
	}
	if len(tr.Domains) != 6 || tr.Domains[0].Domain != "youtube.com" {
		t.Errorf("domains = %+v", tr.Domains)
	}
	if len(tr.TargetDevices) != 2 || tr.TargetDevices[0].ClientMAC != "aa:aa:aa:aa:aa:01" || tr.TargetDevices[0].Type != unifi.TargetTypeClient {
		t.Errorf("targets = %+v", tr.TargetDevices)
	}
	if !strings.HasPrefix(tr.Description, DescriptionPrefix) || !strings.Contains(tr.Description, "fr1") {
		t.Errorf("description = %q", tr.Description)
	}
	if tr.Schedule.Mode != unifi.ModeAlways {
		t.Errorf("schedule = %+v", tr.Schedule)
	}
	if !tr.Enabled {
		t.Error("enabled should mirror family rule")
	}
	if tr.Domains[0].Ports == nil || tr.Domains[0].PortRanges == nil {
		t.Error("domain ports/port_ranges must be [] not null")
	}
}

func TestTranslateEveryoneUsesAllClients(t *testing.T) {
	fr := baseRule()
	fr.ProfileID = store.EveryoneProfileID
	tr, err := Translate(fr, store.Profile{ID: store.EveryoneProfileID})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if len(tr.TargetDevices) != 1 || tr.TargetDevices[0].Type != unifi.TargetTypeAllClients {
		t.Errorf("targets = %+v", tr.TargetDevices)
	}
}

func TestTranslateProfileWithoutDevicesErrors(t *testing.T) {
	fr := baseRule()
	if _, err := Translate(fr, store.Profile{ID: "p1", Name: "Empty"}); err == nil {
		t.Fatal("profile with no devices must error, not create a matches-nothing rule")
	}
}

func TestTranslateRecurringWeekSchedule(t *testing.T) {
	fr := baseRule()
	fr.When = store.When{Kind: store.WhenRecurring, Days: []string{"sun", "mon", "tue", "wed", "thu"}, Start: "20:00", End: "07:00"}
	tr, _ := Translate(fr, emma)
	s := tr.Schedule
	if s.Mode != unifi.ModeEveryWeek || len(s.RepeatOnDays) != 5 || s.TimeRangeStart != "20:00" || s.TimeRangeEnd != "07:00" {
		t.Errorf("schedule = %+v", s)
	}
}

func TestTranslateAllSevenDaysBecomesEveryDay(t *testing.T) {
	fr := baseRule()
	fr.When = store.When{Kind: store.WhenRecurring, Days: []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}, Start: "09:00", End: "17:00"}
	tr, _ := Translate(fr, emma)
	if tr.Schedule.Mode != unifi.ModeEveryDay || len(tr.Schedule.RepeatOnDays) != 0 {
		t.Errorf("schedule = %+v", tr.Schedule)
	}
}

func TestTranslateRecurringAllDay(t *testing.T) {
	fr := baseRule()
	fr.When = store.When{Kind: store.WhenRecurring, Days: []string{"sat", "sun"}}
	tr, _ := Translate(fr, emma)
	if !tr.Schedule.TimeAllDay || tr.Schedule.TimeRangeStart != "" {
		t.Errorf("no times set → all-day: %+v", tr.Schedule)
	}
}

func TestTranslateOneTime(t *testing.T) {
	fr := baseRule()
	until := time.Date(2026, 7, 3, 21, 30, 0, 0, time.Local)
	fr.When = store.When{Kind: store.WhenOneTime, Start: "21:00", Until: until.Format(time.RFC3339)}
	tr, err := Translate(fr, emma)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	s := tr.Schedule
	if s.Mode != unifi.ModeOneTime || s.Date != "2026-07-03" || s.TimeRangeStart != "21:00" || s.TimeRangeEnd != "21:30" {
		t.Errorf("schedule = %+v", s)
	}
}

func TestTranslateOneTimeOvernightAnchorsStartDay(t *testing.T) {
	fr := baseRule()
	until := time.Date(2026, 7, 4, 7, 0, 0, 0, time.Local) // Saturday 07:00
	fr.When = store.When{Kind: store.WhenOneTime, Start: "23:30", Until: until.Format(time.RFC3339)}
	tr, err := Translate(fr, emma)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	s := tr.Schedule
	if s.Mode != unifi.ModeOneTime || s.Date != "2026-07-03" || s.TimeRangeStart != "23:30" || s.TimeRangeEnd != "07:00" {
		t.Errorf("overnight one-time must anchor date to the start day: %+v", s)
	}
}

func TestTranslateZeroPadsRecurringTimes(t *testing.T) {
	fr := baseRule()
	fr.When = store.When{Kind: store.WhenRecurring, Days: []string{"mon"}, Start: "9:00", End: "17:00"}
	tr, err := Translate(fr, emma)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if tr.Schedule.TimeRangeStart != "09:00" {
		t.Errorf("TimeRangeStart = %q, want zero-padded 09:00", tr.Schedule.TimeRangeStart)
	}
}

func TestTranslateZeroPadsOneTimeStart(t *testing.T) {
	fr := baseRule()
	until := time.Date(2026, 7, 3, 21, 30, 0, 0, time.Local)
	fr.When = store.When{Kind: store.WhenOneTime, Start: "9:00", Until: until.Format(time.RFC3339)}
	tr, err := Translate(fr, emma)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if tr.Schedule.TimeRangeStart != "09:00" {
		t.Errorf("TimeRangeStart = %q, want zero-padded 09:00", tr.Schedule.TimeRangeStart)
	}
}

func TestTranslateRejectsDegenerateRecurringWindow(t *testing.T) {
	fr := baseRule()
	fr.When = store.When{Kind: store.WhenRecurring, Days: []string{"mon"}, Start: "20:00", End: "20:00"}
	if _, err := Translate(fr, emma); err == nil {
		t.Fatal("recurring window with equal start and end must error")
	} else if !strings.Contains(err.Error(), "can't be the same") {
		t.Errorf("error = %q, want mention of times being the same", err.Error())
	}
}

func TestTranslateRejectsDegenerateOneTimeWindow(t *testing.T) {
	fr := baseRule()
	until := time.Date(2026, 7, 3, 21, 0, 0, 0, time.Local)
	fr.When = store.When{Kind: store.WhenOneTime, Start: "21:00", Until: until.Format(time.RFC3339)}
	if _, err := Translate(fr, emma); err == nil {
		t.Fatal("one-time window ending at the same time of day it starts must error")
	} else if !strings.Contains(err.Error(), "different time of day") {
		t.Errorf("error = %q, want mention of a different time of day", err.Error())
	}
}

func TestTranslateCategoryAndEverythingAndDomains(t *testing.T) {
	fr := baseRule()
	fr.What = store.What{Type: store.WhatCategory, CategoryID: "social"}
	tr, _ := Translate(fr, emma)
	if tr.MatchingTarget != unifi.MatchAppCategory || len(tr.AppCategoryIDs) != 1 || tr.AppCategoryIDs[0] != 8 {
		t.Errorf("category rule = %+v", tr)
	}

	fr.What = store.What{Type: store.WhatEverything}
	tr, _ = Translate(fr, emma)
	if tr.MatchingTarget != unifi.MatchInternet || len(tr.Domains) != 0 {
		t.Errorf("everything rule = %+v", tr)
	}

	fr.What = store.What{Type: store.WhatDomains, Domains: []string{"HTTPS://CoolMathGames.com/games?x=1"}}
	tr, err := Translate(fr, emma)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if len(tr.Domains) != 1 || tr.Domains[0].Domain != "coolmathgames.com" {
		t.Errorf("custom domains = %+v", tr.Domains)
	}
}

func TestTranslateRejectsUnknownIDs(t *testing.T) {
	fr := baseRule()
	fr.What = store.What{Type: store.WhatPreset, PresetID: "nope"}
	if _, err := Translate(fr, emma); err == nil {
		t.Error("unknown preset must error")
	}
	fr.What = store.What{Type: store.WhatCategory, CategoryID: "nope"}
	if _, err := Translate(fr, emma); err == nil {
		t.Error("unknown category must error")
	}
	fr.What = store.What{Type: store.WhatDomains, Domains: []string{"not a domain"}}
	if _, err := Translate(fr, emma); err == nil {
		t.Error("invalid domain must error")
	}
}

func TestDescriptionRoundTrip(t *testing.T) {
	d := Description("fr42", "No YouTube")
	if !IsFamilyTime(d) {
		t.Errorf("IsFamilyTime(%q) = false", d)
	}
	if got := FamilyRuleID(d); got != "fr42" {
		t.Errorf("FamilyRuleID(%q) = %q", d, got)
	}
	if IsFamilyTime("kids apps") || IsFamilyTime("") {
		t.Error("non-familytime descriptions must not match")
	}
	if FamilyRuleID("kids apps") != "" {
		t.Error("FamilyRuleID of foreign rule must be empty")
	}
}

func TestNormalizeDomain(t *testing.T) {
	good := map[string]string{
		"youtube.com":                 "youtube.com",
		"HTTPS://Example.COM/path":    "example.com",
		"http://example.com:8080/x?y": "example.com",
		"  www.example.com  ":         "www.example.com",
	}
	for in, want := range good {
		got, err := NormalizeDomain(in)
		if err != nil || got != want {
			t.Errorf("NormalizeDomain(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "nodots", "has space.com", "-bad.com"} {
		if _, err := NormalizeDomain(bad); err == nil {
			t.Errorf("NormalizeDomain(%q) should error", bad)
		}
	}
}

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	tt, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return tt
}

func TestActiveNow(t *testing.T) {
	// 2026-07-03 is a Friday.
	fri2130 := mustParse(t, "2026-07-03 21:30")
	sat0630 := mustParse(t, "2026-07-04 06:30")
	sat1200 := mustParse(t, "2026-07-04 12:00")

	always := store.When{Kind: store.WhenAlways}
	if active, _, hasUntil := ActiveNow(always, fri2130); !active || hasUntil {
		t.Error("always should be active with no until")
	}

	// Fri+Sat 21:00→07:00 (crosses midnight).
	wk := store.When{Kind: store.WhenRecurring, Days: []string{"fri", "sat"}, Start: "21:00", End: "07:00"}
	if active, until, _ := ActiveNow(wk, fri2130); !active || until.Format("2006-01-02 15:04") != "2026-07-04 07:00" {
		t.Errorf("fri 21:30 should be active until sat 07:00, got active=%v until=%v", active, until)
	}
	// Sat 06:30 is still inside Friday night's window.
	if active, until, _ := ActiveNow(wk, sat0630); !active || until.Format("15:04") != "07:00" {
		t.Errorf("sat 06:30 should be active until 07:00, got active=%v until=%v", active, until)
	}
	if active, _, _ := ActiveNow(wk, sat1200); active {
		t.Error("sat noon should be inactive")
	}
	// Sunday 06:30: Saturday is in Days, so Sat-night window reaches into Sunday morning.
	sun0630 := mustParse(t, "2026-07-05 06:30")
	if active, _, _ := ActiveNow(wk, sun0630); !active {
		t.Error("sun 06:30 should be active (sat night window)")
	}
	// Monday 06:30: Sunday not in Days → inactive.
	mon0630 := mustParse(t, "2026-07-06 06:30")
	if active, _, _ := ActiveNow(wk, mon0630); active {
		t.Error("mon 06:30 should be inactive")
	}

	// One-time.
	ot := store.When{Kind: store.WhenOneTime, Until: mustParse(t, "2026-07-03 22:00").Format(time.RFC3339)}
	if active, until, _ := ActiveNow(ot, fri2130); !active || !until.Equal(mustParse(t, "2026-07-03 22:00")) {
		t.Error("one-time should be active until 22:00")
	}
	if Expired(ot, fri2130) {
		t.Error("not yet expired at 21:30")
	}
	if !Expired(ot, mustParse(t, "2026-07-03 22:01")) {
		t.Error("expired at 22:01")
	}
	if Expired(always, fri2130) || Expired(wk, fri2130) {
		t.Error("only one-time rules expire")
	}
}

func TestOneTimeStart(t *testing.T) {
	// Overnight window 23:45 → 00:45: start anchors to the day BEFORE Until.
	w := store.When{Kind: store.WhenOneTime, Start: "23:45",
		Until: mustParse(t, "2026-07-04 00:45").Format(time.RFC3339)}
	start, ok := OneTimeStart(w)
	if !ok || !start.Equal(mustParse(t, "2026-07-03 23:45")) {
		t.Errorf("overnight start = %v ok=%v, want 2026-07-03 23:45", start, ok)
	}

	// Same-day window 20:30 → 21:30: start anchors to Until's own day.
	w = store.When{Kind: store.WhenOneTime, Start: "20:30",
		Until: mustParse(t, "2026-07-03 21:30").Format(time.RFC3339)}
	start, ok = OneTimeStart(w)
	if !ok || !start.Equal(mustParse(t, "2026-07-03 20:30")) {
		t.Errorf("same-day start = %v ok=%v, want 2026-07-03 20:30", start, ok)
	}

	// Legacy rule without a start clock → no start.
	if _, ok := OneTimeStart(store.When{Kind: store.WhenOneTime, Until: w.Until}); ok {
		t.Error("missing start clock must return ok=false")
	}
	// Non-one-time kinds have no one-time start.
	if _, ok := OneTimeStart(store.When{Kind: store.WhenAlways}); ok {
		t.Error("non-onetime kind must return ok=false")
	}
}

func TestActiveNowOneTimeStartGating(t *testing.T) {
	// Scheduled window 22:00 → 23:00.
	ot := store.When{Kind: store.WhenOneTime, Start: "22:00",
		Until: mustParse(t, "2026-07-03 23:00").Format(time.RFC3339)}

	// Before the start: pending — inactive, but until is still reported.
	active, until, hasUntil := ActiveNow(ot, mustParse(t, "2026-07-03 21:30"))
	if active || !hasUntil || !until.Equal(mustParse(t, "2026-07-03 23:00")) {
		t.Errorf("pending at 21:30: active=%v until=%v hasUntil=%v", active, until, hasUntil)
	}
	if active, _, _ := ActiveNow(ot, mustParse(t, "2026-07-03 22:30")); !active {
		t.Error("must be active at 22:30 (inside the window)")
	}
	if active, _, _ := ActiveNow(ot, mustParse(t, "2026-07-03 23:01")); active {
		t.Error("must be inactive at 23:01 (window over)")
	}

	// Overnight window 23:45 → 00:45 (crosses midnight).
	overnight := store.When{Kind: store.WhenOneTime, Start: "23:45",
		Until: mustParse(t, "2026-07-04 00:45").Format(time.RFC3339)}
	if active, _, _ := ActiveNow(overnight, mustParse(t, "2026-07-03 23:00")); active {
		t.Error("overnight window must be pending at 23:00")
	}
	if active, _, _ := ActiveNow(overnight, mustParse(t, "2026-07-04 00:10")); !active {
		t.Error("overnight window must be active at 00:10")
	}
}
