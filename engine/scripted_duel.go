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

// BaselineDuelRuleset is the standard balance-measurement arena: the real
// default economy (300 resources, income 5, 20 lives) on seeded random maps,
// assists off, capped at 5 waves so duels resolve quickly.
func BaselineDuelRuleset() ArenaRuleset {
	rs := DefaultArenaRuleset()
	rs.Name = "baseline-duel"
	rs.MapType = "" // seeded random maps: layout variance is the point
	rs.MaxWaves = 5
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
			// Scripted providers respond instantly; this only yields to the
			// worker goroutine, so keep it tiny (200µs here made sleeps the
			// dominant cost of whole sweeps).
			time.Sleep(10 * time.Microsecond)
			continue
		}
		g.UpdateGameState()
		g.HandleAIDecisions()
		ticks++
	}
	g.ResolveTimeout()
	return g.BuildMatchResult()
}
