package engine

import (
	"reflect"
	"strings"
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
	// custom is deliberately absent: it is not in the prompt schema and
	// placeTower rejects it, so advertising it would guarantee rejections.
	want := []string{
		"save",
		"place:basic", "place:splash",
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

func TestAffordableActionsExcludesPlaceWhenBoardSaturated(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Defender] = 500
	// Shrink to a two-tile path and block every legal neighbor cell so no
	// tower placement exists anywhere.
	g.Paths = [][]Position{{{Y: 2, X: 2}, {Y: 2, X: 3}}}
	g.rebuildPathTileSet()
	g.Obstacles = nil
	g.ObstacleTileSet = map[string]struct{}{}
	g.Towers = nil
	for _, pos := range []Position{{Y: 1, X: 2}, {Y: 3, X: 2}, {Y: 2, X: 1}, {Y: 1, X: 3}, {Y: 3, X: 3}, {Y: 2, X: 4}} {
		tw := NewTower(pos.Y, pos.X, "basic", nil)
		g.Towers = append(g.Towers, &tw)
	}

	got := g.affordableActions(g.Defender, "defender")
	for _, action := range got {
		if strings.HasPrefix(action, "place:") {
			t.Fatalf("expected no place actions on saturated board, got %v", got)
		}
	}
	// Everything else stays affordable at 500 resources.
	found := map[string]bool{}
	for _, action := range got {
		found[action] = true
	}
	for _, want := range []string{"save", "place_slow_zone", "invest"} {
		if !found[want] {
			t.Fatalf("expected %q still affordable, got %v", want, got)
		}
	}
}

func TestFormatAffordableActionsIncludesTowerCandidates(t *testing.T) {
	state := map[string]interface{}{
		"affordable_actions":     []string{"save", "place:basic"},
		"valid_tower_candidates": [][]int{{3, 12}, {5, 7}, {6, 9}, {8, 1}},
	}
	got := formatAffordableActions(state)
	if !strings.Contains(got, "place:basic") {
		t.Fatalf("expected place listed, got %q", got)
	}
	if !strings.Contains(got, "[3 12]") || !strings.Contains(got, "[5 7]") || !strings.Contains(got, "[6 9]") {
		t.Fatalf("expected first three candidate cells inline, got %q", got)
	}
	if strings.Contains(got, "[8 1]") {
		t.Fatalf("expected candidate list capped at three, got %q", got)
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
