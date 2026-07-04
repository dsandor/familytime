# Final-review fix wave — report

Date: 2026-07-03
Scope: 4 fixes (1 important, 3 minor) across the Go server, the embedded web UI, and the static landing page. No git operations performed (not a git repo; git use forbidden per task).

---

## Fix 1 (Important) — pre-validate the grant before removing the pause

**File:** `internal/server/rules_handlers.go`, function `handlePauseDelay`.

**Defect:** For an indefinite ("Until I resume") pause, a grant whose `newStart` lands exactly in the 07:00 clock minute makes `until = nextMorning(newStart)` a same-clock-minute window 24h away. The collapse check (`!until.After(newStart.Add(time.Minute))`) doesn't fire in that case, so the code proceeded to call `removePauseRules` (deleting the live pause on the gateway and in the store) and only *then* attempted `createFamilyRule`, whose call to `rules.Translate` 400s with "the pause needs to end at a different time of day than it starts" — leaving the profile with no pause at all.

**Fix:** Restructured `handlePauseDelay` so the candidate `store.FamilyRule` is built and dry-run through `rules.Translate(fr, p)` (the same function/signature `createFamilyRule` uses) *before* `removePauseRules` is called. The dry-run only runs on the non-collapse path (collapse still returns `{"removed":true}` exactly as before, without ever needing a translate check). If the dry-run errors, the handler calls `fail(w, 400, err.Error())` and returns immediately — the existing pause is never touched. If the dry-run succeeds, execution proceeds through the existing remove → create flow, reusing the already-built `fr` (avoids rebuilding/duplicating the literal).

Net diff shape (in `handlePauseDelay`):
```go
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
```

### TDD evidence

Added `TestPauseGrantDegenerateWindowLeavesPauseIntact` to `internal/server/rules_handlers_test.go` (placed next to the other pause-grant tests, right before `TestPauseGrantPendingPushesPromisedStart`):

```go
func TestPauseGrantDegenerateWindowLeavesPauseIntact(t *testing.T) {
	ts, _, st, srv := newTestServer(t)
	c := doSetup(t, ts)
	seedProfile(t, st)
	fixedNow(t, srv, "2026-07-04 06:30")

	// Indefinite pause; a 30m grant lands exactly at 07:00, so the morning
	// re-engage window would start and end in the same clock minute.
	doJSON(t, c, "POST", ts.URL+"/api/pause", `{"profileId":"p1","duration":"indefinite"}`, nil)
	if code := doJSON(t, c, "POST", ts.URL+"/api/pause/p1/delay", `{"delay":"30m"}`, nil); code != 400 {
		t.Fatalf("degenerate grant = %d, want 400", code)
	}
	rs := st.Snapshot().Rules
	if len(rs) != 1 || rs[0].When.Kind != store.WhenAlways {
		t.Errorf("failed grant must leave the pause untouched: %+v", rs)
	}
}
```

**Red (before fix)** — `go test ./internal/server/ -run TestPauseGrantDegenerate -v`:
```
=== RUN   TestPauseGrantDegenerateWindowLeavesPauseIntact
    rules_handlers_test.go:499: failed grant must leave the pause untouched: []
--- FAIL: TestPauseGrantDegenerateWindowLeavesPauseIntact (0.07s)
FAIL
FAIL	familytime/internal/server	0.323s
FAIL
```
This confirms the exact defect: the request already returned 400 (the first assertion passed), but the pause rule list was empty (`[]`) — the pause had already been deleted before the translate failure.

**Green (after fix)** — `go test ./internal/server/ -run TestPauseGrant -v` (full pause-grant family, to check for regressions too):
```
=== RUN   TestPauseGrantKeepsOriginalEnd
--- PASS: TestPauseGrantKeepsOriginalEnd (0.07s)
=== RUN   TestPauseGrantOutlastingPauseRemovesIt
--- PASS: TestPauseGrantOutlastingPauseRemovesIt (0.05s)
=== RUN   TestPauseGrantIndefiniteEndsAtMorning
--- PASS: TestPauseGrantIndefiniteEndsAtMorning (0.05s)
=== RUN   TestPauseGrantDegenerateWindowLeavesPauseIntact
--- PASS: TestPauseGrantDegenerateWindowLeavesPauseIntact (0.05s)
=== RUN   TestPauseGrantPendingPushesPromisedStart
--- PASS: TestPauseGrantPendingPushesPromisedStart (0.05s)
=== RUN   TestPauseGrantValidation
--- PASS: TestPauseGrantValidation (0.05s)
PASS
ok  	familytime/internal/server	0.525s
```
Full package also passes: `go test ./internal/server/...` → `ok  familytime/internal/server  3.619s`.

---

## Fix 2 (Minor) — soften a landing foot-note

**File:** `landing/index.html`, `#sturdy` card (the "Built for iPhone privacy features" card).

Changed:
```
- <p class="foot-note">Private Wi-Fi Address on? Still works.</p>
+ <p class="foot-note">Apple's default Private Wi-Fi Address? Still works.</p>
```
This avoids overclaiming that *every* Private Wi-Fi Address configuration (including the non-default "Rotating" sub-option, which the README notes requires re-enrolling) is unconditionally supported.

---

## Fix 3 (Minor) — tooltip on the pending pause's "+30 min" button

**File:** `web/static/index.html`.

Confirmed via `web/static/css/app.css` (`.pausebox.scheduled { background: rgba(251, 191, 36, ...) }`) that the *pending* (amber) pausebox is the `<div class="pausebox scheduled">` block, whose button reads `+30 min` (the active/green pausebox's button reads `30 more min` and was left untouched, per instructions).

Added the `title` attribute, nothing else changed:
```html
<button class="more" @click="grantMore(p.id)" title="Give them 30 more minutes before the pause starts">+30 min</button>
```
Verified live in the browser (accessibility snapshot showed `button "+30 min" description="Give them 30 more minutes before the pause starts"`).

---

## Fix 4 (Minor) — pin preview-mock times to clean boundaries + recapture desktop shots

**File:** `web/static/js/app.js`, `mockPreview`.

Applied the specified change:
```js
- const now = Date.now();
+ const now = Math.floor(Date.now() / 1800e3) * 1800e3; // half-hour boundary: preview times (and marketing screenshots) stay clean
```

**One additional necessary correction (deviation, documented):** after applying only the line above, I loaded the live home preview and found the Teens pause showing "until 11:20 PM" — not a clean :00/:30 boundary. Root cause: rule `r4` ("Internet pause" for Teens) computed its `until` as `now + 20 * 60e3` (a fixed 20-minute offset), and 20 is not a multiple of 30, so even with `now` snapped to the half-hour, the derived time still lands on `:20`/`:50`. All other offsets in `mockPreview` (`±3600e3`, `±3*3600e3`, `9*3600e3`, `30*60e3`, `90*60e3`) are already multiples of 30 minutes, so `r4` was the sole outlier. Since the task's acceptance bar explicitly requires "all visible times on clean :00/:30 boundaries," I changed `r4`'s offset from 20 to 30 minutes (both occurrences: `when.until` and the outer `until` field) so the stated goal of Fix 4 — no ugly baked-in times in the screenshots — actually holds. This is the only code change beyond the literal instruction.

### Screenshot recapture

1. `go build ./...` (embeds the updated JS) — confirmed OK.
2. Ran `go run ./cmd/familytime -port 8080 -data <scratchpad>/ft-shots3.json` in the background; restarted it once after the `r4` offset correction so the embedded JS was current.
3. Used chrome-devtools MCP with `emulate` set to viewport `1280x1300x1` (device scale factor 1 — plain `resize_page` alone produced a 2x/retina-scaled 2560×1708 capture, corrected by pinning `deviceScaleFactor` via `emulate`).
4. Recaptured all three files, confirmed dimensions with `sips -g pixelWidth -g pixelHeight`.

**Per-file acceptance observations** (each viewed with the Read tool):

- **`landing/assets/home.png`** — 1280×1300, 345,962 bytes (~338 KB). Nav bar fully visible; full stat row (Groups/Rules/Pauses) visible; Kids card shows "Scheduled" badge with "Pauses at 11:30 PM · until Sat 12:30 AM"; Teens card shows "Active" badge, "Paused until 11:30 PM", and the "30 more min" button; "Active Right Now" list shows "until 11:30 PM" and "until Sat 8:00 AM". All visible times land on :00/:30. No card sliced at top or bottom edge (bottom card's rounded border is fully visible).
- **`landing/assets/rules.png`** — 1280×1300, 317,688 bytes (~310 KB). Nav bar visible, page header, "+ New rule" button, search box, and all 3 rule cards fully visible with rounded borders intact (page content ends well above the bottom edge, plenty of empty space below — not sliced). Times shown: "22:00–08:00", "00:00–02:00", "22:00–06:00" — all clean boundaries.
- **`landing/assets/groups.png`** — 1280×1300, 353,877 bytes (~346 KB). Nav bar, header, both group cards (Kids, Teens) and all 4 device rows fully visible, footnote text visible at the bottom, no slicing. No time-based content on this page (N/A for the clean-boundary check).

All three files are well under the ~400 KB budget and match 1280×1300 exactly.

5. Server stopped after capture; confirmed with `lsof -i :8080` that no `familytime` process remains listening (only unrelated Chrome helper client sockets in `CLOSE_WAIT`, which are not listeners and don't hold the port).

---

## Final verification

From `/Users/dsandor/Projects/bedtime`:

```
$ gofmt -l cmd internal
(no output — silent, i.e. clean)

$ go build ./...
(no output — success)

$ go test ./... -count=1
ok  	familytime/cmd/familytime	0.217s
ok  	familytime/internal/e2e	0.511s
ok  	familytime/internal/rules	0.378s
ok  	familytime/internal/server	4.133s
ok  	familytime/internal/store	0.880s
ok  	familytime/internal/unifi	1.019s
?   	familytime/web	[no test files]
```

All packages pass. gofmt silent. Build clean.

---

## Files touched

- `internal/server/rules_handlers.go` — Fix 1 (handlePauseDelay restructure)
- `internal/server/rules_handlers_test.go` — Fix 1 (new regression test)
- `landing/index.html` — Fix 2 (foot-note text)
- `web/static/index.html` — Fix 3 (title attribute)
- `web/static/js/app.js` — Fix 4 (`now` snapping + `r4` offset correction)
- `landing/assets/home.png`, `landing/assets/rules.png`, `landing/assets/groups.png` — Fix 4 (recaptured; enroll PNGs untouched)

No git operations were performed. No deployment was performed.
