package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	eng "tower-defense/engine"
)

type balanceOverride struct {
	Towers               map[string]eng.TowerStat `json:"towers"`
	Enemies              map[string]eng.EnemyStat `json:"enemies"`
	BreachResourceBounty *int                     `json:"breach_resource_bounty"`
	BreachScore          *int                     `json:"breach_score"`
}

type balanceSweepCandidate struct {
	Name    string          `json:"name"`
	Balance balanceOverride `json:"balance"`
}

type balanceSweepConfig struct {
	Seeds          []int64                 `json:"seeds"`
	MaxTicks       int                     `json:"max_ticks"`
	Ruleset        *eng.ArenaRuleset       `json:"ruleset"`
	DefenderScript string                  `json:"defender_script"`
	AttackerScript string                  `json:"attacker_script"`
	Candidates     []balanceSweepCandidate `json:"candidates"`
}

// applyBalanceOverride overlays a candidate's overrides on a copy of base.
// Tower/enemy overrides replace the WHOLE stat for that type (no field merge).
func applyBalanceOverride(base eng.BalanceConfig, o balanceOverride) eng.BalanceConfig {
	out := base
	out.Towers = make(map[string]eng.TowerStat, len(base.Towers))
	for k, v := range base.Towers {
		out.Towers[k] = v
	}
	out.Enemies = make(map[string]eng.EnemyStat, len(base.Enemies))
	for k, v := range base.Enemies {
		out.Enemies[k] = v
	}
	for k, v := range o.Towers {
		out.Towers[k] = v
	}
	for k, v := range o.Enemies {
		out.Enemies[k] = v
	}
	if o.BreachResourceBounty != nil {
		out.BreachResourceBounty = *o.BreachResourceBounty
	}
	if o.BreachScore != nil {
		out.BreachScore = *o.BreachScore
	}
	return out
}

func runBalanceSweep(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg balanceSweepConfig
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return fmt.Errorf("parse balance sweep config %s: %w", path, err)
	}
	if len(cfg.Seeds) == 0 {
		cfg.Seeds = []int64{1, 2, 3, 4, 5, 6, 7, 8}
	}
	if cfg.MaxTicks <= 0 {
		cfg.MaxTicks = 400
	}
	if cfg.DefenderScript == "" {
		cfg.DefenderScript = "defender_baseline"
	}
	if cfg.AttackerScript == "" {
		// The default scripted attacker launches waves when resources allow,
		// giving the defender a real wave-clear victory path.
		cfg.AttackerScript = "attacker_baseline"
	}
	ruleset := eng.BaselineDuelRuleset()
	if cfg.Ruleset != nil {
		ruleset = *cfg.Ruleset
	}

	fmt.Printf("%-24s | %-8s | %-8s | %-9s | %s\n", "candidate", "def wins", "win rate", "avg ticks", "avg def score")
	for _, cand := range cfg.Candidates {
		balance := applyBalanceOverride(eng.DefaultBalanceConfig(), cand.Balance)
		wins, totalTicks, totalScore := 0, int64(0), 0
		for _, seed := range cfg.Seeds {
			result := eng.RunScriptedDuel(eng.ScriptedDuelConfig{
				Seed: seed, MaxTicks: cfg.MaxTicks, Ruleset: ruleset, Balance: balance,
				DefenderScript: cfg.DefenderScript, AttackerScript: cfg.AttackerScript,
			})
			if result.DefenderHeld() {
				wins++
			}
			totalTicks += result.Ticks
			totalScore += result.Score[result.Defender]
		}
		n := len(cfg.Seeds)
		fmt.Printf("%-24s | %2d/%-5d | %7.0f%% | %9.1f | %.1f\n",
			cand.Name, wins, n, float64(wins)/float64(n)*100,
			float64(totalTicks)/float64(n), float64(totalScore)/float64(n))
	}
	return nil
}
