package engine

import "testing"

func TestRecordReplayEventFillsDefaultsAndCapsBuffer(t *testing.T) {
	g := NewGame("test", "test")
	g.MaxReplayEvents = 2
	g.TickCount = 7

	g.recordReplayEvent(ReplayEvent{Type: ReplayTick})
	g.recordReplayEvent(ReplayEvent{Type: ReplayDecision, PlayerID: g.Player1, Action: "place"})
	g.recordReplayEvent(ReplayEvent{Type: ReplayDecision, PlayerID: g.Player2, Action: "spawn"})

	if len(g.ReplayEvents) != 2 {
		t.Fatalf("expected capped replay length 2, got %d", len(g.ReplayEvents))
	}
	// The oldest event (the tick) is still discarded to respect the cap, but
	// unlike before, it is not discarded silently: a ReplayTruncated marker
	// takes its place so a caller reconstructing from this stream can tell.
	if g.ReplayEvents[0].Type != ReplayTruncated {
		t.Fatalf("expected a truncation marker at index 0, got %#v", g.ReplayEvents[0])
	}
	if g.ReplayEvents[1].Action != "spawn" {
		t.Fatalf("expected newest event kept, got %#v", g.ReplayEvents)
	}
	if !g.ReplayTruncated {
		t.Fatalf("expected g.ReplayTruncated to be set once events are discarded")
	}
	// Max is 2 and 3 events were recorded (tick, place, spawn); both the
	// tick and the place decision had to go to make room for the marker
	// itself plus the newest event.
	discarded, ok := toIntFromAny(g.ReplayEvents[0].Details["discarded_events"])
	if !ok || discarded != 2 {
		t.Fatalf("expected marker to record 2 discarded events, got %v (ok=%v)", g.ReplayEvents[0].Details["discarded_events"], ok)
	}
	if g.ReplayEvents[1].Tick != 7 {
		t.Fatalf("expected tick to be copied into events, got %#v", g.ReplayEvents)
	}
	if g.ReplayEvents[1].Model == "" {
		t.Fatalf("expected model names to be populated for player events")
	}
}

// TestTrimReplayEventsMarkerUpdatesInPlace verifies that once a truncation
// marker exists, further overflow keeps updating that same marker (merging
// newly-discarded events into its count) rather than growing the stream or
// planting a second marker -- this is what keeps trimming O(1) per event for
// the remainder of a long match.
func TestTrimReplayEventsMarkerUpdatesInPlace(t *testing.T) {
	g := NewGame("test", "test")
	g.MaxReplayEvents = 3
	g.recordMapInitEvent()

	for i := 0; i < 20; i++ {
		g.recordReplayEvent(ReplayEvent{Type: ReplayTick})
	}

	if len(g.ReplayEvents) != 3 {
		t.Fatalf("expected capped replay length 3, got %d", len(g.ReplayEvents))
	}
	if g.ReplayEvents[0].Type != ReplayMapInit {
		t.Fatalf("expected map_init preserved at index 0, got %#v", g.ReplayEvents[0])
	}
	if g.ReplayEvents[1].Type != ReplayTruncated {
		t.Fatalf("expected a single truncation marker at index 1, got %#v", g.ReplayEvents[1])
	}
	// 1 map_init + 20 ticks = 21 events total; 3 survive untouched (map_init,
	// marker, last tick), so 19 must have been folded into the marker.
	discarded, ok := toIntFromAny(g.ReplayEvents[1].Details["discarded_events"])
	if !ok || discarded != 19 {
		t.Fatalf("expected marker to record 19 discarded events, got %v (ok=%v)", g.ReplayEvents[1].Details["discarded_events"], ok)
	}
}

func TestBuildMatchResultIncludesReplayCount(t *testing.T) {
	g := NewGame("test", "test")
	g.recordReplayEvent(ReplayEvent{Type: ReplayTick})
	g.recordReplayEvent(ReplayEvent{Type: ReplayWave, PlayerID: g.Player2})

	result := g.BuildMatchResult()
	if result.ReplayEvents != 2 {
		t.Fatalf("expected replay event count 2, got %d", result.ReplayEvents)
	}
}
