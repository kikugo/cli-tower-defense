package engine

import "testing"

func TestBuildMatchResultIncludesCoreMetrics(t *testing.T) {
	g := NewGame("test", "test")
	g.TickCount = 12
	g.Wave = 3
	g.GameOver = true
	g.Winner = g.Defender
	g.ActionCounters[g.Player1+":place"] = 2
	g.ProviderCalls[g.Player1] = 2
	g.ProviderLatencyMS[g.Player1] = 30
	g.ProviderTokenUsage[g.Player1] = 123
	g.ProviderCostMicros[g.Player1] = 456

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
	if _, ok := result.NormalizedScore[g.Player1]; !ok {
		t.Fatalf("expected normalized score for player1")
	}
	if _, ok := result.ScoreBreakdown[g.Player2]; !ok {
		t.Fatalf("expected score breakdown for player2")
	}
	if result.ProviderLatency[g.Player1] != 15 {
		t.Fatalf("expected average latency 15ms, got %f", result.ProviderLatency[g.Player1])
	}
	if result.TokenUsage[g.Player1] != 123 || result.CostMicros[g.Player1] != 456 {
		t.Fatalf("expected token/cost telemetry to be copied")
	}
}
