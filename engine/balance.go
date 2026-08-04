package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// TowerStat and EnemyStat hold the tunable numbers for one entity type.
type TowerStat struct {
	Damage   int `json:"damage"`
	Range    int `json:"range"`
	Cooldown int `json:"cooldown"`
	Cost     int `json:"cost"`
}

type EnemyStat struct {
	Health    int     `json:"health"`
	Speed     float64 `json:"speed"`
	Reward    int     `json:"reward"`
	SpawnCost int     `json:"spawn_cost"`
	Shield    int     `json:"shield"`
}

// BalanceConfig is the single source of truth for combat/economy numbers.
// Game logic, affordability, and prompt text all read from it.
type BalanceConfig struct {
	Version              string               `json:"version"`
	Towers               map[string]TowerStat `json:"towers"`
	Enemies              map[string]EnemyStat `json:"enemies"`
	BreachResourceBounty int                  `json:"breach_resource_bounty"`
	BreachScore          int                  `json:"breach_score"`
}

// placeableTowerTypes is the ordered set models may build (prompt schema).
var placeableTowerTypes = []string{"basic", "splash", "sniper", "buffer"}

// attackerEnemyTypes is the ordered set models may spawn.
var attackerEnemyTypes = []string{"basic", "fast", "healer", "shielded", "tank"}

// Display glyphs are presentation, not balance; they stay fixed.
var towerChars = map[string]rune{"basic": '^', "sniper": '⌖', "splash": '⊕', "buffer": 'B', "custom": '?'}
var enemyChars = map[string]rune{"basic": 'o', "fast": '>', "tank": '□', "shielded": 'S', "healer": 'H', "custom": '?'}

// v3 changes only the sniper: 50 damage / cooldown 5 / cost 100, from
// 50 / 15 / 250. At the shipped numbers a sniper-only defender could afford
// exactly one tower on a ~350 lifetime budget and lost 0/40 at tick 101 with
// a score of zero -- it delivered 1.3 damage/tick per 100 resources against
// basic's 17.0, so it was never a real option. v3 puts it at 10.0, still
// below basic, but its range of 12 covers both branches of a fork that
// basic's range of 5 cannot reach. Measured over 40 seeds it INVERTS the
// stratum pattern rather than dominating: 8% on one-lane maps where basic
// holds 100%, and 62% on two-lane maps where basic holds 6% -- and 69%
// against the live-calibrated attacker. Overall it stays worse than basic
// (30% against 62%), so basic remains the default buy and the sniper is a
// map-dependent alternative.
//
// The window is narrow: at 50 / cooldown 3 / cost 100 (16.7 damage/tick per
// 100, a hair under basic's 17.0, with range 12) the sniper wins 100% of
// every stratum and is simply a better basic. Do not tune toward that.
//
// splash and buffer were left alone deliberately. Neither could be made
// situational: splash given basic's exact stats plus a free three-target
// attack gains three points against attacker_baseline and nothing at all
// against a live-like one, because enemies never occupy the same
// neighbourhood; buffer is unaffordable at any price the defender can reach.
// See the readme for both.
//
// DefaultBalanceConfig returns the tuned numbers. v1's basic tower
// (15 dmg / 5 cooldown) could not kill a single 100 HP enemy per pass, making
// defense mathematically unwinnable (0% baseline hold rate). v2 raises the
// basic tower to 34 dmg / 2 cooldown, measured at a 72% hold rate for a
// competent scripted defender over 40 seeded random-map duels — an upper
// bound that live models undercut, putting real matches in contested range.
// Sweeps showed outcomes are bimodal per map layout, so the originally
// targeted 40-60% band does not exist on any knob axis; 72% is the nearest
// stable point above it. Tune via -balance-sweep; guarded by
// TestBaselineDuelBandWithDefaults.
func DefaultBalanceConfig() BalanceConfig {
	return BalanceConfig{
		Version: "v3",
		Towers: map[string]TowerStat{
			"basic":  {Damage: 34, Range: 5, Cooldown: 2, Cost: 100},
			"sniper": {Damage: 50, Range: 12, Cooldown: 5, Cost: 100},
			"splash": {Damage: 10, Range: 3, Cooldown: 3, Cost: 200},
			"buffer": {Damage: 0, Range: 2, Cooldown: 0, Cost: 300},
			"custom": {Damage: 20, Range: 7, Cooldown: 8, Cost: 150},
		},
		Enemies: map[string]EnemyStat{
			"basic":    {Health: 100, Speed: 1.0, Reward: 20, SpawnCost: 20},
			"fast":     {Health: 50, Speed: 2.0, Reward: 15, SpawnCost: 30},
			"tank":     {Health: 300, Speed: 0.5, Reward: 50, SpawnCost: 50},
			"shielded": {Health: 150, Speed: 0.8, Reward: 40, SpawnCost: 40, Shield: 2},
			"healer":   {Health: 80, Speed: 1.0, Reward: 30, SpawnCost: 30},
			"custom":   {Health: 150, Speed: 1.2, Reward: 25},
		},
		BreachResourceBounty: 30,
		BreachScore:          50,
	}
}

// ComputeBalanceHash returns a short, deterministic fingerprint of the
// numeric content of cfg: every tower stat, every enemy stat, the breach
// resource bounty, and the breach score. It deliberately excludes Version --
// a hand-written human label (see DefaultBalanceConfig) that can drift from
// the actual numbers, e.g. balance_sweep.go's applyBalanceOverride rewrites
// tower/enemy stats for a sweep candidate without ever touching Version.
// Two configs differing only in Version hash identically; two configs
// differing in any stat hash differently.
//
// Go randomises map iteration order per process, so cfg.Towers and
// cfg.Enemies are walked in sorted-key order before hashing -- without that,
// this would produce a different hash for the same config on different
// runs, which is worse than no hash at all. Uses SHA-256 (stdlib, no new
// dependency), truncated to the first 16 hex characters (64 bits) for
// legibility in manifests; that's plenty to detect accidental drift and
// collisions are not a concern here.
func ComputeBalanceHash(cfg BalanceConfig) string {
	var b strings.Builder

	towerNames := make([]string, 0, len(cfg.Towers))
	for name := range cfg.Towers {
		towerNames = append(towerNames, name)
	}
	sort.Strings(towerNames)
	for _, name := range towerNames {
		st := cfg.Towers[name]
		fmt.Fprintf(&b, "tower:%s:%d:%d:%d:%d\n", name, st.Damage, st.Range, st.Cooldown, st.Cost)
	}

	enemyNames := make([]string, 0, len(cfg.Enemies))
	for name := range cfg.Enemies {
		enemyNames = append(enemyNames, name)
	}
	sort.Strings(enemyNames)
	for _, name := range enemyNames {
		st := cfg.Enemies[name]
		fmt.Fprintf(&b, "enemy:%s:%d:%g:%d:%d:%d\n", name, st.Health, st.Speed, st.Reward, st.SpawnCost, st.Shield)
	}

	fmt.Fprintf(&b, "breach_resource_bounty:%d\n", cfg.BreachResourceBounty)
	fmt.Fprintf(&b, "breach_score:%d\n", cfg.BreachScore)

	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:16]
}

func (g *Game) towerCost(name string) (int, bool) {
	st, ok := g.Balance.Towers[name]
	if !ok {
		return 0, false
	}
	return st.Cost, true
}

func (g *Game) spawnCost(name string) (int, bool) {
	st, ok := g.Balance.Enemies[name]
	if !ok || st.SpawnCost <= 0 {
		return 0, false
	}
	return st.SpawnCost, true
}

// newTower builds a tower from the game's balance config. "custom" params
// may override individual fields (legacy behavior).
func (g *Game) newTower(y, x int, towerType string, params map[string]interface{}) Tower {
	st := g.Balance.Towers[towerType]
	if towerType == "custom" && params != nil {
		if v, ok := params["damage"]; ok {
			st.Damage = toInt(v)
		}
		if v, ok := params["range"]; ok {
			st.Range = toInt(v)
		}
		if v, ok := params["cooldown"]; ok {
			st.Cooldown = toInt(v)
		}
		if v, ok := params["cost"]; ok {
			st.Cost = toInt(v)
		}
	}
	char, ok := towerChars[towerType]
	if !ok {
		char = '^'
	}
	return Tower{
		Entity: Entity{
			Pos: Position{Y: y, X: x}, Char: char,
			Health: 100, MaxHealth: 100,
			Damage: st.Damage, Cooldown: 0, MaxCD: st.Cooldown,
		},
		TowerType: towerType, Range: st.Range, Cost: st.Cost, Strategy: "nearest",
	}
}

// newEnemy builds an enemy from the game's balance config.
func (g *Game) newEnemy(y, x int, enemyType string, params map[string]interface{}) Enemy {
	st := g.Balance.Enemies[enemyType]
	if enemyType == "custom" && params != nil {
		if v, ok := params["health"]; ok {
			st.Health = toInt(v)
		}
		if v, ok := params["speed"].(float64); ok {
			st.Speed = v
		}
		if v, ok := params["reward"]; ok {
			st.Reward = toInt(v)
		}
		if v, ok := params["shield"]; ok {
			st.Shield = toInt(v)
		}
	}
	char, ok := enemyChars[enemyType]
	if !ok {
		char = '?'
	}
	return Enemy{
		Entity: Entity{
			Pos: Position{Y: y, X: x}, Char: char,
			Health: st.Health, MaxHealth: st.Health,
		},
		EnemyType: enemyType, Speed: st.Speed, Reward: st.Reward, Shield: st.Shield,
	}
}
