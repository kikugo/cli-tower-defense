package engine

import (
	"fmt"
	"strings"
)

// Action menus render ONLY the JSON templates a player can currently afford.
// Models copy the schemas they see: showing a broke attacker the spawn
// template produced hundreds of consecutive unaffordable spawns in live
// matches. Unaffordable actions are named with prices so models can save
// toward them. When affordable_actions is absent (synthetic states), the
// full schema set is shown.

func affordableSetFromState(gameState map[string]interface{}) ([]string, bool) {
	affordable, ok := gameState["affordable_actions"].([]string)
	return affordable, ok
}

func suppressedActionFromState(gameState map[string]interface{}) string {
	s, _ := gameState["suppressed_action"].(string)
	return s
}

// buildDefenderActionMenu renders the defender's affordable action templates.
func buildDefenderActionMenu(gameState map[string]interface{}) string {
	affordable, filtered := affordableSetFromState(gameState)
	suppressed := suppressedActionFromState(gameState)

	placeTypes := []string{}
	upgradeIDs := []string{}
	researchTechs := []string{}
	hasSlow, hasInvest := !filtered, !filtered
	if filtered {
		for _, a := range affordable {
			switch {
			case strings.HasPrefix(a, "place:"):
				placeTypes = append(placeTypes, strings.TrimPrefix(a, "place:"))
			case strings.HasPrefix(a, "upgrade:"):
				upgradeIDs = append(upgradeIDs, strings.TrimPrefix(a, "upgrade:"))
			case strings.HasPrefix(a, "research:"):
				researchTechs = append(researchTechs, strings.TrimPrefix(a, "research:"))
			case a == "place_slow_zone":
				hasSlow = true
			case a == "invest":
				hasInvest = true
			}
		}
	} else {
		placeTypes = append(placeTypes, placeableTowerTypes...)
		upgradeIDs = append(upgradeIDs, "<id>")
		researchTechs = append(researchTechs, "economy", "range", "control")
	}
	if suppressed != "" {
		placeTypes = filterSuppressed(placeTypes, suppressed == "place")
		upgradeIDs = filterSuppressed(upgradeIDs, suppressed == "upgrade")
		researchTechs = filterSuppressed(researchTechs, suppressed == "research")
		if suppressed == "place_slow_zone" {
			hasSlow = false
		}
		if suppressed == "invest" {
			hasInvest = false
		}
	}

	var b strings.Builder
	b.WriteString("Choose EXACTLY ONE of these currently affordable actions:\n")
	n := 1
	if len(placeTypes) > 0 {
		costs := make([]string, 0, len(placeTypes))
		for _, tt := range placeTypes {
			costs = append(costs, fmt.Sprintf("%s (%d)", tt, promptCost(gameState, "tower_costs", tt)))
		}
		fmt.Fprintf(&b, "%d. {\"action\": \"place\", \"tower_type\": \"%s\", \"position\": [y, x], \"reason\": \"...\", \"taunt\": \"...\"}\n", n, strings.Join(placeTypes, "|"))
		fmt.Fprintf(&b, "   Costs: %s. Position must be inside map, not on path, obstacle, or another tower.\n", strings.Join(costs, ", "))
		n++
	}
	if len(upgradeIDs) > 0 {
		fmt.Fprintf(&b, "%d. {\"action\": \"upgrade\", \"tower_id\": <int>, \"reason\": \"...\", \"taunt\": \"...\"}\n", n)
		fmt.Fprintf(&b, "   Cost: 150 * (current_level + 1). Affordable tower ids: %s.\n", strings.Join(upgradeIDs, ", "))
		n++
	}
	if hasSlow {
		fmt.Fprintf(&b, "%d. {\"action\": \"place_slow_zone\", \"position\": [y, x], \"reason\": \"...\", \"taunt\": \"...\"}\n", n)
		fmt.Fprintf(&b, "   Cost: 150. Reduces enemy speed by 50%%. MUST be on a path.\n")
		n++
	}
	if len(researchTechs) > 0 {
		costs := make([]string, 0, len(researchTechs))
		for _, tech := range researchTechs {
			if cost, ok := defenderResearchCosts[tech]; ok {
				costs = append(costs, fmt.Sprintf("%s (%d)", tech, cost))
			} else {
				costs = append(costs, tech)
			}
		}
		fmt.Fprintf(&b, "%d. {\"action\": \"research\", \"tech\": \"%s\", \"reason\": \"...\", \"taunt\": \"...\"}\n", n, strings.Join(researchTechs, "|"))
		fmt.Fprintf(&b, "   Costs: %s. Unlocks persistent defender bonuses.\n", strings.Join(costs, ", "))
		n++
	}
	if hasInvest {
		fmt.Fprintf(&b, "%d. {\"action\": \"invest\", \"reason\": \"...\", \"taunt\": \"...\"}\n", n)
		fmt.Fprintf(&b, "   Cost: 150. Permanently increases passive income.\n")
		n++
	}
	fmt.Fprintf(&b, "%d. {\"action\": \"save\", \"reason\": \"...\", \"taunt\": \"...\"}\n", n)

	if filtered {
		missing := defenderUnaffordable(gameState, placeTypes, researchTechs, hasSlow, hasInvest)
		if missing != "" {
			fmt.Fprintf(&b, "NOT affordable yet (do NOT choose these; save or invest toward them): %s\n", missing)
		}
	}
	if suppressed != "" {
		fmt.Fprintf(&b, "BLOCKED after repeated rejections: %q — you must choose a different action this turn.\n", suppressed)
	}
	return b.String()
}

// buildAttackerActionMenu renders the attacker's affordable action templates.
func buildAttackerActionMenu(gameState map[string]interface{}) string {
	affordable, filtered := affordableSetFromState(gameState)
	suppressed := suppressedActionFromState(gameState)
	wave, _ := gameState["wave"].(int)
	waveCost := waveCostForWave(wave)

	spawnTypes := []string{}
	abilities := []string{}
	hasWave, hasInvest := !filtered, !filtered
	if filtered {
		for _, a := range affordable {
			switch {
			case strings.HasPrefix(a, "spawn:"):
				spawnTypes = append(spawnTypes, strings.TrimPrefix(a, "spawn:"))
			case strings.HasPrefix(a, "ability:"):
				abilities = append(abilities, strings.TrimPrefix(a, "ability:"))
			case a == "wave":
				hasWave = true
			case a == "invest":
				hasInvest = true
			}
		}
	} else {
		spawnTypes = append(spawnTypes, attackerEnemyTypes...)
		for _, spec := range availableAttackerAbilities() {
			abilities = append(abilities, spec.Name)
		}
	}
	if suppressed != "" {
		spawnTypes = filterSuppressed(spawnTypes, suppressed == "spawn")
		abilities = filterSuppressed(abilities, suppressed == "ability")
		if suppressed == "wave" {
			hasWave = false
		}
		if suppressed == "invest" {
			hasInvest = false
		}
	}

	var b strings.Builder
	b.WriteString("Choose EXACTLY ONE of these currently affordable actions:\n")
	n := 1
	if len(spawnTypes) > 0 {
		costs := make([]string, 0, len(spawnTypes))
		for _, et := range spawnTypes {
			costs = append(costs, fmt.Sprintf("%s (%d)", et, promptCost(gameState, "spawn_costs", et)))
		}
		fmt.Fprintf(&b, "%d. {\"action\": \"spawn\", \"enemy_type\": \"%s\", \"reason\": \"...\", \"taunt\": \"...\"}\n", n, strings.Join(spawnTypes, "|"))
		fmt.Fprintf(&b, "   Costs: %s.\n", strings.Join(costs, ", "))
		n++
	}
	if hasWave {
		fmt.Fprintf(&b, "%d. {\"action\": \"wave\", \"reason\": \"...\", \"taunt\": \"...\"}\n", n)
		fmt.Fprintf(&b, "   Cost: %d. Massive multi-path assault — usually your strongest play.\n", waveCost)
		n++
	}
	if len(abilities) > 0 {
		specs := map[string]AbilitySpec{}
		for _, spec := range availableAttackerAbilities() {
			specs[spec.Name] = spec
		}
		costs := make([]string, 0, len(abilities))
		for _, name := range abilities {
			if spec, ok := specs[name]; ok {
				costs = append(costs, fmt.Sprintf("%s (%d/%d)", name, spec.Cost, spec.Cooldown))
			} else {
				costs = append(costs, name)
			}
		}
		fmt.Fprintf(&b, "%d. {\"action\": \"ability\", \"ability\": \"%s\", \"reason\": \"...\", \"taunt\": \"...\"}\n", n, strings.Join(abilities, "|"))
		fmt.Fprintf(&b, "   Costs/cooldowns: %s.\n", strings.Join(costs, ", "))
		n++
	}
	if hasInvest {
		fmt.Fprintf(&b, "%d. {\"action\": \"invest\", \"reason\": \"...\", \"taunt\": \"...\"}\n", n)
		fmt.Fprintf(&b, "   Cost: 150. Permanently increases passive income.\n")
		n++
	}
	fmt.Fprintf(&b, "%d. {\"action\": \"save\", \"reason\": \"...\", \"taunt\": \"...\"}\n", n)

	if filtered {
		missing := attackerUnaffordable(gameState, spawnTypes, hasWave, hasInvest, waveCost)
		if missing != "" {
			fmt.Fprintf(&b, "NOT affordable yet (do NOT choose these; save or invest toward them): %s\n", missing)
		}
	}
	if suppressed != "" {
		fmt.Fprintf(&b, "BLOCKED after repeated rejections: %q — you must choose a different action this turn.\n", suppressed)
	}
	return b.String()
}

func filterSuppressed(items []string, suppress bool) []string {
	if suppress {
		return nil
	}
	return items
}

func defenderUnaffordable(gameState map[string]interface{}, placeTypes, researchTechs []string, hasSlow, hasInvest bool) string {
	have := map[string]bool{}
	for _, tt := range placeTypes {
		have[tt] = true
	}
	missing := []string{}
	for _, tt := range placeableTowerTypes {
		if !have[tt] {
			missing = append(missing, fmt.Sprintf("%s tower (%d)", tt, promptCost(gameState, "tower_costs", tt)))
		}
	}
	haveTech := map[string]bool{}
	for _, tech := range researchTechs {
		haveTech[tech] = true
	}
	for _, tech := range []string{"economy", "range", "control"} {
		if !haveTech[tech] {
			missing = append(missing, fmt.Sprintf("research %s (%d)", tech, defenderResearchCosts[tech]))
		}
	}
	if !hasSlow {
		missing = append(missing, "slow zone (150)")
	}
	if !hasInvest {
		missing = append(missing, "invest (150)")
	}
	return strings.Join(missing, ", ")
}

func attackerUnaffordable(gameState map[string]interface{}, spawnTypes []string, hasWave, hasInvest bool, waveCost int) string {
	have := map[string]bool{}
	for _, et := range spawnTypes {
		have[et] = true
	}
	missing := []string{}
	for _, et := range attackerEnemyTypes {
		if !have[et] {
			missing = append(missing, fmt.Sprintf("spawn %s (%d)", et, promptCost(gameState, "spawn_costs", et)))
		}
	}
	if !hasWave {
		missing = append(missing, fmt.Sprintf("wave (%d)", waveCost))
	}
	if !hasInvest {
		missing = append(missing, "invest (150)")
	}
	return strings.Join(missing, ", ")
}
