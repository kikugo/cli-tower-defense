package engine

import "testing"

func TestGameStateIncludesValidTowerCandidates(t *testing.T) {
	g := NewGame("test", "test")
	state := g.getGameState()

	candidates, ok := state["valid_tower_candidates"].([][]int)
	if !ok {
		t.Fatalf("expected valid_tower_candidates in state")
	}
	if len(candidates) == 0 {
		t.Fatalf("expected at least one valid tower candidate")
	}
}

func TestGameStateIncludesPressureSummary(t *testing.T) {
	g := NewGame("test", "test")
	state := g.getGameState()

	pressure, ok := state["pressure"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected pressure summary in state")
	}
	if _, ok := pressure["attacker_resources"]; !ok {
		t.Fatalf("expected attacker_resources in pressure summary")
	}
}
