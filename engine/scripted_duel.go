package engine

import "time"

// ScriptedDuelConfig describes one offline scripted-vs-scripted match used
// for balance sweeps and regression tests.
type ScriptedDuelConfig struct {
	Seed           int64
	MaxTicks       int
	Ruleset        ArenaRuleset
	Balance        BalanceConfig
	DefenderScript string
	AttackerScript string
}

// BaselineDuelRuleset is the standard balance-measurement arena: assists off,
// straight lane, modest lives so duels resolve quickly.
func BaselineDuelRuleset() ArenaRuleset {
	rs := DefaultArenaRuleset()
	rs.Name = "baseline-duel"
	rs.MapType = "straight"
	rs.MaxWaves = 5
	rs.StartingResources = 400
	rs.StartingIncome = 10
	rs.StartingLives = 6
	rs.DisableAssists = true
	return rs
}

// RunScriptedDuel plays a full offline match between two scripted providers
// and returns the result. No network, sub-second runtime.
func RunScriptedDuel(cfg ScriptedDuelConfig) MatchResult {
	resolved := ResolvedMatchConfig{
		Player1: ResolvedPlayerModelConfig{PlayerModelConfig: PlayerModelConfig{
			Provider: ProviderScripted, Model: cfg.DefenderScript, APIKeyEnv: "NONE",
		}},
		Player2: ResolvedPlayerModelConfig{PlayerModelConfig: PlayerModelConfig{
			Provider: ProviderScripted, Model: cfg.AttackerScript, APIKeyEnv: "NONE",
		}},
	}
	g := NewGameFromResolvedConfig(resolved)
	g.Balance = cfg.Balance
	g.ApplyRuleset(cfg.Ruleset)
	if cfg.Seed != 0 {
		g.SetRandomSeed(cfg.Seed)
	}
	g.PauseBetweenTurns = false
	g.AIDecisionInterval[g.Player1] = 0
	g.AIDecisionInterval[g.Player2] = 0

	maxTicks := cfg.MaxTicks
	if maxTicks <= 0 {
		maxTicks = 400
	}
	ticks := 0
	deadline := time.Now().Add(60 * time.Second)
	for ticks < maxTicks && !g.GameOver && time.Now().Before(deadline) {
		if g.AIThinking[g.Player1] || g.AIThinking[g.Player2] {
			g.HandleAIDecisions()
			time.Sleep(200 * time.Microsecond)
			continue
		}
		g.UpdateGameState()
		g.HandleAIDecisions()
		ticks++
	}
	return g.BuildMatchResult()
}
