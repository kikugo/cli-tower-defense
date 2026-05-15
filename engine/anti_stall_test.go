package engine

import "testing"

func TestRepeatedDefenderSavesCanAutoPlaceDefense(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Player1] = 300
	g.NoopStreak[g.Player1] = 2

	g.applyDecision(g.Player1, "defender", map[string]interface{}{"action": "save"})

	if len(g.Towers) == 0 {
		t.Fatalf("expected repeated defender saves to create an auto defense")
	}
	if g.LastActionStatus[g.Player1] != "applied_auto_defense" {
		t.Fatalf("expected auto defense status, got %s", g.LastActionStatus[g.Player1])
	}
}

func TestRepeatedAttackerSavesLowerAutoWaveThreshold(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Player2] = 180
	g.NoopStreak[g.Player2] = 2

	if !g.shouldAutoLaunchWave(g.Player2) {
		t.Fatalf("expected repeated attacker saves to lower wave threshold")
	}
}
