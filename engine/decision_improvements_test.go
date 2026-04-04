package engine

import "testing"

func TestDefenderPlaceUsesFallbackTile(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Player1] = 1000
	g.CurrentTurn = g.Player1

	py, px := g.Paths[0][0].Y, g.Paths[0][0].X
	g.applyDecision(g.Player1, "defender", map[string]interface{}{
		"action":     "place",
		"tower_type": "basic",
		"position":   []interface{}{float64(py), float64(px)},
	})

	if len(g.Towers) == 0 {
		t.Fatalf("expected fallback tower placement to succeed")
	}
	if g.LastActionStatus[g.Player1] != "applied_fallback" && g.LastActionStatus[g.Player1] != "applied_primary" {
		t.Fatalf("expected applied status, got %s", g.LastActionStatus[g.Player1])
	}
}

func TestAttackerAutoWaveWhenRich(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Player2] = 500
	g.WaveQueue = nil

	g.applyDecision(g.Player2, "attacker", map[string]interface{}{
		"action": "save",
	})

	if g.Wave == 0 {
		t.Fatalf("expected auto wave launch to increase wave count")
	}
	if len(g.WaveQueue) == 0 {
		t.Fatalf("expected auto wave queue to have enemies")
	}
}

