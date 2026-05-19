package engine

import "fmt"

type ArenaRuleset struct {
	Name                string `json:"name"`
	MaxTicks            int    `json:"max_ticks"`
	MaxWaves            int    `json:"max_waves"`
	MapType             string `json:"map_type"`
	StartingResources   int    `json:"starting_resources"`
	StartingIncome      int    `json:"starting_income"`
	StartingLives       int    `json:"starting_lives"`
	AutoWaveMinResource int    `json:"auto_wave_min_resource"`
	AutoDefendMinStreak int    `json:"auto_defend_min_streak"`
}

func DefaultArenaRuleset() ArenaRuleset {
	return ArenaRuleset{
		Name:                "default",
		MaxTicks:            3000,
		MaxWaves:            30,
		MapType:             "",
		StartingResources:   300,
		StartingIncome:      5,
		StartingLives:       20,
		AutoWaveMinResource: 260,
		AutoDefendMinStreak: 2,
	}
}

func PresetArenaRuleset(name string) (ArenaRuleset, error) {
	base := DefaultArenaRuleset()
	switch name {
	case "", "default":
		return base, nil
	case "fast":
		base.Name = "fast"
		base.MaxTicks = 1800
		base.MaxWaves = 18
		base.AutoWaveMinResource = 180
		return base, nil
	case "marathon":
		base.Name = "marathon"
		base.MaxTicks = 6000
		base.MaxWaves = 45
		base.StartingResources = 350
		return base, nil
	default:
		return ArenaRuleset{}, fmt.Errorf("unknown ruleset preset %q", name)
	}
}

func (g *Game) ApplyRuleset(ruleset ArenaRuleset) {
	if g == nil {
		return
	}
	if ruleset.MaxWaves > 0 {
		g.MaxWaves = ruleset.MaxWaves
	}
	if ruleset.StartingResources > 0 {
		g.Resources[g.Player1] = ruleset.StartingResources
		g.Resources[g.Player2] = ruleset.StartingResources
	}
	if ruleset.StartingIncome > 0 {
		g.Income[g.Player1] = ruleset.StartingIncome
		g.Income[g.Player2] = ruleset.StartingIncome
	}
	if ruleset.StartingLives > 0 {
		g.Lives[g.Player1] = ruleset.StartingLives
		g.Lives[g.Player2] = ruleset.StartingLives
	}
	if ruleset.AutoWaveMinResource > 0 {
		g.AutoWaveMinResource = ruleset.AutoWaveMinResource
	}
	if ruleset.AutoDefendMinStreak > 0 {
		g.AutoDefendMinStreak = ruleset.AutoDefendMinStreak
	}
	if ruleset.MapType != "" {
		g.SetMapType(ruleset.MapType)
	}
}
