# Task 3 Report: One-Time Overnight Schedule Fixes

## Fix: one-time overnight date anchor

### Problem
In `internal/rules/translate.go`, the `translateSchedule` function's `store.WhenOneTime` branch was anchoring overnight one-time windows to the END day instead of the START day. For example, a pause created at 23:30 ending 07:00 the next day would incorrectly use tomorrow's date instead of today's date.

### Solution Applied
Modified the one-time schedule logic to detect when the time range crosses midnight (end time ≤ start time) and anchor the date to the previous day when appropriate.

#### Code Changes

**File: `/Users/dsandor/Projects/bedtime/internal/rules/translate.go` (lines 179-182)**

Before:
```go
s.Mode = unifi.ModeOneTime
s.Date = until.Format("2006-01-02")
s.TimeRangeStart = w.Start
s.TimeRangeEnd = until.Format("15:04")
```

After:
```go
s.Mode = unifi.ModeOneTime
// Anchor the date to the window's START day: an overnight pause
// created at 23:30 ending 07:00 tomorrow carries today's date with
// the range crossing midnight (accepted natively by the gateway).
// Same-day windows are unaffected.
startM, _ := parseHM(w.Start) // already validated above
endM := until.Hour()*60 + until.Minute()
date := until
if endM <= startM {
	date = until.AddDate(0, 0, -1)
}
s.Date = date.Format("2006-01-02")
s.TimeRangeStart = w.Start
s.TimeRangeEnd = until.Format("15:04")
```

**File: `/Users/dsandor/Projects/bedtime/internal/rules/translate_test.go` (after line 113)**

Added test:
```go
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
```

### Test Results

```
$ go test ./internal/rules/ -v
=== RUN   TestTranslatePresetForProfile
--- PASS: TestTranslatePresetForProfile (0.00s)
=== RUN   TestTranslateEveryoneUsesAllClients
--- PASS: TestTranslateEveryoneUsesAllClients (0.00s)
=== RUN   TestTranslateProfileWithoutDevicesErrors
--- PASS: TestTranslateProfileWithoutDevicesErrors (0.00s)
=== RUN   TestTranslateRecurringWeekSchedule
--- PASS: TestTranslateRecurringWeekSchedule (0.00s)
=== RUN   TestTranslateAllSevenDaysBecomesEveryDay
--- PASS: TestTranslateAllSevenDaysBecomesEveryDay (0.00s)
=== RUN   TestTranslateRecurringAllDay
--- PASS: TestTranslateRecurringAllDay (0.00s)
=== RUN   TestTranslateOneTime
--- PASS: TestTranslateOneTime (0.00s)
=== RUN   TestTranslateOneTimeOvernightAnchorsStartDay
--- PASS: TestTranslateOneTimeOvernightAnchorsStartDay (0.00s)
=== RUN   TestTranslateCategoryAndEverythingAndDomains
--- PASS: TestTranslateCategoryAndEverythingAndDomains (0.00s)
=== RUN   TestTranslateRejectsUnknownIDs
--- PASS: TestTranslateRejectsUnknownIDs (0.00s)
=== RUN   TestDescriptionRoundTrip
--- PASS: TestDescriptionRoundTrip (0.00s)
=== RUN   TestNormalizeDomain
--- PASS: TestNormalizeDomain (0.00s)
=== RUN   TestActiveNow
--- PASS: TestActiveNow (0.00s)
PASS
ok  	bedtime/internal/rules	0.240s
```

### Full Verification

```
$ go build ./... && go vet ./... && go test ./...
ok  	bedtime/internal/rules	0.168s
ok  	bedtime/internal/store	(cached)
ok  	bedtime/internal/unifi	(cached)
```

### Summary
- Fixed overnight one-time window date anchoring to use START day instead of END day
- Added comprehensive test case covering the overnight scenario (23:30 to 07:00)
- All 13 tests in internal/rules pass (was 12, now 13)
- Full build, vet, and test suite passes with no issues
