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
	if g.ReplayEvents[0].Action != "place" || g.ReplayEvents[1].Action != "spawn" {
		t.Fatalf("expected oldest event trimmed, got %#v", g.ReplayEvents)
	}
	if g.ReplayEvents[0].Tick != 7 || g.ReplayEvents[1].Tick != 7 {
		t.Fatalf("expected tick to be copied into events, got %#v", g.ReplayEvents)
	}
	if g.ReplayEvents[0].Model == "" || g.ReplayEvents[1].Model == "" {
		t.Fatalf("expected model names to be populated for player events")
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
