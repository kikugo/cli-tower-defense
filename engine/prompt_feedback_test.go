package engine

import "testing"

func TestGameStateIncludesRejectedActionFeedback(t *testing.T) {
	g := NewGame("test", "test")
	g.applyDecision(g.Player1, "defender", map[string]interface{}{
		"action":   "upgrade",
		"tower_id": float64(99),
	})

	state := g.getGameState()
	feedback, ok := state["last_rejected_reason"].(map[string]string)
	if !ok {
		t.Fatalf("expected last rejected reason feedback")
	}
	if feedback[g.Player1] == "" {
		t.Fatalf("expected rejected reason for player1")
	}
}
