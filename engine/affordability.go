package engine

import "fmt"

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
		// Only advertise tower placement when a legal cell actually exists;
		// on a saturated board "place" would be guaranteed to be rejected.
		if len(g.validTowerCandidates(1)) > 0 {
			for _, name := range placeableTowerTypes {
				if cost, ok := g.towerCost(name); ok && res >= cost {
					actions = append(actions, "place:"+name)
				}
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

	for _, name := range attackerEnemyTypes {
		if cost, ok := g.spawnCost(name); ok && res >= cost {
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
