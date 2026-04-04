package engine

import (
	"errors"
	"testing"
)

func TestApplyDecisionTracksRejectedActions(t *testing.T) {
	g := NewGame("test", "test")
	g.applyDecision(g.Player1, "defender", map[string]interface{}{
		"action":     "place",
		"tower_type": "basic",
		"position":   []interface{}{float64(-100), float64(-100)},
	})

	if g.RejectedActions[g.Player1+":place"] == 0 {
		t.Fatalf("expected rejected place action counter to increment")
	}
}

func TestProcessPendingTurnResultsTracksProviderErrors(t *testing.T) {
	g := NewGame("test", "test")
	g.PauseBetweenTurns = false
	g.pendingTurnResults <- turnResult{
		playerID: g.Player1,
		role:     "defender",
		err:      errors.New("status 500"),
	}

	g.processPendingTurnResults()

	if g.TotalProviderErrorsForPlayer(g.Player1) == 0 {
		t.Fatalf("expected provider error counter to increment")
	}
}
