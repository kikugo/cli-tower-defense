package engine

import "fmt"

// attackerSpawnCosts is the single source of truth for spawn pricing, shared
// by spawnEnemy and affordability reporting.
var attackerSpawnCosts = map[string]int{
	"basic": 20, "fast": 30, "tank": 50, "shielded": 40, "healer": 30,
}

// defenderTowerCosts mirrors NewTower's cost table for affordability checks.
var defenderTowerCosts = map[string]int{
	"basic": 100, "custom": 150, "splash": 200, "sniper": 250, "buffer": 300,
}

var defenderResearchCosts = map[string]int{
	"economy": 180, "range": 160, "control": 140,
}

// waveCostForWave is the single source of truth for wave pricing.
func waveCostForWave(wave int) int {
	cost := 40 + wave*5
	if cost > 200 {
		cost = 200
	}
	return cost
}

// affordableActions lists the actions a player can legally attempt right now,
// in deterministic order, so prompts can steer models away from rejected
// moves. "save" is always legal.
func (g *Game) affordableActions(playerID, role string) []string {
	actions := []string{"save"}
	res := g.Resources[playerID]

	if role == "defender" {
		for _, name := range []string{"basic", "custom", "splash", "sniper", "buffer"} {
			if res >= defenderTowerCosts[name] {
				actions = append(actions, "place:"+name)
			}
		}
		for id, tower := range g.Towers {
			if res >= 150*(tower.Level+1) {
				actions = append(actions, fmt.Sprintf("upgrade:%d", id))
			}
		}
		if res >= 150 {
			actions = append(actions, "place_slow_zone")
		}
		for _, tech := range []string{"economy", "range", "control"} {
			if g.ResearchLevels[tech] < 2 && res >= defenderResearchCosts[tech] {
				actions = append(actions, "research:"+tech)
			}
		}
		if res >= 150 {
			actions = append(actions, "invest")
		}
		return actions
	}

	for _, name := range []string{"basic", "fast", "healer", "shielded", "tank"} {
		if res >= attackerSpawnCosts[name] {
			actions = append(actions, "spawn:"+name)
		}
	}
	if res >= waveCostForWave(g.Wave) && g.Wave < g.MaxWaves {
		actions = append(actions, "wave")
	}
	for _, ability := range availableAttackerAbilities() {
		if g.AbilityCooldowns[ability.Name] == 0 && res >= ability.Cost {
			actions = append(actions, "ability:"+ability.Name)
		}
	}
	if res >= 150 {
		actions = append(actions, "invest")
	}
	return actions
}
