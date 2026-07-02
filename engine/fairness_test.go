package engine

import (
	"testing"
	"time"
)

func TestStrictAlternationGivesBothPlayersTurns(t *testing.T) {
	g := NewGame("test", "test")
	g.PauseBetweenTurns = false
	g.AIDecisionInterval[g.Player1] = 0
	g.AIDecisionInterval[g.Player2] = 0
	// Below the old 100-resource defender gate: before the fix the defender
	// turn is silently skipped and it never accumulates provider calls.
	g.Resources[g.Defender] = 50

	sp := &scriptedProvider{
		defenderAction: map[string]interface{}{"action": "save"},
		attackerAction: map[string]interface{}{"action": "save"},
	}
	g.DecisionRouter.SetPlayerProvider(g.Player1, sp)
	g.DecisionRouter.SetPlayerProvider(g.Player2, sp)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		g.HandleAIDecisions()
		if g.ProviderCalls[g.Player1] >= 6 && g.ProviderCalls[g.Player2] >= 6 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	p1, p2 := g.ProviderCalls[g.Player1], g.ProviderCalls[g.Player2]
	if p1 < 6 || p2 < 6 {
		t.Fatalf("expected both players to accumulate calls, got p1=%d p2=%d", p1, p2)
	}
	if diff := abs(p1 - p2); diff > 1 {
		t.Fatalf("expected call parity, got p1=%d p2=%d", p1, p2)
	}
}
