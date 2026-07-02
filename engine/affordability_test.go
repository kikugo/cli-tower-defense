package engine

import (
	"reflect"
	"testing"
)

func TestAffordableActionsDefenderBroke(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Defender] = 50
	got := g.affordableActions(g.Defender, "defender")
	want := []string{"save"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestAffordableActionsDefenderRich(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Defender] = 200
	got := g.affordableActions(g.Defender, "defender")
	want := []string{
		"save",
		"place:basic", "place:custom", "place:splash",
		"place_slow_zone",
		"research:economy", "research:range", "research:control",
		"invest",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestAffordableActionsAttackerMidResources(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Attacker] = 100
	got := g.affordableActions(g.Attacker, "attacker")
	want := []string{
		"save",
		"spawn:basic", "spawn:fast", "spawn:healer", "spawn:shielded", "spawn:tank",
		"wave",
		"ability:surge", "ability:shield_burst", "ability:reinforce_wave",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestPlayerGameStateIncludesAffordableActions(t *testing.T) {
	g := NewGame("test", "test")
	defState := g.getPlayerGameState(g.Defender, "defender")
	if _, ok := defState["affordable_actions"].([]string); !ok {
		t.Fatalf("expected defender affordable_actions, got %#v", defState["affordable_actions"])
	}
	attState := g.getPlayerGameState(g.Attacker, "attacker")
	if _, ok := attState["affordable_actions"].([]string); !ok {
		t.Fatalf("expected attacker affordable_actions, got %#v", attState["affordable_actions"])
	}
}

func TestWaveCostForWaveCapsAt200(t *testing.T) {
	if got := waveCostForWave(0); got != 40 {
		t.Fatalf("wave 0 cost: got %d want 40", got)
	}
	if got := waveCostForWave(50); got != 200 {
		t.Fatalf("wave 50 cost: got %d want 200", got)
	}
}
