package engine

import (
	"testing"
	"time"
)

func TestDecisionIntervalBlocksWhenNotElapsed(t *testing.T) {
	g := NewGame("test", "test")
	player := g.Player1
	now := time.Now()
	g.AIDecisionInterval[player] = 3
	g.LastAIDecision[player] = now.Add(-2 * time.Second)

	if g.isDecisionIntervalElapsed(player, now) {
		t.Fatalf("expected decision interval to block action before full interval")
	}
}

func TestDecisionIntervalAllowsWhenElapsed(t *testing.T) {
	g := NewGame("test", "test")
	player := g.Player2
	now := time.Now()
	g.AIDecisionInterval[player] = 2
	g.LastAIDecision[player] = now.Add(-2 * time.Second)

	if !g.isDecisionIntervalElapsed(player, now) {
		t.Fatalf("expected decision interval to allow action when elapsed")
	}
}

func TestDecisionIntervalAllowsWhenDisabled(t *testing.T) {
	g := NewGame("test", "test")
	player := g.Player1
	g.AIDecisionInterval[player] = 0

	if !g.isDecisionIntervalElapsed(player, time.Now()) {
		t.Fatalf("expected disabled interval to always allow action")
	}
}

