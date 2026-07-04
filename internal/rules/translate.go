// Package rules translates Family Time rules into UniFi traffic rules
// and evaluates schedules for status display. It does no I/O.
package rules

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"familytime/internal/store"
	"familytime/internal/unifi"
)

// DescriptionPrefix tags every gateway rule Family Time owns. Rules without it
// are never touched.
const DescriptionPrefix = "[family-time] "

// Description encodes ownership + the family rule id into the gateway
// rule's description: "[family-time] <ruleID> <name>".
func Description(ruleID, name string) string {
	return DescriptionPrefix + ruleID + " " + name
}

func IsFamilyTime(desc string) bool {
	return strings.HasPrefix(desc, DescriptionPrefix)
}

// FamilyRuleID extracts the family rule id from a Family Time description, or
// "" for foreign rules.
func FamilyRuleID(desc string) string {
	if !IsFamilyTime(desc) {
		return ""
	}
	rest := strings.TrimPrefix(desc, DescriptionPrefix)
	if i := strings.IndexByte(rest, ' '); i > 0 {
		return rest[:i]
	}
	return rest
}

var dayNames = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

var domainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// NormalizeDomain accepts what parents actually type — URLs, mixed case,
// stray spaces — and returns a bare lowercase domain.
func NormalizeDomain(raw string) (string, error) {
	d := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.Index(d, "://"); i >= 0 {
		d = d[i+3:]
	}
	if i := strings.IndexAny(d, "/?#"); i >= 0 {
		d = d[:i]
	}
	if i := strings.IndexByte(d, ':'); i >= 0 {
		d = d[:i]
	}
	if !domainRe.MatchString(d) {
		return "", fmt.Errorf("%q doesn't look like a website address", raw)
	}
	return d, nil
}

// Translate converts one family rule into the single UniFi traffic rule
// that enforces it. The profile must be the rule's target (or the virtual
// Everyone profile).
func Translate(fr store.FamilyRule, p store.Profile) (unifi.TrafficRule, error) {
	tr := unifi.NewBlockRule()
	tr.Description = Description(fr.ID, fr.Name)
	tr.Enabled = fr.Enabled

	// Who.
	if fr.ProfileID == store.EveryoneProfileID {
		tr.TargetDevices = []unifi.TargetDevice{{Type: unifi.TargetTypeAllClients}}
	} else {
		if len(p.Devices) == 0 {
			return tr, fmt.Errorf("profile %q has no devices yet — add a device first", p.Name)
		}
		for _, d := range p.Devices {
			tr.TargetDevices = append(tr.TargetDevices, unifi.TargetDevice{
				ClientMAC: d.MAC, Type: unifi.TargetTypeClient,
			})
		}
	}

	// What.
	switch fr.What.Type {
	case store.WhatPreset:
		preset, ok := PresetByID(fr.What.PresetID)
		if !ok {
			return tr, fmt.Errorf("unknown app preset %q", fr.What.PresetID)
		}
		tr.MatchingTarget = unifi.MatchDomain
		for _, d := range preset.Domains {
			tr.Domains = append(tr.Domains, unifi.Domain{Domain: d, Ports: []int{}, PortRanges: []any{}})
		}
	case store.WhatCategory:
		cat, ok := CategoryByID(fr.What.CategoryID)
		if !ok {
			return tr, fmt.Errorf("unknown category %q", fr.What.CategoryID)
		}
		tr.MatchingTarget = unifi.MatchAppCategory
		tr.AppCategoryIDs = []int{cat.UnifiID}
	case store.WhatDomains:
		if len(fr.What.Domains) == 0 {
			return tr, fmt.Errorf("no websites given")
		}
		tr.MatchingTarget = unifi.MatchDomain
		for _, raw := range fr.What.Domains {
			d, err := NormalizeDomain(raw)
			if err != nil {
				return tr, err
			}
			tr.Domains = append(tr.Domains, unifi.Domain{Domain: d, Ports: []int{}, PortRanges: []any{}})
		}
	case store.WhatEverything:
		tr.MatchingTarget = unifi.MatchInternet
	default:
		return tr, fmt.Errorf("unknown block type %q", fr.What.Type)
	}

	// When.
	sched, err := translateSchedule(fr.When)
	if err != nil {
		return tr, err
	}
	tr.Schedule = sched
	return tr, nil
}

func translateSchedule(w store.When) (unifi.Schedule, error) {
	s := unifi.Schedule{RepeatOnDays: []string{}}
	switch w.Kind {
	case store.WhenAlways:
		s.Mode = unifi.ModeAlways
	case store.WhenRecurring:
		valid := map[string]bool{}
		for _, d := range dayNames {
			valid[d] = true
		}
		for _, d := range w.Days {
			if !valid[d] {
				return s, fmt.Errorf("unknown day %q", d)
			}
		}
		if len(w.Days) == 0 {
			return s, fmt.Errorf("pick at least one day")
		}
		if len(w.Days) == 7 {
			s.Mode = unifi.ModeEveryDay
		} else {
			s.Mode = unifi.ModeEveryWeek
			s.RepeatOnDays = append(s.RepeatOnDays, w.Days...)
		}
		if w.Start == "" && w.End == "" {
			s.TimeAllDay = true
		} else {
			startM, err := parseHM(w.Start)
			if err != nil {
				return s, fmt.Errorf("bad start time %q", w.Start)
			}
			endM, err := parseHM(w.End)
			if err != nil {
				return s, fmt.Errorf("bad end time %q", w.End)
			}
			if startM == endM {
				return s, fmt.Errorf("start and end times can't be the same")
			}
			s.TimeRangeStart = fmt.Sprintf("%02d:%02d", startM/60, startM%60)
			s.TimeRangeEnd = fmt.Sprintf("%02d:%02d", endM/60, endM%60)
		}
	case store.WhenOneTime:
		until, err := time.Parse(time.RFC3339, w.Until)
		if err != nil {
			return s, fmt.Errorf("bad until time: %w", err)
		}
		if w.Start == "" {
			return s, fmt.Errorf("one-time rule needs a start time")
		}
		startM, err := parseHM(w.Start)
		if err != nil {
			return s, fmt.Errorf("bad start time %q", w.Start)
		}
		endM := until.Hour()*60 + until.Minute()
		if startM == endM {
			return s, fmt.Errorf("the pause needs to end at a different time of day than it starts")
		}
		s.Mode = unifi.ModeOneTime
		// Anchor the date to the window's START day: an overnight pause
		// created at 23:30 ending 07:00 tomorrow carries today's date with
		// the range crossing midnight (accepted natively by the gateway).
		// Same-day windows are unaffected.
		date := until
		if endM < startM {
			date = until.AddDate(0, 0, -1)
		}
		s.Date = date.Format("2006-01-02")
		s.TimeRangeStart = fmt.Sprintf("%02d:%02d", startM/60, startM%60)
		s.TimeRangeEnd = fmt.Sprintf("%02d:%02d", endM/60, endM%60)
	default:
		return s, fmt.Errorf("unknown schedule kind %q", w.Kind)
	}
	return s, nil
}

// parseHM parses "21:05" into minutes since midnight.
func parseHM(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}

func dayInList(days []string, wd time.Weekday) bool {
	name := dayNames[int(wd)]
	for _, d := range days {
		if d == name {
			return true
		}
	}
	return false
}

// ActiveNow reports whether the schedule is currently enforcing, and when
// the current window ends (hasUntil=false for open-ended rules).
func ActiveNow(w store.When, now time.Time) (bool, time.Time, bool) {
	switch w.Kind {
	case store.WhenAlways:
		return true, time.Time{}, false
	case store.WhenOneTime:
		until, err := time.Parse(time.RFC3339, w.Until)
		if err != nil {
			return false, time.Time{}, false
		}
		if start, ok := OneTimeStart(w); ok && now.Before(start) {
			return false, until, true // scheduled, not yet started
		}
		return now.Before(until), until, true
	case store.WhenRecurring:
		if w.Start == "" && w.End == "" {
			active := dayInList(w.Days, now.Weekday())
			if !active {
				return false, time.Time{}, false
			}
			midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
			return true, midnight, true
		}
		start, err1 := parseHM(w.Start)
		end, err2 := parseHM(w.End)
		if err1 != nil || err2 != nil {
			return false, time.Time{}, false
		}
		nowM := now.Hour()*60 + now.Minute()
		at := func(day time.Time, minutes int) time.Time {
			return time.Date(day.Year(), day.Month(), day.Day(), minutes/60, minutes%60, 0, 0, now.Location())
		}
		if start < end {
			if dayInList(w.Days, now.Weekday()) && nowM >= start && nowM < end {
				return true, at(now, end), true
			}
			return false, time.Time{}, false
		}
		// Crosses midnight: evening part belongs to today, morning part to yesterday's day-name.
		if dayInList(w.Days, now.Weekday()) && nowM >= start {
			return true, at(now.AddDate(0, 0, 1), end), true
		}
		yesterday := time.Weekday((int(now.Weekday()) + 6) % 7)
		if dayInList(w.Days, yesterday) && nowM < end {
			return true, at(now, end), true
		}
		return false, time.Time{}, false
	}
	return false, time.Time{}, false
}

// OneTimeStart derives the moment a one-time window opens: the stored start
// clock anchored to Until's date, minus a day when the window crosses
// midnight — the same anchoring translateSchedule uses. ok=false when the
// rule has no parseable start (legacy rules stored only Until).
func OneTimeStart(w store.When) (time.Time, bool) {
	if w.Kind != store.WhenOneTime || w.Start == "" {
		return time.Time{}, false
	}
	until, err := time.Parse(time.RFC3339, w.Until)
	if err != nil {
		return time.Time{}, false
	}
	startM, err := parseHM(w.Start)
	if err != nil {
		return time.Time{}, false
	}
	endM := until.Hour()*60 + until.Minute()
	day := until
	if endM < startM {
		day = until.AddDate(0, 0, -1)
	}
	return time.Date(day.Year(), day.Month(), day.Day(), startM/60, startM%60, 0, 0, until.Location()), true
}

// Expired reports whether a one-time rule's window has fully passed —
// used by the janitor to clean up spent pause rules.
func Expired(w store.When, now time.Time) bool {
	if w.Kind != store.WhenOneTime {
		return false
	}
	until, err := time.Parse(time.RFC3339, w.Until)
	if err != nil {
		return true // unparseable one-time rule is garbage — collect it
	}
	return now.After(until)
}
