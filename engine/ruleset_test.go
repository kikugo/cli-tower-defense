package engine

import "testing"

func TestPresetArenaRulesetFast(t *testing.T) {
	r, err := PresetArenaRuleset("fast")
	if err != nil {
		t.Fatalf("expected preset to load: %v", err)
	}
	if r.MaxTicks >= DefaultArenaRuleset().MaxTicks {
		t.Fatalf("expected fast ruleset max ticks to be lower")
	}
	if r.MaxWaves >= DefaultArenaRuleset().MaxWaves {
		t.Fatalf("expected fast ruleset max waves to be lower")
	}
}

func TestApplyRulesetUpdatesGameSettings(t *testing.T) {
	g := NewGame("a", "b")
	r := ArenaRuleset{
		MaxWaves:            12,
		MapType:             "straight",
		StartingResources:   420,
		StartingIncome:      9,
		StartingLives:       17,
		AutoWaveMinResource: 190,
		AutoDefendMinStreak: 3,
	}
	g.ApplyRuleset(r)

	if g.MaxWaves != 12 {
		t.Fatalf("expected max waves 12, got %d", g.MaxWaves)
	}
	if g.Resources[g.Player1] != 420 || g.Resources[g.Player2] != 420 {
		t.Fatalf("expected both player resources to be 420, got %d/%d", g.Resources[g.Player1], g.Resources[g.Player2])
	}
	if g.Income[g.Player1] != 9 || g.Income[g.Player2] != 9 {
		t.Fatalf("expected both player income to be 9, got %d/%d", g.Income[g.Player1], g.Income[g.Player2])
	}
	if g.Lives[g.Player1] != 17 || g.Lives[g.Player2] != 17 {
		t.Fatalf("expected both player lives to be 17, got %d/%d", g.Lives[g.Player1], g.Lives[g.Player2])
	}
	if g.AutoWaveMinResource != 190 || g.AutoDefendMinStreak != 3 {
		t.Fatalf("expected thresholds 190/3, got %d/%d", g.AutoWaveMinResource, g.AutoDefendMinStreak)
	}
	if g.MapType != "straight" {
		t.Fatalf("expected map type straight, got %q", g.MapType)
	}
}
