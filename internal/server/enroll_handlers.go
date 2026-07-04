package server

import (
	"log"
	"net"
	"net/http"
	"strings"

	"familytime/internal/store"
	"familytime/internal/unifi"
)

// requesterIP extracts the caller's IP from an http.Request's RemoteAddr,
// stripping the port. Falls back to the raw value if it isn't a host:port
// pair, so a malformed address can't be silently treated as "no IP" (it
// just won't match any live client, which is the safe failure mode).
func requesterIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// findClientByIP returns the live gateway client whose IP matches ip. This
// is the only way enroll identifies a device — a client-supplied MAC is
// never trusted, since the entire point of enrollment is working around
// clients that can't (or won't, via Private Wi-Fi Address) report their
// real MAC honestly.
func findClientByIP(clients []unifi.NetClient, ip string) (unifi.NetClient, bool) {
	if ip == "" {
		return unifi.NetClient{}, false
	}
	for _, c := range clients {
		if c.IPAddress == ip {
			return c, true
		}
	}
	return unifi.NetClient{}, false
}

type enrollGroup struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
	Color string `json:"color"`
}

// enrollGroups lists the family's real groups for the enroll picker — never
// the virtual Everyone profile, which isn't a valid enrollment target.
func enrollGroups(d store.Data) []enrollGroup {
	out := make([]enrollGroup, 0, len(d.Profiles))
	for _, p := range d.Profiles {
		out = append(out, enrollGroup{ID: p.ID, Name: p.Name, Emoji: p.Emoji, Color: p.Color})
	}
	return out
}

// handleEnrollWhoami identifies the calling device by matching its
// requesting IP against the gateway's live client list. Public — no
// session required — because it can only ever describe the device making
// the request, never anyone else's.
func (s *Server) handleEnrollWhoami(w http.ResponseWriter, r *http.Request) {
	if !s.store.IsConfigured() {
		fail(w, http.StatusConflict, "Family Time isn't set up yet.")
		return
	}
	d := s.store.Snapshot()
	groups := enrollGroups(d)

	live, err := s.api().ListClients(r.Context(), d.Gateway.SiteID)
	if err != nil {
		failErr(w, err)
		return
	}
	client, ok := findClientByIP(live, requesterIP(r.RemoteAddr))
	if !ok {
		writeJSON(w, 200, map[string]any{"found": false, "groups": groups})
		return
	}
	mac := strings.ToLower(client.MACAddress)
	profileID, deviceName := "", ""
	for _, p := range d.Profiles {
		for _, dev := range p.Devices {
			if dev.MAC == mac {
				profileID, deviceName = p.ID, dev.Name
			}
		}
	}
	writeJSON(w, 200, map[string]any{
		"found":             true,
		"mac":               mac,
		"name":              client.Name,
		"ip":                client.IPAddress,
		"currentProfileId":  profileID,
		"currentDeviceName": deviceName,
		"groups":            groups,
	})
}

// handleEnroll assigns the calling device — identified the same IP-to-
// client way as whoami — to a group. The request body's name is trusted
// (it's just a label), but never a MAC: the server re-resolves it fresh
// from RemoteAddr, exactly like whoami, so nothing the client claims about
// its own identity is ever trusted.
//
// Gateway-first, same as profile editing: every affected profile's rules
// (the device's old group, if it's moving, and its new one) are re-targeted
// on the gateway before the store is updated. A mid-loop failure still
// persists the new intent (via persistPartialEnroll) rather than leaving
// the UI showing a stale assignment while the gateway has already started
// enforcing the new one — see retargetProfileRules for the full rationale.
func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if !s.store.IsConfigured() {
		fail(w, http.StatusConflict, "Family Time isn't set up yet.")
		return
	}
	var in struct{ Name, ProfileID string }
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "Bad request.")
		return
	}
	d := s.store.Snapshot()

	var target store.Profile
	found := false
	for _, p := range d.Profiles {
		if p.ID == in.ProfileID {
			target, found = p, true
			break
		}
	}
	if !found {
		// Note: the virtual Everyone profile is never in d.Profiles, so
		// this also naturally rejects profileId="everyone".
		fail(w, 400, "Pick a valid group.")
		return
	}

	live, err := s.api().ListClients(r.Context(), d.Gateway.SiteID)
	if err != nil {
		failErr(w, err)
		return
	}
	client, ok := findClientByIP(live, requesterIP(r.RemoteAddr))
	if !ok {
		fail(w, http.StatusNotFound, "Couldn't spot this device on your network. Make sure you're on the home Wi-Fi (not cellular or a VPN) and try again.")
		return
	}
	mac := strings.ToLower(client.MACAddress)
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = client.Name
	}
	if name == "" {
		name = mac
	}

	// Find the profile (other than the target) that currently owns this
	// MAC, if any — a device already assigned somewhere else is moving,
	// not just being newly assigned.
	var source *store.Profile
	for i := range d.Profiles {
		if d.Profiles[i].ID == target.ID {
			continue
		}
		for _, dev := range d.Profiles[i].Devices {
			if dev.MAC == mac {
				p := d.Profiles[i]
				source = &p
				break
			}
		}
		if source != nil {
			break
		}
	}

	updates := map[string]store.Profile{}
	// steps holds, in gateway-first processing order (source before
	// target), each profile that needs its rules re-targeted alongside the
	// rules it owns — computed once up front so the zero-devices-with-
	// rules guard below and the retarget loop after it see the same data.
	type step struct {
		profile store.Profile
		rules   []store.FamilyRule
	}
	rulesFor := func(profileID string) []store.FamilyRule {
		var out []store.FamilyRule
		for _, fr := range d.Rules {
			if fr.ProfileID == profileID {
				out = append(out, fr)
			}
		}
		return out
	}
	var steps []step
	if source != nil {
		s2 := *source
		devs := make([]store.Device, 0, len(s2.Devices))
		for _, dev := range s2.Devices {
			if dev.MAC != mac {
				devs = append(devs, dev)
			}
		}
		s2.Devices = devs
		updates[s2.ID] = s2
		steps = append(steps, step{s2, rulesFor(s2.ID)})
	}
	replaced := false
	devs := make([]store.Device, 0, len(target.Devices)+1)
	for _, dev := range target.Devices {
		if dev.MAC == mac {
			devs = append(devs, store.Device{MAC: mac, Name: name})
			replaced = true
		} else {
			devs = append(devs, dev)
		}
	}
	if !replaced {
		devs = append(devs, store.Device{MAC: mac, Name: name})
	}
	target.Devices = devs
	updates[target.ID] = target
	steps = append(steps, step{target, rulesFor(target.ID)})

	// Same guard as handleProfileUpdate: never leave a profile with rules
	// but zero devices (which would make those rules untranslatable). This
	// only matters for the source profile — the target always gains a
	// device here — but checking both keeps the two code paths identical.
	for _, st := range steps {
		if len(st.profile.Devices) == 0 && len(st.rules) > 0 {
			fail(w, 400, "This profile still has rules — keep at least one device, or delete the rules first.")
			return
		}
	}

	recreated := map[string][]string{}
	for _, st := range steps {
		got, rerr := s.retargetProfileRules(r.Context(), st.profile, st.rules)
		for k, v := range got {
			recreated[k] = v
		}
		if rerr != nil {
			s.persistPartialEnroll(updates, recreated)
			respondRetargetError(w, rerr)
			return
		}
	}

	if err := s.persistProfilesAndRuleIDs(updates, recreated); err != nil {
		fail(w, 500, err.Error())
		return
	}
	// Best-effort: push the final name to UniFi as the client's alias so
	// both UIs agree. Never fails the enrollment — a device is already
	// enrolled by this point.
	if err := s.api().RenameClient(r.Context(), mac, name); err != nil {
		log.Printf("familytime: unifi rename failed for %s: %v", mac, err)
	}
	writeJSON(w, 200, map[string]any{"ok": true, "groupName": target.Name, "deviceName": name})
}
