package engine

import (
	"testing"
	"time"
)

// TestSwitchTurnDoesNotBlock guards against reintroducing the synchronous
// time.Sleep(PauseDuration) that used to live in switchTurn. switchTurn is
// reached from HandleAIDecisions, which the Bubble Tea Update loop calls
// directly (main.go), so any blocking call here stalls the whole TUI (no
// repaints, no keypress handling) for PauseDuration on every turn switch.
func TestSwitchTurnDoesNotBlock(t *testing.T) {
	g := NewGame("test", "test")
	g.PauseBetweenTurns = true
	g.PauseDuration = 1 * time.Second

	start := time.Now()
	g.switchTurn()
	elapsed := time.Since(start)

	if elapsed > time.Millisecond {
		t.Fatalf("expected switchTurn to return in under 1ms, took %v", elapsed)
	}
}

// newScriptedPauseGame builds a game with instant, offline scripted
// providers on both sides so HandleAIDecisions can actually dispatch a turn
// without a network call.
func newScriptedPauseGame() *Game {
	resolved := ResolvedMatchConfig{
		Player1: ResolvedPlayerModelConfig{PlayerModelConfig: PlayerModelConfig{
			Provider: ProviderScripted, Model: "defender_baseline",
		}},
		Player2: ResolvedPlayerModelConfig{PlayerModelConfig: PlayerModelConfig{
			Provider: ProviderScripted, Model: "attacker_baseline",
		}},
	}
	g := NewGameFromResolvedConfig(resolved)
	g.AIDecisionInterval[g.Player1] = 0
	g.AIDecisionInterval[g.Player2] = 0
	return g
}

// TestPausedGameWithholdsDispatchUntilPauseDurationElapses is the
// watchability-pacing contract: with PauseBetweenTurns true, the next
// player's turn must not be dispatched (handlePlayerTurn setting AIThinking)
// before PauseDuration real time has passed since the switch, even though
// switchTurn itself no longer blocks to enforce that.
func TestPausedGameWithholdsDispatchUntilPauseDurationElapses(t *testing.T) {
	g := newScriptedPauseGame()
	g.PauseBetweenTurns = true
	g.PauseDuration = 60 * time.Millisecond

	g.switchTurn()
	current := g.CurrentTurn

	// Immediately after the switch: must not dispatch yet.
	g.HandleAIDecisions()
	if g.AIThinking[current] {
		t.Fatalf("expected dispatch to wait for PauseDuration, but AIThinking[%s] is true immediately after switchTurn", current)
	}

	// Still within the pause window: must still not dispatch.
	time.Sleep(g.PauseDuration / 2)
	g.HandleAIDecisions()
	if g.AIThinking[current] {
		t.Fatalf("expected dispatch to remain withheld before PauseDuration elapses")
	}

	// Past the deadline: dispatch must now happen.
	time.Sleep(g.PauseDuration)
	g.HandleAIDecisions()
	if !g.AIThinking[current] {
		t.Fatalf("expected dispatch once PauseDuration has elapsed")
	}
}

// TestUnpausedGameDispatchesImmediately confirms PauseBetweenTurns=false
// (the setting used by RunScriptedDuel, headless runs, and tournaments)
// keeps dispatching without any withholding, i.e. the new deadline check is
// a no-op on that path.
func TestUnpausedGameDispatchesImmediately(t *testing.T) {
	g := newScriptedPauseGame()
	g.PauseBetweenTurns = false

	g.switchTurn()
	current := g.CurrentTurn

	g.HandleAIDecisions()
	if !g.AIThinking[current] {
		t.Fatalf("expected immediate dispatch when PauseBetweenTurns is false")
	}
}
