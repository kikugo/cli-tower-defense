package engine

import "testing"

func TestGameStateIncludesFogVisibilitySummary(t *testing.T) {
	g := NewGame("test", "test")
	g.SetRandomSeed(7)
	g.Resources[g.Player1] = 1000

	if !g.placeTower(2, 2, "basic") {
		t.Fatalf("expected setup tower placement to succeed")
	}

	path := g.Paths[0]
	near := NewEnemy(path[3].Y, path[3].X, "basic", nil)
	near.PathID = 0
	near.PathIndex = 3
	far := NewEnemy(path[len(path)-1].Y, path[len(path)-1].X, "basic", nil)
	far.PathID = 0
	far.PathIndex = len(path) - 1
	g.Enemies = []*Enemy{&near, &far}

	state := g.getGameState()
	visibility, ok := state["visibility"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected visibility summary in game state")
	}
	if visibility["fog_enabled"] != true {
		t.Fatalf("expected fog-enabled visibility summary")
	}
	visibleEnemies, ok := visibility["visible_enemies"].(int)
	if !ok {
		t.Fatalf("expected visible enemy count")
	}
	if visibleEnemies >= len(g.Enemies) {
		t.Fatalf("expected fog to hide at least one enemy, saw %d of %d", visibleEnemies, len(g.Enemies))
	}
}

func TestDefenderResearchImprovesEconomy(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Player1] = 1000
	startIncome := g.Income[g.Player1]

	g.applyDecision(g.Player1, "defender", map[string]interface{}{
		"action": "research",
		"tech":   "economy",
	})

	if g.Income[g.Player1] <= startIncome {
		t.Fatalf("expected economy research to improve defender income, got %d from %d", g.Income[g.Player1], startIncome)
	}

	state := g.getGameState()
	research, ok := state["research"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected research summary in game state")
	}
	if _, ok := research["economy"]; !ok {
		t.Fatalf("expected economy research level in state")
	}
}

func TestAttackerAbilityAppliesToActiveEnemies(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Player2] = 1000

	path := g.Paths[0]
	enemy := NewEnemy(path[0].Y, path[0].X, "basic", nil)
	enemy.PathID = 0
	g.Enemies = []*Enemy{&enemy}
	baseSpeed := g.Enemies[0].Speed

	g.applyDecision(g.Player2, "attacker", map[string]interface{}{
		"action":  "ability",
		"ability": "surge",
	})

	if g.Enemies[0].Speed <= baseSpeed {
		t.Fatalf("expected surge ability to increase enemy speed, got %.2f from %.2f", g.Enemies[0].Speed, baseSpeed)
	}

	state := g.getGameState()
	abilities, ok := state["attacker_abilities"].([]interface{})
	if !ok || len(abilities) == 0 {
		t.Fatalf("expected attacker abilities to be exposed in game state")
	}
}

func TestAdaptiveWaveDirectorAddsPressureOnQuietBoard(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Player2] = 500
	g.NoopStreak[g.Player2] = 4
	g.WaveQueue = nil
	g.Enemies = nil

	for i := 0; i < 40; i++ {
		g.UpdateGameState()
	}

	if len(g.WaveQueue) == 0 && len(g.Enemies) == 0 {
		t.Fatalf("expected adaptive wave director to add pressure on a quiet board")
	}

	state := g.getGameState()
	pressure, ok := state["director"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected director summary in game state")
	}
	if _, ok := pressure["pressure_level"]; !ok {
		t.Fatalf("expected director pressure level in state")
	}
}
