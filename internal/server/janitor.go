package server

import (
	"context"
	"log"
	"time"

	"familytime/internal/rules"
	"familytime/internal/store"
)

// CleanupOnce reconciles app state with the gateway:
//
//  1. expired one-time rules (spent pauses) are deleted from the gateway
//     and the store — enforcement already ended, this is cosmetic tidying;
//  2. store metadata whose gateway rule vanished (deleted in the UniFi app)
//     is forgotten;
//  3. [family-time]-tagged gateway rules the store doesn't know (e.g. left by a
//     crashed compensation) are deleted.
//
// Cases 2 and 3 act only on the second consecutive sighting (s.suspects) so
// an in-flight write can't be mistaken for drift. Foreign rules — anything
// without the [family-time] prefix — are never touched.
func (s *Server) CleanupOnce(ctx context.Context) error {
	if !s.store.IsConfigured() {
		return nil
	}
	gw, err := s.api().ListTrafficRules(ctx)
	if err != nil {
		return err
	}
	onGateway := map[string]bool{}
	familytimeGw := map[string]bool{}
	for _, r := range gw {
		onGateway[r.ID] = true
		if rules.IsFamilyTime(r.Description) {
			familytimeGw[r.ID] = true
		}
	}
	now := s.now()
	d := s.store.Snapshot()

	suspectedNow := map[string]bool{}
	var deleteIDs []string
	drop := map[string]bool{}
	tracked := map[string]bool{}

	for _, fr := range d.Rules {
		for _, id := range fr.UnifiRuleIDs {
			tracked[id] = true
		}
		if rules.Expired(fr.When, now) {
			drop[fr.ID] = true
			deleteIDs = append(deleteIDs, fr.UnifiRuleIDs...)
			continue
		}
		alive := false
		for _, id := range fr.UnifiRuleIDs {
			if onGateway[id] {
				alive = true
			}
		}
		if !alive {
			suspectedNow["meta:"+fr.ID] = true
			if s.suspects["meta:"+fr.ID] {
				drop[fr.ID] = true
			}
		}
	}
	for id := range familytimeGw {
		if !tracked[id] {
			suspectedNow["gw:"+id] = true
			if s.suspects["gw:"+id] {
				deleteIDs = append(deleteIDs, id)
			}
		}
	}
	s.suspects = suspectedNow

	if err := s.deleteGatewayRules(ctx, deleteIDs); err != nil {
		return err
	}
	if len(drop) == 0 {
		return nil
	}
	return s.store.Update(func(d *store.Data) error {
		kept := d.Rules[:0]
		for _, fr := range d.Rules {
			if !drop[fr.ID] {
				kept = append(kept, fr)
			}
		}
		d.Rules = kept
		return nil
	})
}

// RunJanitor cleans immediately, then on every tick until ctx ends. Errors
// are logged and retried next tick — the gateway may be briefly offline.
func (s *Server) RunJanitor(ctx context.Context, interval time.Duration) {
	if err := s.CleanupOnce(ctx); err != nil {
		log.Printf("janitor: %v", err)
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.CleanupOnce(ctx); err != nil {
				log.Printf("janitor: %v", err)
			}
		}
	}
}
