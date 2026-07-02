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

// TestHeadlessLoopAdvancesTicks mirrors main's runHeadlessSimulation loop:
// with strict alternation the sim must still tick between decisions instead
// of starving behind back-to-back dispatches.
func TestHeadlessLoopAdvancesTicks(t *testing.T) {
	g := NewGame("test", "test")
	g.PauseBetweenTurns = false
	g.AIDecisionInterval[g.Player1] = 0
	g.AIDecisionInterval[g.Player2] = 0

	sp := &scriptedProvider{
		defenderAction: map[string]interface{}{"action": "save"},
		attackerAction: map[string]interface{}{"action": "save"},
	}
	g.DecisionRouter.SetPlayerProvider(g.Player1, sp)
	g.DecisionRouter.SetPlayerProvider(g.Player2, sp)

	ticks := 0
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && ticks < 10 && !g.GameOver {
		if g.AIThinking[g.Player1] || g.AIThinking[g.Player2] {
			g.HandleAIDecisions()
			time.Sleep(time.Millisecond)
			continue
		}
		g.UpdateGameState()
		g.HandleAIDecisions()
		ticks++
	}
	if ticks < 10 {
		t.Fatalf("expected simulation to reach 10 ticks, got %d (loop starved by dispatches)", ticks)
	}
}

func TestBreachStopsScoringAfterGameOver(t *testing.T) {
	g := NewGame("test", "test")
	g.SetMapType("straight")
	g.Lives[g.Defender] = 1

	path := g.Paths[0]
	last := path[len(path)-1]
	for i := 0; i < 3; i++ {
		e := NewEnemy(last.Y, last.X, "basic", nil)
		e.PathIndex = len(path) - 1
		g.Enemies = append(g.Enemies, &e)
	}
	attackerScoreBefore := g.Score[g.Attacker]

	g.UpdateGameState()

	if g.Lives[g.Defender] != 0 {
		t.Fatalf("expected lives clamped at 0, got %d", g.Lives[g.Defender])
	}
	if !g.GameOver || g.Winner != g.Attacker {
		t.Fatalf("expected attacker win, over=%v winner=%q", g.GameOver, g.Winner)
	}
	// Only the first breach may score; the two post-game-over breaches must not.
	if got := g.Score[g.Attacker] - attackerScoreBefore; got != 50 {
		t.Fatalf("expected exactly one scored breach (50), got %d", got)
	}
	ends := 0
	for _, ev := range g.ReplayEvents {
		if ev.Type == ReplayGameEnd {
			ends++
		}
	}
	if ends != 1 {
		t.Fatalf("expected exactly one game_end event, got %d", ends)
	}
}
