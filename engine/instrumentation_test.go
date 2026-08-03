package engine

import (
	"errors"
	"strings"
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

// TestApplyDecisionActionCounterIncludesEntityType guards the ActionCounters
// key extension: place/spawn now key on playerID:action:type so per-content
// usage (which tower/enemy types are actually instantiated) can be read
// directly off a match result instead of mined from ReplayEvents.
func TestApplyDecisionActionCounterIncludesEntityType(t *testing.T) {
	g := NewGame("test", "test")
	candidates := g.validTowerCandidates(1)
	if len(candidates) == 0 {
		t.Fatalf("expected at least one valid tower candidate on a fresh game")
	}
	y, x := candidates[0][0], candidates[0][1]
	g.applyDecision(g.Player1, "defender", map[string]interface{}{
		"action":     "place",
		"tower_type": "basic",
		"position":   []interface{}{float64(y), float64(x)},
	})

	if g.ActionCounters[g.Player1+":place:basic"] != 1 {
		t.Fatalf("expected typed place counter to increment, got counters=%v", g.ActionCounters)
	}
	if _, ok := g.ActionCounters[g.Player1+":place"]; ok {
		t.Fatalf("did not expect an untyped 2-part key once entity type is known, counters=%v", g.ActionCounters)
	}
}

// TestApplyDecisionActionCounterTypedEvenWhenRejected checks the type is
// recorded from the attempted decision, not just on success, matching the
// old counter's behavior of counting every decision regardless of outcome.
func TestApplyDecisionActionCounterTypedEvenWhenRejected(t *testing.T) {
	g := NewGame("test", "test")
	g.applyDecision(g.Player1, "defender", map[string]interface{}{
		"action":     "place",
		"tower_type": "basic",
		"position":   []interface{}{float64(-100), float64(-100)},
	})

	if g.ActionCounters[g.Player1+":place:basic"] != 1 {
		t.Fatalf("expected typed place counter to increment even on rejection, got counters=%v", g.ActionCounters)
	}
}

// TestApplyDecisionActionCounterTypedForSpawn covers the attacker side of
// the key extension.
func TestApplyDecisionActionCounterTypedForSpawn(t *testing.T) {
	g := NewGame("test", "test")
	// Keep resources below AutoWaveMinResource (260) so shouldAutoLaunchWave
	// doesn't intercept this as an auto "wave" action instead of "spawn".
	g.Resources[g.Player2] = 200
	g.applyDecision(g.Player2, "attacker", map[string]interface{}{
		"action":     "spawn",
		"enemy_type": "shielded",
	})

	if g.ActionCounters[g.Player2+":spawn:shielded"] != 1 {
		t.Fatalf("expected typed spawn counter to increment, got counters=%v", g.ActionCounters)
	}
}

// TestApplyDecisionActionCounterKeepsTwoPartKeyWithoutEntityType is the
// backward-compatibility half of the contract: actions with no tower/enemy
// type (invest, upgrade, research, ...) must keep the original 2-part key
// so any downstream reader that assumes "playerID:action" still works.
func TestApplyDecisionActionCounterKeepsTwoPartKeyWithoutEntityType(t *testing.T) {
	g := NewGame("test", "test")
	g.applyDecision(g.Player1, "defender", map[string]interface{}{"action": "invest"})

	if g.ActionCounters[g.Player1+":invest"] != 1 {
		t.Fatalf("expected untyped invest counter to increment, got counters=%v", g.ActionCounters)
	}
}

func TestActionCounterKeyHelper(t *testing.T) {
	if got := actionCounterKey("p1", "place", "basic"); got != "p1:place:basic" {
		t.Fatalf("expected typed 3-part key, got %q", got)
	}
	if got := actionCounterKey("p1", "invest", ""); got != "p1:invest" {
		t.Fatalf("expected untyped 2-part key, got %q", got)
	}
}

func TestRejectedActionLogsReason(t *testing.T) {
	g := NewGame("test", "test")
	g.applyDecision(g.Player1, "defender", map[string]interface{}{
		"action":   "upgrade",
		"tower_id": float64(999),
	})

	found := false
	for _, msg := range g.Logs {
		if strings.Contains(msg, "action rejected") && strings.Contains(msg, "invalid_or_unaffordable_upgrade") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected rejected-action log with reason, logs=%v", g.Logs)
	}
}
