package engine

import "testing"

func TestBuildMatchResultIncludesCoreMetrics(t *testing.T) {
	g := NewGame("test", "test")
	g.TickCount = 12
	g.Wave = 3
	g.GameOver = true
	g.Winner = g.Defender
	g.ActionCounters[g.Player1+":place"] = 2

	result := g.BuildMatchResult()

	if result.Ticks != 12 {
		t.Fatalf("expected ticks 12, got %d", result.Ticks)
	}
	if result.WinReason != "max_waves_cleared" {
		t.Fatalf("expected defender win reason, got %s", result.WinReason)
	}
	if result.ActionCounters[g.Player1+":place"] != 2 {
		t.Fatalf("expected action counters to be copied")
	}
}

