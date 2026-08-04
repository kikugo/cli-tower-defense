package main

// These tests cover the viewer-side half of the replay-truncation fix. The
// engine side (engine/replay.go, engine/replay_snapshot.go) already plants a
// ReplayTruncated marker event in the stream and sets
// ReplaySnapshot.Truncated/TruncatedEvents when ReconstructSnapshot walks
// past it -- see engine/replay_truncation_test.go for that half. What was
// still missing: replayView (main.go) called ReconstructSnapshot and threw
// the Truncated signal away, so a user opening a long replay still saw a
// board quietly missing towers with nothing on screen saying so. The tests
// below build small hand-written event streams (a map_init event, optionally
// a ReplayTruncated marker, then a placement/spawn) rather than driving a
// real scripted match, since the mechanism under test is entirely in
// replayView's row budgeting and text, not in how the marker gets planted --
// engine/replay_truncation_test.go and TestReconstructSnapshotFlagsTruncationOnRealMatch
// already cover that a real trimmed match produces exactly this shape.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	eng "tower-defense/engine"

	"github.com/charmbracelet/lipgloss"
)

// mapInitDetails builds a minimal but valid map_init Details payload --
// applyMapInit (engine/replay_snapshot.go) requires map_height/map_width > 0
// and a paths array before it will set snap.HasMap, which renderReplayBoard
// requires before it draws anything.
func mapInitDetails() map[string]interface{} {
	return map[string]interface{}{
		"map_height": 6,
		"map_width":  12,
		"paths":      [][][]int{{{0, 0}, {0, 1}, {0, 2}, {0, 3}}},
		"obstacles":  [][]int{{3, 5}},
	}
}

// truncatedReplayStream builds a hand-written event stream that mirrors what
// engine/replay.go's trimReplayEvents actually produces once MaxReplayEvents
// forces a trim: map_init first, then a ReplayTruncated marker carrying
// discarded_events, then ordinary events (a placement and a spawn) that
// happened after the trim. discardedEvents is the count trimReplayEvents
// would have folded into the marker's Details["discarded_events"].
func truncatedReplayStream(discardedEvents int) []eng.ReplayEvent {
	pos := eng.Position{Y: 0, X: 1}
	return []eng.ReplayEvent{
		{Type: eng.ReplayMapInit, Details: mapInitDetails()},
		{Type: eng.ReplayTruncated, Tick: 50, Details: map[string]interface{}{
			"discarded_events": discardedEvents,
		}},
		{Type: eng.ReplayPlacement, Tick: 51, PlayerID: "def", Position: &pos,
			Details: map[string]interface{}{"tower_type": "basic"}},
		{Type: eng.ReplaySpawn, Tick: 52, PlayerID: "att",
			Details: map[string]interface{}{"enemy_type": "fast"}},
	}
}

// completeReplayStream is the same shape MINUS the ReplayTruncated marker --
// a stream that was never trimmed, so ReconstructSnapshot must report
// Truncated=false for every index into it.
func completeReplayStream() []eng.ReplayEvent {
	pos := eng.Position{Y: 0, X: 1}
	return []eng.ReplayEvent{
		{Type: eng.ReplayMapInit, Details: mapInitDetails()},
		{Type: eng.ReplayPlacement, Tick: 1, PlayerID: "def", Position: &pos,
			Details: map[string]interface{}{"tower_type": "basic"}},
		{Type: eng.ReplaySpawn, Tick: 2, PlayerID: "att",
			Details: map[string]interface{}{"enemy_type": "fast"}},
	}
}

// replayTruncationSizes is the same terminal-size matrix
// TestGameOverAndReplayFitInvariant and TestViewNeverExceedsTerminal use,
// covering every layout mode (compact/stacked/wide) and the {0,0}
// pre-WindowSizeMsg case.
var replayTruncationSizes = []struct{ w, h int }{
	{0, 0}, {60, 15}, {80, 24}, {84, 24}, {100, 30},
	{119, 40}, {120, 40}, {160, 50}, {204, 60},
}

// TestReplayViewShowsTruncationWarning is the positive case: a replay view
// built from a stream containing a ReplayTruncated marker must render a
// warning that (a) is visible in the plain rendered text (not just present
// in some ANSI attribute nobody reads) and (b) states the consequence -- how
// many events were discarded and that towers/enemies from before then are
// missing -- not just a bare "truncated" flag.
func TestReplayViewShowsTruncationWarning(t *testing.T) {
	events := truncatedReplayStream(858)
	m := model{replayMode: true, replay: events, replayIdx: len(events) - 1, tickDur: 100 * time.Millisecond, width: 120, height: 40}

	out := m.View()

	if !strings.Contains(out, "TRUNCATED") {
		t.Fatalf("replay view for a truncated stream does not contain a %q warning at all:\n%s", "TRUNCATED", out)
	}
	if !strings.Contains(out, "858") {
		t.Errorf("replay view warning does not report the discarded event count (858):\n%s", out)
	}
	if !strings.Contains(out, "missing") || !strings.Contains(out, "board") {
		t.Errorf("replay view warning states the fact but not the consequence (expected wording about what is missing from the board):\n%s", out)
	}

	// The warning must sit on its own row directly above the board, not
	// buried inside the JSON event-details dump: find its row index and
	// confirm it comes before any line that looks like the details pane
	// ("Event details:" is the literal heading replayView writes above the
	// JSON dump).
	lines := strings.Split(out, "\n")
	warnRow, detailsHeadingRow := -1, -1
	for i, line := range lines {
		if strings.Contains(line, "TRUNCATED") && warnRow == -1 {
			warnRow = i
		}
		if strings.Contains(line, "Event details:") && detailsHeadingRow == -1 {
			detailsHeadingRow = i
		}
	}
	if warnRow == -1 {
		t.Fatalf("could not locate the warning row in the rendered output")
	}
	if warnRow > 1 {
		t.Errorf("warning row is at line %d; expected it near the top (row 0 or 1, directly under/as the status line), not buried further down:\n%s", warnRow, out)
	}
	if detailsHeadingRow != -1 && warnRow > detailsHeadingRow {
		t.Errorf("warning row (%d) comes AFTER the 'Event details:' heading (%d) -- it is buried in the details pane instead of being adjacent to the board", warnRow, detailsHeadingRow)
	}
	t.Logf("warning row index=%d, rendered output:\n%s", warnRow, out)
}

// TestReplayViewNoWarningWhenComplete is the negative case: a replay view
// built from a stream that was never truncated must not show the warning at
// any index -- Truncated is precise (false for any window entirely before a
// discard gap, and there is no gap here at all), and replayView must not
// replace that precision with a blanket per-file flag.
func TestReplayViewNoWarningWhenComplete(t *testing.T) {
	events := completeReplayStream()
	for idx := 0; idx < len(events); idx++ {
		idx := idx
		t.Run(fmt.Sprintf("idx_%d", idx), func(t *testing.T) {
			m := model{replayMode: true, replay: events, replayIdx: idx, tickDur: 100 * time.Millisecond, width: 120, height: 40}
			out := m.View()
			if strings.Contains(out, "TRUNCATED") {
				t.Errorf("replay view for a complete stream at index %d shows a truncation warning it should not:\n%s", idx, out)
			}
		})
	}
}

// TestReplayViewTruncationFitInvariant applies the same fit invariant
// TestViewNeverExceedsTerminal and TestGameOverAndReplayFitInvariant check
// elsewhere in this package to the truncated-replay case specifically: the
// warning row must come OUT of an existing pane's budget, not get added on
// top of it, at every layout mode in the matrix.
func TestReplayViewTruncationFitInvariant(t *testing.T) {
	events := truncatedReplayStream(42)
	idx := len(events) - 1

	for _, sz := range replayTruncationSizes {
		sz := sz
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			m := model{replayMode: true, replay: events, replayIdx: idx, tickDur: 100 * time.Millisecond, width: sz.w, height: sz.h}
			out := m.View()
			gotH, gotW := lipgloss.Height(out), lipgloss.Width(out)
			wantW, wantH := fitTarget(sz.w, sz.h)
			if gotH > wantH {
				t.Errorf("rendered height %d exceeds terminal height %d (truncated replay)", gotH, wantH)
			}
			if gotW > wantW {
				t.Errorf("rendered width %d exceeds terminal width %d (truncated replay)", gotW, wantW)
			}
		})
	}
}
