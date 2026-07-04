# Bedtime SDD progress ledger
# Plan: docs/superpowers/plans/2026-07-02-bedtime.md (11 tasks + final review)
# No git in this project — "commits" columns replaced by verification evidence.
Task 1: complete (store package; review found go.mod 1.25→fixed to 1.24; re-verified build+vet+test green)
Task 2: complete (unifi client; review clean, approved; fresh test re-run 8/8)
  Minor (plan-mandated, for final review): ListClients pagination lacks hard iteration cap (silent truncation if server omits count); empty-array marshal test skips ip_ranges/regions/repeat_on_days
  ⚠️ fixture-vs-live faithfulness → covered by Task 11 live E2E
Task 3: review found Important one-time overnight date-anchor bug (plan-deferred ambiguity); live probe confirmed gateway accepts overnight ONE_TIME_ONLY; fix dispatched (anchor date to start day + new test)
  Minor (for final review): ActiveNow/Expired knife-edge at now==until; NormalizeDomain accepts syntactically-valid junk TLDs; raw HH:MM stored unformatted ("9:00" accepted); no Start==End degenerate guard; FamilyRuleID leading-space edge if empty id
Task 3: complete (translator; overnight one-time date-anchor fixed to start day, 13/13 tests, re-review approved)
Task 4: approved (server core; 8/8 tests, security ordering verified by reviewer). Fixer dispatched: move 4 handlers server.go→auth.go per plan layout (Task 8 briefs edit them in auth.go).
  FOR USER DECISION at end (plan-mandated, reviewer Important): login guard is global/unkeyed — one LAN actor failing PINs 5x locks login for everyone up to 15min. Deliberate tradeoff (global backoff = stronger brute-force protection for 4-6 digit PIN); alternative is per-IP keying.
  Minor (plan-mandated): newSecret ignores rand.Read error (Go 1.24: rand.Read cannot fail); readJSON MaxBytesReader nil writer skips connection-close hardening
Task 4: complete (handlers relocated to auth.go, 8/8 tests green)
Task 5: complete (profiles/devices/presets API; approved; 14/14 tests)
  Minor (plan-mandated, for final review): handleDevices sort.Slice unstable ties (use SliceStable/MAC tiebreak); handleProfileUpdate writes store even on 404 no-op; assigned-MAC map trusts stored normalization at read time
Task 6: approved (rules/pause/status/settings API; 28/28 tests; transactional paths hand-traced by reviewer). Hardening fixer dispatched: gofmt, stale-id cleanup in update-recreate branch, logged compensation failures.
  Minor (for final review): no test for morning-pause before 07:00 branch (logic hand-verified correct); settings gateway/trust-cert handlers untested; snapshot-then-Update race pattern (janitor two-pass reconciliation is designed backstop)
Task 6: complete (hardening verified: gofmt clean, stale-id cleanup, logged compensation; 28/28)
Task 7: complete (janitor+embed+main; approved clean; reviewer re-ran smoke test; backend done)
Task 8: complete (UI foundation; approved; 54/54 tests; browser-verified setup wizard + PIN pad, zero console errors; WCAG contrast fixes independently re-derived by reviewer)
  Minor (deferred to Tasks 9-10 by design): base button style 4.01:1 contrast (dormant, no view uses it yet)
Task 9: complete (home+family UI; approved; contract byte-identical; contrast math independently verified; previews browser-checked)
  Folded into Task 10 dispatch: add 'rules' to authedView() (plan gap — tabbar hidden on rules view); ghost-button hover contrast 4.36/4.45 just under AA; .chips button.on:hover loses selected state
Task 10: complete (rules wizard + settings UI; approved after toggle off-state contrast fix: knob/track 4.38:1, track/card 4.07:1, new --track-off token; carry-over fixes verified 4.55/4.58/6.20)
  Minor (pre-existing app-wide, for final review): implicit-label DevTools advisory on form fields (accessible names exist via label wrapping)
Task 11: complete (live E2E PASS; UI walk-through verified against real gateway incl. start-day pause anchoring; gateway left clean — only "kids apps"; 4 cross-compiles OK; README written)
  Deferred to user: category-label visual check in UniFi UI; real-device enforcement spot-check
Final review (fable): sound architecture; 1 Critical cross-task seam (C1 profile-edit doesn't re-target gateway rules) + I1 PIN-change session rotation, I2 env-key exfil window pre-setup, I3 failErr swallows diagnostics, I4 device picker not searchable (spec gap, 82 clients). Fix wave dispatched (first attempt died on session limit, re-dispatched).
  Accepted as-is: pagination cap, knife-edge until, junk TLDs, 404-write, janitor ctx, a11y advisory, M1/M2/M3/M7
  USER DECISION pending: global login guard tradeoff (see Task 4 entry)
Final fix wave: 13/13 applied and verified by re-review; re-review found 1 new Major (multi-rule partial failure in profile update) → fixed with persist-intent-on-partial-failure + convergence test. Controller-verified: build/vet/test green, gofmt clean.
PROJECT COMPLETE. Remaining for user: (1) login-guard tradeoff decision; (2) category label visual check in UniFi UI; (3) real-device enforcement spot-check; (4) run ./bedtime and do real setup with own PIN.
Rebrand+redesign: complete. Bedtime→Family Time repo-wide (module familytime, [family-time] prefixes, cookie, data dir, env gates); UI restructured per Lovable mockup (top nav, dashboard w/ stat cards + quick pause 15m/30m/1h, flat searchable Rules tab, Groups & Devices with per-device dropdowns, wizard Who step); dark sci-fi glassmorphism theme, contrast re-verified. Review fixes: reassignDevice rollback on partial failure; contrast table corrected; blocking indicator styled. 68+ tests green.
Investigation (iPhone missing from devices): NOT an app bug. Gateway reports 80 clients, app fetches all 80. Hardware MAC 28:d5:b1:cf:1c:ee absent from gateway data entirely (no OUI matches); 7 clients use locally-administered randomized MACs incl. 3 generic-named iPhones → iOS Private Wi-Fi Address. Fix = self-enrollment feature (identify by requesting IP), dispatched.
Enrollment feature: complete + reviewed (Approved). /enroll page (no auth, by owner decision): server resolves visiting device by RemoteAddr IP against gateway client list — immune to MAC randomization. Refactor: retargetProfileRules extracted, verified behavior-preserving. Reviewer probe-tested zero-devices guard + idempotent rename; both now pinned by regression tests (15 enroll tests total). Settings shows enrollUrl; devices page hints about Private Wi-Fi Address; README documents Rotate-Wi-Fi-Address caveat.
Group editor device management: complete (frontend-only; member list w/ remove, staged add-search, other-group devices disabled w/ guidance, groups-list member summaries; implementer caught Alpine :disabled truthiness bug via DOM inspection; controller visually verified)
UniFi alias sync: complete. RenameClient via legacy /api/s/default/rest/user (API-key auth verified live; set/clear probe clean). Push points: enroll (always, final name) + profile-update (changed names only), best-effort with logging, never fatal. Group editor gained inline ✎ device rename (staged). 8 new tests; suite green; browser-verified.
--- Delayed Quick Pause plan (docs/superpowers/plans/2026-07-03-delayed-quick-pause.md, 3 tasks + final review; qp- prefix) ---
QP Task 1: complete (OneTimeStart + start-gated ActiveNow; 21/21 rules tests; review clean, approved)
  Minor (for final review): midnight-crossing anchor calc duplicated between translateSchedule and OneTimeStart (shared helper possible)
QP Task 2: complete (delay on POST /api/pause + startsAt in rules list; 59/59 server tests; review clean, approved; handleStatus untouched as planned)
  Minor (plan-mandated, for final review): delay/duration switches duplicate the 15m/30m/1h string→Duration mapping
  Minor (for final review): delayed-morning test's until assertion doesn't discriminate anchor-to-start vs anchor-to-now (Start assertion does)
QP Task 3: complete (Starting selector, Scheduled badge/pausebox, upcoming-list pending support, preview mock r9; gofmt/build/full suite green; browser-verified via chrome-devtools, zero console errors; review approved)
  Minor (for final review): pending-pausebox x-text triple-calls groupPending guarded only by sibling x-if (mirrors pre-existing groupPause pattern); Cancel click itself not exercisable in preview mode
  Minor (for final review): preview mock r9 coexists with active r4 — impossible with real data (pauses replace) — suggest one-line comment
  Minor (note): upcomingTodayList pending branch relies on invariant that only pause rules carry startsAt
QP Final review (fable): READY TO SHIP — no Critical/Important; all 6 carried Minors triaged accept; notable accepted edges: translate-stage 400 after removePauseRules (UI-unreachable, pre-existing class), DST fall-back 1h pending-display skew (cosmetic, 2 nights/yr). Polish wave applied+verified: discriminating 06:50+30m morning-anchor test, 15m-delay coverage, preview mock r9 → kids + comment. Controller-verified: gofmt clean, build ok, full suite green.
QP FEATURE COMPLETE (not committed — user handles VCS). Spec follow-up idea for user: UI affordance to grant "30 more minutes" while a pause is currently ACTIVE (API supports it; duration chips are hidden during an active pause).
--- Give More Minutes + Landing Screens plans (2026-07-03; gm-/ls- prefixes) ---
GM Task 1: complete (POST /api/pause/{profileId}/delay + nextMorning extraction; 56/56 server tests; review approved)
  Important (plan-mandated, pre-existing class, for final review + user summary): remove-then-create without rollback — gateway failure during grant leaves group unpaused (same shape as handlePause; janitor is designed backstop)
  Minor (for final review): pending/indefinite grant tests don't assert gateway state; error-body text untested (status codes only); pause FamilyRule literal duplicated handlePause↔handlePauseDelay; no repeated-grant test; defensive dead default branch
GM Task 2: complete (grantMore + "30 more min"/"+30 min" buttons + CSS; gofmt/build/suite green; browser-verified; review found Important narrow-width clipping (<350px, plan gap) → fixed with .pausebox flex-wrap:wrap, live re-verified 320px+1280px, re-review Resolved)
  Minor (for final review): redundant border decl on .pausebox button.more (base button rule sets identical value)
LS Task 1: complete (5 PNGs in landing/assets; visual review found home.png stat-row top slice (Critical) + groups.png truncated last row (Important) → recaptured all 3 desktop shots at uniform 1280x1300, re-review Resolved; enroll shots 780x1688 @2x approved first pass; all ≤360KB)
LS Task 2: complete (#enroll + #peek sections, sturdy card re-angled, CSS appended; implementer caught plan bug — .step-n class collision with #how, renamed enroll-step-n, folded into plan; reviewer independently byte-diffed copy + live-browser-verified both breakpoints; approved, no Important findings)
  Minor (plan-mandated, for final review): new 880px breakpoint vs stylesheet's existing 860px; gallery shots 1x vs enroll 2x DPR mix
GM+LS Final review (fable): READY TO SHIP. 1 Important found beyond per-task gates — deterministic 06:30 indefinite-grant edge (pause removed then 400) → fixed via Translate dry-run before removePauseRules + regression test. Also fixed: Private Wi-Fi foot-note softened to "Apple's default"; +30 min tooltip added; mockPreview clock pinned to :00/:30 (+r4 offset 20m→30m) + 3 desktop shots recaptured with clean times. Re-review: all 4 fixes Resolved, no new issues. Controller-verified: gofmt clean, build ok, full suite green, 5 assets ≤358KB.
Carried-finding accepted (user visibility): handlePauseDelay remove-then-create without rollback on gateway failure — matches pre-existing handlePause shape, janitor is designed backstop; Translate dry-run now eliminates the only deterministic instance. Others accepted: grant-test gateway asserts/error-body strings, pause-literal duplication, dead-looking default branch (actually legit defense), redundant border decl, 880/860 breakpoint mix, DPR mix.
GM + LS FEATURES COMPLETE (not committed — user handles VCS).
