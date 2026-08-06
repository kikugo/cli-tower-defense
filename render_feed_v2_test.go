package main

import (
	"strconv"
	"strings"
	"testing"

	eng "tower-defense/engine"
)

// --- shared fixtures ------------------------------------------------------

// buildDemoEventsV2 is one representative replay stream carrying all five
// treatments the task brief asks the report to demonstrate: a rejection
// run (worth collapsing), a breach, a provider error, an engine assist, and
// a wave boundary -- plus a handful of ordinary applied actions so the feed
// isn't ONLY special cases. Used by the fit sweep (needs a realistic mix at
// every size) and by TestRenderFeedV2Demo (prints the three report frames).
func buildDemoEventsV2() []eng.ReplayEvent {
	var evs []eng.ReplayEvent
	add := func(e eng.ReplayEvent) { evs = append(evs, e) }
	outcome := func(tick int64, player, role, action, quality string) eng.ReplayEvent {
		return eng.ReplayEvent{
			Tick: tick, PlayerID: player, Role: role, Action: action,
			Type: eng.ReplayOutcome, Reason: "applied_primary",
			Details: map[string]interface{}{"quality": quality},
		}
	}

	// A few ordinary applied turns, oldest first.
	add(eng.ReplayEvent{Tick: 5, PlayerID: "p1", Role: "defender", Action: "place",
		Type: eng.ReplayPlacement, Position: &eng.Position{Y: 5, X: 10},
		Details: map[string]interface{}{"tower_type": "basic", "cost": 100}})
	add(outcome(5, "p1", "defender", "place", "primary"))

	add(eng.ReplayEvent{Tick: 6, PlayerID: "p2", Role: "attacker", Action: "spawn",
		Type: eng.ReplaySpawn, Position: &eng.Position{Y: 0, X: 0},
		Details: map[string]interface{}{"enemy_type": "grunt", "cost": 10}})
	add(outcome(6, "p2", "attacker", "spawn", "primary"))

	add(outcome(7, "p1", "defender", "save", "primary"))
	add(outcome(8, "p2", "attacker", "save", "primary"))

	// A run of 40 identical rejections -- the measured defect this whole
	// pane exists to fix. Requirement 1.
	for i := 0; i < 40; i++ {
		add(eng.ReplayEvent{
			Tick: int64(20 + i), PlayerID: "p1", Role: "defender", Action: "place",
			Type: eng.ReplayOutcome, Reason: "rejected:unaffordable_placement",
			Details: map[string]interface{}{"quality": "rejected"},
		})
	}

	// A wave boundary -- requirement 4. Wave 1 -> wave 2.
	add(eng.ReplayEvent{Tick: 60, PlayerID: "p2", Role: "attacker", Action: "wave",
		Type: eng.ReplayWave, Amount: 10,
		Details: map[string]interface{}{"cost": 50, "wave": 2, "queue": 10}})
	add(outcome(60, "p2", "attacker", "wave", "primary"))

	// A breach -- requirement 2 (marker "!!"), highest non-gameend priority.
	add(eng.ReplayEvent{Tick: 65, PlayerID: "p2", Role: "attacker", Action: "breach",
		Type: eng.ReplayBreach, Position: &eng.Position{Y: 8, X: 79},
		Details: map[string]interface{}{"enemy_type": "o", "defender_lives": 6}})

	// A provider error -- requirement 2 (marker "!").
	add(eng.ReplayEvent{Tick: 70, PlayerID: "p2", Role: "attacker",
		Type: eng.ReplayProviderErr, Reason: "timeout"})

	// A substituted decision -- requirement 2 (marker "~").
	add(eng.ReplayEvent{
		Tick: 72, PlayerID: "p2", Role: "attacker", Action: "save",
		Type: eng.ReplayOutcome, Reason: "substituted:provider_failure:applied_primary",
		Details: map[string]interface{}{"quality": "substituted"},
	})

	// An engine assist -- requirement 3, the third actor.
	add(eng.ReplayEvent{
		Tick: 75, PlayerID: "p2", Role: "attacker", Action: "assist_queue_enemy",
		Type: eng.ReplayEngineAssist, Reason: "queue_enemy",
		Details: map[string]interface{}{"branch": "queue_enemy", "enemy_type": "basic", "queue": 5},
	})

	// Second wave boundary, closing wave 2.
	add(eng.ReplayEvent{Tick: 80, PlayerID: "p2", Role: "attacker", Action: "wave",
		Type: eng.ReplayWave, Amount: 12,
		Details: map[string]interface{}{"cost": 60, "wave": 3, "queue": 8}})
	add(outcome(80, "p2", "attacker", "wave", "primary"))

	add(outcome(85, "p1", "defender", "research", "primary"))
	add(outcome(86, "p1", "defender", "upgrade", "fallback"))

	return evs
}

// --- 1. fit at every size --------------------------------------------------

// TestRenderFeedV2FitsEverySize sweeps every width/height computeLayoutV2
// recognizes (sweepWidthsV2/sweepHeightsV2, defined in
// main_layout_v2_test.go) and asserts the feed pane renders EXACTLY
// layout.feed.h rows of EXACTLY layout.feed.w columns, via checkFits
// (mockup_fit_test.go) -- reused, not reimplemented, per the task brief.
//
// layout.feed.h can legitimately be 0 at the low end of a valid terminal
// size (see main_layout_v2.go's height-allocation doc comment); checkFits
// itself cannot represent "0 rows" (strings.Split("", "\n") is
// length-1, not length-0), so that case is asserted directly instead.
func TestRenderFeedV2FitsEverySize(t *testing.T) {
	events := buildDemoEventsV2()
	for _, w := range sweepWidthsV2() {
		for _, h := range sweepHeightsV2() {
			layout := computeLayoutV2(w, h)
			rows := RenderFeedV2(events, layout.feed.w, layout.feed.h)

			if layout.feed.h == 0 {
				if len(rows) != 0 {
					t.Fatalf("w=%d h=%d: feed.h is 0 but RenderFeedV2 returned %d rows", w, h, len(rows))
				}
				continue
			}

			frame := strings.Join(rows, "\n")
			if err := checkFits(frame, layout.feed.w, layout.feed.h); err != nil {
				t.Fatalf("w=%d h=%d (mode %v): %v", w, h, layout.mode, err)
			}
		}
	}
}

// --- 2. collapse -----------------------------------------------------------

// TestRenderFeedV2CollapsesIdenticalRuns asserts that 40 identical
// rejections collapse into exactly one row, with the correct count --
// requirement 1, and the specific measured defect (a real captured frame
// where ~90% of visible rows were one repeated rejection) this whole pane
// redesign responds to.
func TestRenderFeedV2CollapsesIdenticalRuns(t *testing.T) {
	events := buildDemoEventsV2() // contains a run of 40 identical rejections
	rows := buildFeedRowsV2(events)
	collapsed := collapseFeedRows(rows)

	var found *feedRow
	rejectionRuns := 0
	for i := range collapsed {
		if collapsed[i].kind == feedRejected {
			rejectionRuns++
			found = &collapsed[i]
		}
	}
	if rejectionRuns != 1 {
		t.Fatalf("want exactly 1 collapsed rejection row, got %d", rejectionRuns)
	}
	if found.count != 40 {
		t.Fatalf("collapsed rejection row count = %d, want 40", found.count)
	}

	// And the rendered text actually shows the count, per requirement 1
	// ("collapsed rows render like x9 at the right").
	rendered := renderFeedRowText(*found, 100)
	if !strings.Contains(rendered, "x40") {
		t.Fatalf("rendered collapsed row %q does not contain the count suffix x40", rendered)
	}
}

// --- 3. priority -------------------------------------------------------

// TestSelectFeedRowsV2PriorityOverChronology is the direct, white-box test
// of the priority ordering itself: given a breach, a provider error, and
// ten distinct routine "save" rows, and a budget of only 3, the breach and
// the provider error must both survive even though most of the saves are
// more recent than the breach. This exercises selectFeedRowsV2 directly
// (rather than the full RenderFeedV2 pipeline) so collapsing can't
// interfere with the result -- ten hand-built rows are given DIFFERENT
// targets specifically so they are never candidates for collapseFeedRows
// to merge, isolating the property under test to priority selection.
func TestSelectFeedRowsV2PriorityOverChronology(t *testing.T) {
	var rows []feedRow
	idx := 0
	next := func(r feedRow) feedRow {
		r.origIndex = idx
		idx++
		if r.count == 0 {
			r.count = 1
		}
		return r
	}

	breach := next(feedRow{tick: 10, kind: feedBreach, side: "ATT", action: "BREACH", marker: "!!"})
	rows = append(rows, breach)

	for i := 0; i < 4; i++ {
		rows = append(rows, next(feedRow{tick: int64(11 + i), kind: feedApplied, side: "DEF", action: "save", target: strconv.Itoa(i)}))
	}

	provErr := next(feedRow{tick: 20, kind: feedProviderError, side: "ATT", action: "ERROR", marker: "!"})
	rows = append(rows, provErr)

	for i := 4; i < 10; i++ {
		rows = append(rows, next(feedRow{tick: int64(21 + i), kind: feedApplied, side: "DEF", action: "save", target: strconv.Itoa(i)}))
	}

	firstSave := rows[1] // the oldest, least-recent save -- must be dropped first

	got := selectFeedRowsV2(rows, 3)
	if len(got) != 3 {
		t.Fatalf("selectFeedRowsV2 returned %d rows, want 3", len(got))
	}

	var haveBreach, haveErr, haveFirstSave bool
	for _, r := range got {
		if r.origIndex == breach.origIndex {
			haveBreach = true
		}
		if r.origIndex == provErr.origIndex {
			haveErr = true
		}
		if r.origIndex == firstSave.origIndex {
			haveFirstSave = true
		}
	}
	if !haveBreach {
		t.Errorf("breach did not survive a 3-row budget")
	}
	if !haveErr {
		t.Errorf("provider error did not survive a 3-row budget")
	}
	if haveFirstSave {
		t.Errorf("the oldest routine save survived over rows with higher priority")
	}

	// Chronology within the kept set must still hold (requirement 6:
	// newest at the bottom / display order tracks event order).
	for i := 1; i < len(got); i++ {
		if got[i].origIndex <= got[i-1].origIndex {
			t.Errorf("selected rows are not in chronological order: %+v", got)
		}
	}
}

// TestRenderFeedV2PriorityEndToEnd is the same property exercised through
// the full RenderFeedV2 pipeline (build -> collapse -> select -> render),
// with a realistic mixed stream and a 3-row pane -- matching the task
// brief's test description verbatim ("3 rows and a stream containing a
// breach, a provider error and ten saves, the breach and error survive").
func TestRenderFeedV2PriorityEndToEnd(t *testing.T) {
	var events []eng.ReplayEvent
	for i := 0; i < 10; i++ {
		events = append(events, eng.ReplayEvent{
			Tick: int64(i), PlayerID: "p1", Role: "defender", Action: "save",
			Type: eng.ReplayOutcome, Reason: "applied_primary",
			Details: map[string]interface{}{"quality": "primary"},
		})
	}
	events = append(events, eng.ReplayEvent{
		Tick: 20, PlayerID: "p2", Role: "attacker", Action: "breach",
		Type: eng.ReplayBreach, Position: &eng.Position{Y: 1, X: 2},
		Details: map[string]interface{}{"enemy_type": "o", "defender_lives": 4},
	})
	events = append(events, eng.ReplayEvent{
		Tick: 21, PlayerID: "p2", Role: "attacker",
		Type: eng.ReplayProviderErr, Reason: "timeout",
	})

	rows := RenderFeedV2(events, 100, 3)
	if len(rows) != 3 {
		t.Fatalf("RenderFeedV2 returned %d rows, want 3", len(rows))
	}
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "!!") {
		t.Errorf("rendered 3-row feed does not show the breach marker (!!): %q", joined)
	}
	if !strings.Contains(joined, "ERROR") {
		t.Errorf("rendered 3-row feed does not show the provider error row: %q", joined)
	}
}

// --- 4. engine rows -------------------------------------------------------

// TestRenderFeedV2EngineIsThirdActor asserts a ReplayEngineAssist event
// renders with the ">>>"/"ENGINE" treatment (requirement 3) and is not
// attributable to either player -- i.e. its side is neither "DEF" nor
// "ATT", even though the underlying event's Role field is "attacker" (see
// engine/assist.go's recordEngineAssist doc comment: applyAdaptivePressure
// only ever acts on the attacker's behalf, but that is not the same as the
// attacker's MODEL having chosen it).
func TestRenderFeedV2EngineIsThirdActor(t *testing.T) {
	events := []eng.ReplayEvent{{
		Tick: 42, PlayerID: "p2", Role: "attacker", Action: "assist_auto_wave",
		Type: eng.ReplayEngineAssist, Reason: "auto_wave",
		Details: map[string]interface{}{"branch": "auto_wave", "wave": 7},
	}}

	rows := buildFeedRowsV2(events)
	if len(rows) != 1 {
		t.Fatalf("want 1 row for a single engine assist event, got %d", len(rows))
	}
	r := rows[0]
	if r.side != ">>>" {
		t.Errorf("engine row side = %q, want \">>>\"", r.side)
	}
	if r.action != "ENGINE" {
		t.Errorf("engine row action = %q, want \"ENGINE\"", r.action)
	}
	if r.side == "DEF" || r.side == "ATT" {
		t.Errorf("engine row must not be attributed to a player, got side %q", r.side)
	}

	rendered := renderFeedRowText(r, 100)
	if !strings.Contains(rendered, ">>>") || !strings.Contains(rendered, "ENGINE") {
		t.Errorf("rendered engine row missing >>>/ENGINE treatment: %q", rendered)
	}

	// Missing/empty branch renders "branch not recorded" verbatim, per the
	// task brief -- not reachable from current engine code (recordEngineAssist
	// always sets Details["branch"]), but exercised here as documented
	// defensive behavior.
	noBranch := []eng.ReplayEvent{{
		Tick: 43, PlayerID: "p2", Role: "attacker",
		Type: eng.ReplayEngineAssist, Details: map[string]interface{}{},
	}}
	rows2 := buildFeedRowsV2(noBranch)
	if len(rows2) != 1 || !strings.Contains(rows2[0].detail, "branch not recorded") {
		t.Fatalf("engine row with no branch detail = %+v, want detail containing \"branch not recorded\"", rows2)
	}
}

// --- 5. no wrapping ever ----------------------------------------------

// TestRenderFeedV2NeverWraps feeds a 500-character reason string through a
// provider-error row and asserts the result is still exactly `height` rows
// of exactly `width` display columns each -- requirement 5, "truncate,
// never wrap": a row that would need to wrap must instead be cut off.
func TestRenderFeedV2NeverWraps(t *testing.T) {
	longReason := strings.Repeat("x", 500)
	events := []eng.ReplayEvent{{
		Tick: 1, PlayerID: "p2", Role: "attacker",
		Type: eng.ReplayProviderErr, Reason: longReason,
	}}

	const width, height = 100, 5
	rows := RenderFeedV2(events, width, height)
	if len(rows) != height {
		t.Fatalf("got %d rows, want %d", len(rows), height)
	}
	for i, row := range rows {
		if strings.Contains(row, "\n") {
			t.Fatalf("row %d contains an embedded newline (wrapped): %q", i, row)
		}
		if w := frameDisplayWidth(row); w != width {
			t.Fatalf("row %d is %d display columns wide, want %d: %q", i, w, width, row)
		}
	}
}

// --- demo: the three report frames -----------------------------------

// TestRenderFeedV2Demo prints the feed at the three sizes the task brief's
// report asks for (160/25, 100/10, 80/6), from one stream that touches all
// five treatments. Run with `-v` to capture the output for the report; it
// also sanity-checks each frame against checkFits so a broken demo can't
// silently ship.
func TestRenderFeedV2Demo(t *testing.T) {
	events := buildDemoEventsV2()
	sizes := []struct{ w, h int }{{160, 25}, {100, 10}, {80, 6}}
	for _, sz := range sizes {
		rows := RenderFeedV2(events, sz.w, sz.h)
		frame := strings.Join(rows, "\n")
		if err := checkFits(frame, sz.w, sz.h); err != nil {
			t.Errorf("demo frame %dx%d: %v", sz.w, sz.h, err)
		}
		t.Logf("=== %dx%d ===\n%s", sz.w, sz.h, frame)
	}
}
