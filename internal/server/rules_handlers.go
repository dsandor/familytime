package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"familytime/internal/rules"
	"familytime/internal/store"
	"familytime/internal/unifi"
)

type ruleInput struct {
	ProfileID string     `json:"profileId"`
	Name      string     `json:"name"`
	What      store.What `json:"what"`
	When      store.When `json:"when"`
	Enabled   *bool      `json:"enabled"`
}

// createFamilyRule translates and applies a family rule gateway-first, so
// the store never references a rule the gateway doesn't have. Returns the
// stored rule and an HTTP status (0 means: call failErr with err).
func (s *Server) createFamilyRule(ctx context.Context, fr store.FamilyRule, p store.Profile) (store.FamilyRule, int, error) {
	tr, err := rules.Translate(fr, p)
	if err != nil {
		return fr, http.StatusBadRequest, err
	}
	created, err := s.api().CreateTrafficRule(ctx, tr)
	if err != nil {
		return fr, 0, err
	}
	fr.UnifiRuleIDs = []string{created.ID}
	err = s.store.Update(func(d *store.Data) error {
		d.Rules = append(d.Rules, fr)
		return nil
	})
	if err != nil {
		// Compensate: don't leave an untracked rule enforcing on the gateway.
		if derr := s.deleteGatewayRules(ctx, fr.UnifiRuleIDs); derr != nil {
			log.Printf("familytime: compensation delete failed (janitor will reconcile): %v", derr)
		}
		return fr, http.StatusInternalServerError, err
	}
	return fr, http.StatusOK, nil
}

func (s *Server) handleRuleCreate(w http.ResponseWriter, r *http.Request) {
	var in ruleInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "Bad request.")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		fail(w, 400, "Give this rule a name.")
		return
	}
	d := s.store.Snapshot()
	p, ok := profileForID(d, in.ProfileID)
	if !ok {
		fail(w, 404, "No such profile.")
		return
	}
	fr := store.FamilyRule{
		ID: store.NewID(), ProfileID: p.ID, Name: in.Name,
		What: in.What, When: in.When,
		Enabled: in.Enabled == nil || *in.Enabled,
	}
	fr, code, err := s.createFamilyRule(r.Context(), fr, p)
	if err != nil {
		if code == 0 {
			failErr(w, err)
		} else {
			fail(w, code, err.Error())
		}
		return
	}
	writeJSON(w, 200, fr)
}

func (s *Server) handleRuleUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in ruleInput
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "Bad request.")
		return
	}
	d := s.store.Snapshot()
	var existing *store.FamilyRule
	for i := range d.Rules {
		if d.Rules[i].ID == id {
			existing = &d.Rules[i]
			break
		}
	}
	if existing == nil {
		fail(w, 404, "No such rule.")
		return
	}
	p, ok := profileForID(d, existing.ProfileID)
	if !ok {
		fail(w, 500, "Rule's profile is missing.")
		return
	}
	updated := *existing
	updated.Name = strings.TrimSpace(in.Name)
	updated.What, updated.When = in.What, in.When
	if in.Enabled != nil {
		updated.Enabled = *in.Enabled
	}
	if updated.Name == "" {
		fail(w, 400, "Give this rule a name.")
		return
	}
	tr, err := rules.Translate(updated, p)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	// Rewrite the existing gateway rule; if it vanished (deleted in the
	// UniFi app), recreate it.
	if len(updated.UnifiRuleIDs) == 1 {
		tr.ID = updated.UnifiRuleIDs[0]
		err = s.api().UpdateTrafficRule(r.Context(), tr)
	} else {
		err = unifi.ErrNotFound
	}
	if errors.Is(err, unifi.ErrNotFound) {
		// Defensive: clear any stale gateway ids so the recreate can't leave
		// untracked rules enforcing (deleteGatewayRules tolerates not-found).
		if derr := s.deleteGatewayRules(r.Context(), updated.UnifiRuleIDs); derr != nil {
			failErr(w, derr)
			return
		}
		tr.ID = ""
		created, cerr := s.api().CreateTrafficRule(r.Context(), tr)
		if cerr != nil {
			failErr(w, cerr)
			return
		}
		updated.UnifiRuleIDs = []string{created.ID}
		err = nil
	}
	if err != nil {
		failErr(w, err)
		return
	}
	serr := s.store.Update(func(d *store.Data) error {
		for i := range d.Rules {
			if d.Rules[i].ID == id {
				d.Rules[i] = updated
			}
		}
		return nil
	})
	if serr != nil {
		fail(w, 500, serr.Error())
		return
	}
	writeJSON(w, 200, updated)
}

func (s *Server) handleRuleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d := s.store.Snapshot()
	var target *store.FamilyRule
	for i := range d.Rules {
		if d.Rules[i].ID == id {
			target = &d.Rules[i]
			break
		}
	}
	if target == nil {
		fail(w, 404, "No such rule.")
		return
	}
	if err := s.deleteGatewayRules(r.Context(), target.UnifiRuleIDs); err != nil {
		failErr(w, err)
		return
	}
	serr := s.store.Update(func(d *store.Data) error {
		for i := range d.Rules {
			if d.Rules[i].ID == id {
				d.Rules = append(d.Rules[:i], d.Rules[i+1:]...)
				break
			}
		}
		return nil
	})
	if serr != nil {
		fail(w, 500, serr.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type ruleView struct {
	store.FamilyRule
	Active   bool   `json:"active"`
	Until    string `json:"until,omitempty"`
	StartsAt string `json:"startsAt,omitempty"` // one-time rule scheduled but not yet started
}

func (s *Server) handleRulesList(w http.ResponseWriter, r *http.Request) {
	d := s.store.Snapshot()
	now := s.now()
	out := []ruleView{}
	for _, fr := range d.Rules {
		v := ruleView{FamilyRule: fr}
		if fr.Enabled {
			active, until, hasUntil := rules.ActiveNow(fr.When, now)
			v.Active = active
			if hasUntil {
				v.Until = until.Format(time.RFC3339)
			}
			if start, ok := rules.OneTimeStart(fr.When); ok && now.Before(start) {
				v.StartsAt = start.Format(time.RFC3339)
			}
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
}

// removePauseRules deletes all pause rules for a profile, gateway-first.
func (s *Server) removePauseRules(ctx context.Context, profileID string) error {
	d := s.store.Snapshot()
	var ids []string
	for _, fr := range d.Rules {
		if fr.ProfileID == profileID && fr.Pause {
			ids = append(ids, fr.UnifiRuleIDs...)
		}
	}
	if err := s.deleteGatewayRules(ctx, ids); err != nil {
		return err
	}
	return s.store.Update(func(d *store.Data) error {
		kept := d.Rules[:0]
		for _, fr := range d.Rules {
			if !(fr.ProfileID == profileID && fr.Pause) {
				kept = append(kept, fr)
			}
		}
		d.Rules = kept
		return nil
	})
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ProfileID string `json:"profileId"`
		Duration  string `json:"duration"`
		Delay     string `json:"delay"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "Bad request.")
		return
	}
	d := s.store.Snapshot()
	p, ok := profileForID(d, in.ProfileID)
	if !ok {
		fail(w, 404, "No such profile.")
		return
	}
	now := s.now()
	// "You have 30 more minutes": an optional delay shifts the whole window
	// forward — the gateway enforces the future start natively.
	start := now
	switch in.Delay {
	case "":
	case "15m":
		start = now.Add(15 * time.Minute)
	case "30m":
		start = now.Add(30 * time.Minute)
	case "1h":
		start = now.Add(time.Hour)
	default:
		fail(w, 400, "Unknown pause delay.")
		return
	}
	var when store.When
	switch in.Duration {
	case "15m":
		when = oneTimeUntil(start, start.Add(15*time.Minute))
	case "30m":
		when = oneTimeUntil(start, start.Add(30*time.Minute))
	case "1h":
		when = oneTimeUntil(start, start.Add(time.Hour))
	case "morning":
		when = oneTimeUntil(start, nextMorning(start))
	case "indefinite":
		if in.Delay != "" {
			fail(w, 400, `"Until I resume" starts right away — pick a timed duration to schedule ahead.`)
			return
		}
		when = store.When{Kind: store.WhenAlways}
	default:
		fail(w, 400, "Unknown pause duration.")
		return
	}
	// Replace any existing pause — tapping Pause twice must not stack rules.
	if err := s.removePauseRules(r.Context(), p.ID); err != nil {
		failErr(w, err)
		return
	}
	fr := store.FamilyRule{
		ID: store.NewID(), ProfileID: p.ID, Name: "Internet pause",
		What: store.What{Type: store.WhatEverything}, When: when,
		Enabled: true, Pause: true,
	}
	fr, code, err := s.createFamilyRule(r.Context(), fr, p)
	if err != nil {
		if code == 0 {
			failErr(w, err)
		} else {
			fail(w, code, err.Error())
		}
		return
	}
	writeJSON(w, 200, fr)
}

func oneTimeUntil(now, until time.Time) store.When {
	return store.When{
		Kind:  store.WhenOneTime,
		Start: now.Format("15:04"),
		Until: until.Format(time.RFC3339),
	}
}

func (s *Server) handleUnpause(w http.ResponseWriter, r *http.Request) {
	if err := s.removePauseRules(r.Context(), r.PathValue("profileId")); err != nil {
		failErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// nextMorning returns the first 7:00 AM strictly after t.
func nextMorning(t time.Time) time.Time {
	m := time.Date(t.Year(), t.Month(), t.Day(), 7, 0, 0, 0, t.Location())
	if !t.Before(m) {
		m = m.AddDate(0, 0, 1)
	}
	return m
}

// handlePauseDelay grants more internet time on an existing pause: the pause
// lifts (or its scheduled start pushes out) and re-engages later, keeping its
// original end. A grant that outlasts the pause removes it instead.
func (s *Server) handlePauseDelay(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Delay string `json:"delay"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "Bad request.")
		return
	}
	var delay time.Duration
	switch in.Delay {
	case "15m":
		delay = 15 * time.Minute
	case "30m":
		delay = 30 * time.Minute
	case "1h":
		delay = time.Hour
	default:
		fail(w, 400, "Unknown pause delay.")
		return
	}
	d := s.store.Snapshot()
	p, ok := profileForID(d, r.PathValue("profileId"))
	if !ok {
		fail(w, 404, "No such profile.")
		return
	}
	var pr *store.FamilyRule
	for i := range d.Rules {
		if d.Rules[i].ProfileID == p.ID && d.Rules[i].Pause {
			pr = &d.Rules[i]
			break
		}
	}
	if pr == nil {
		fail(w, 404, "Nothing is paused.")
		return
	}
	now := s.now()
	base := now
	var until time.Time
	switch pr.When.Kind {
	case store.WhenAlways:
		// "Until I resume" has no end; a scheduled restart needs one.
		until = nextMorning(base.Add(delay))
	case store.WhenOneTime:
		t, err := time.Parse(time.RFC3339, pr.When.Until)
		if err != nil {
			fail(w, 500, "This pause looks corrupted — try Resume instead.")
			return
		}
		until = t
		if start, ok := rules.OneTimeStart(pr.When); ok && now.Before(start) {
			base = start // pending: "+30 min" counts from the promised start
		}
	default:
		fail(w, 400, "This pause can't be extended.")
		return
	}
	newStart := base.Add(delay)
	// The grant outlasts the pause — lifting it is the whole grant.
	collapse := !until.After(newStart.Add(time.Minute))
	var fr store.FamilyRule
	if !collapse {
		fr = store.FamilyRule{
			ID: store.NewID(), ProfileID: p.ID, Name: "Internet pause",
			What: store.What{Type: store.WhatEverything}, When: oneTimeUntil(newStart, until),
			Enabled: true, Pause: true,
		}
		// Dry-run the translation before touching the existing pause: a
		// degenerate re-engage window (start and end landing in the same
		// clock minute) fails translation, and the pause must survive that.
		if _, err := rules.Translate(fr, p); err != nil {
			fail(w, 400, err.Error())
			return
		}
	}
	if err := s.removePauseRules(r.Context(), p.ID); err != nil {
		failErr(w, err)
		return
	}
	if collapse {
		writeJSON(w, 200, map[string]bool{"removed": true})
		return
	}
	fr, code, err := s.createFamilyRule(r.Context(), fr, p)
	if err != nil {
		if code == 0 {
			failErr(w, err)
		} else {
			fail(w, code, err.Error())
		}
		return
	}
	writeJSON(w, 200, fr)
}

type statusLine struct {
	RuleID string `json:"ruleId"`
	Name   string `json:"name"`
	Label  string `json:"label"`
	Active bool   `json:"active"`
	Until  string `json:"until,omitempty"`
	Pause  bool   `json:"pause"`
}

type profileStatus struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Emoji       string       `json:"emoji"`
	Color       string       `json:"color"`
	DeviceCount int          `json:"deviceCount"`
	Paused      bool         `json:"paused"`
	PausedUntil string       `json:"pausedUntil,omitempty"`
	Lines       []statusLine `json:"lines"`
}

func whatLabel(wt store.What) string {
	switch wt.Type {
	case store.WhatPreset:
		if p, ok := rules.PresetByID(wt.PresetID); ok {
			return p.Name
		}
		return wt.PresetID
	case store.WhatCategory:
		if c, ok := rules.CategoryByID(wt.CategoryID); ok {
			return c.Name
		}
		return wt.CategoryID
	case store.WhatDomains:
		return strings.Join(wt.Domains, ", ")
	case store.WhatEverything:
		return "All internet"
	}
	return wt.Type
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	d := s.store.Snapshot()
	now := s.now()
	everyone, _ := profileForID(d, store.EveryoneProfileID)
	profiles := append([]store.Profile{everyone}, d.Profiles...)
	sort.SliceStable(profiles[1:], func(i, j int) bool { return profiles[i+1].Name < profiles[j+1].Name })

	out := []profileStatus{}
	for _, p := range profiles {
		ps := profileStatus{ID: p.ID, Name: p.Name, Emoji: p.Emoji, Color: p.Color, DeviceCount: len(p.Devices), Lines: []statusLine{}}
		for _, fr := range d.Rules {
			if fr.ProfileID != p.ID || !fr.Enabled {
				continue
			}
			active, until, hasUntil := rules.ActiveNow(fr.When, now)
			line := statusLine{RuleID: fr.ID, Name: fr.Name, Label: whatLabel(fr.What), Active: active, Pause: fr.Pause}
			if hasUntil {
				line.Until = until.Format(time.RFC3339)
			}
			ps.Lines = append(ps.Lines, line)
			if fr.Pause && active {
				ps.Paused = true
				ps.PausedUntil = line.Until
			}
		}
		out = append(out, ps)
	}
	writeJSON(w, 200, out)
}
