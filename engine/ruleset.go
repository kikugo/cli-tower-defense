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
	FogOfWar            bool   `json:"fog_of_war"`
	DefenderVisionRange int    `json:"defender_vision_range"`
	BaseVisionRange     int    `json:"base_vision_range"`
	// DisableAssists turns off engine help (auto-wave, auto-defend, adaptive
	// pressure) so matches measure pure model decisions. Zero value keeps
	// assists on, so existing ruleset JSONs are unaffected.
	DisableAssists bool `json:"disable_assists"`
	// SkipForcedSaveTurns short-circuits a player's turn without dispatching
	// to the provider when affordableActions is exactly {"save"} -- nothing
	// else is legal, so the round trip could only ever come back as "save".
	// This is an explicit opt-in: skipping the call also means the engine
	// never records the rejection a provider might otherwise have produced
	// by proposing something unaffordable anyway, and rejection/fallback
	// rates are a recorded discipline metric (see HANDOFF.md 8g and the
	// 0.02/rejection scoring penalty in scoring.go). Zero value is false,
	// so every existing ruleset JSON and every existing test keeps today's
	// behaviour: the provider is always asked. See HandleAIDecisions and
	// SourceSkippedForcedSave.
	SkipForcedSaveTurns bool `json:"skip_forced_save_turns"`
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
		FogOfWar:            true,
		DefenderVisionRange: 8,
		BaseVisionRange:     6,
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
	case "fair":
		base.Name = "fair"
		base.DisableAssists = true
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
		// StartingLives anchors WaveSummary's lives ledger -- see its doc on
		// Game. A ruleset is always applied before the match plays any
		// ticks (both call sites apply it immediately after construction),
		// so there is never telemetry already in flight for this to
		// invalidate.
		g.StartingLives = ruleset.StartingLives
	}
	if ruleset.AutoWaveMinResource > 0 {
		g.AutoWaveMinResource = ruleset.AutoWaveMinResource
	}
	if ruleset.AutoDefendMinStreak > 0 {
		g.AutoDefendMinStreak = ruleset.AutoDefendMinStreak
	}
	if ruleset.FogOfWar {
		g.FogOfWar = true
	}
	if ruleset.DefenderVisionRange > 0 {
		g.DefenderVisionRange = ruleset.DefenderVisionRange
	}
	if ruleset.BaseVisionRange > 0 {
		g.BaseVisionRange = ruleset.BaseVisionRange
	}
	if ruleset.MapType != "" {
		g.SetMapType(ruleset.MapType)
	}
	g.AssistsDisabled = ruleset.DisableAssists
	g.SkipForcedSaveTurns = ruleset.SkipForcedSaveTurns
}
