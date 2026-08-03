package engine

func normalizeDecision(role string, decision map[string]interface{}) map[string]interface{} {
	if decision == nil {
		decision = map[string]interface{}{}
	}
	normalized := map[string]interface{}{}

	action, _ := decision["action"].(string)
	reason, _ := decision["reason"].(string)
	taunt, _ := decision["taunt"].(string)
	if reason == "" {
		reason = "No reasoning provided."
	}
	if taunt != "" {
		normalized["taunt"] = taunt
	}
	normalized["reason"] = reason

	switch role {
	case "defender":
		defenderAction, actionDefaulted := normalizeDefenderAction(action)
		normalized["action"] = defenderAction
		if actionDefaulted {
			markDecisionSource(normalized, SourceNormalizerDefault)
		}
		switch normalized["action"] {
		case "place":
			towerType, _ := decision["tower_type"].(string)
			if !isValidTowerType(towerType) {
				towerType = "basic"
				markDecisionSource(normalized, SourceNormalizerDefault)
			}
			normalized["tower_type"] = towerType
			pos, posDefaulted := normalizePosition(decision["position"], 2, 2)
			normalized["position"] = pos
			if posDefaulted {
				markDecisionSource(normalized, SourceNormalizerDefault)
			}
		case "upgrade":
			if id, ok := toIntFromAny(decision["tower_id"]); ok {
				normalized["tower_id"] = id
			} else {
				normalized["tower_id"] = -1
			}
		case "place_slow_zone":
			pos, posDefaulted := normalizePosition(decision["position"], -1, -1)
			normalized["position"] = pos
			if posDefaulted {
				markDecisionSource(normalized, SourceNormalizerDefault)
			}
		case "research":
			tech, _ := decision["tech"].(string)
			if !isValidResearchTech(tech) {
				tech = "economy"
				markDecisionSource(normalized, SourceNormalizerDefault)
			}
			normalized["tech"] = tech
		}
	default:
		attackerAction, actionDefaulted := normalizeAttackerAction(action)
		normalized["action"] = attackerAction
		if actionDefaulted {
			markDecisionSource(normalized, SourceNormalizerDefault)
		}
		switch normalized["action"] {
		case "spawn":
			enemyType, _ := decision["enemy_type"].(string)
			if !isValidEnemyType(enemyType) {
				enemyType = "basic"
				markDecisionSource(normalized, SourceNormalizerDefault)
			}
			normalized["enemy_type"] = enemyType
		case "ability":
			ability, _ := decision["ability"].(string)
			if !isValidAttackerAbility(ability) {
				ability = "surge"
				markDecisionSource(normalized, SourceNormalizerDefault)
			}
			normalized["ability"] = ability
		}
	}

	return normalized
}

// normalizeDefenderAction returns the validated action and whether the
// input had to be replaced with the "save" default. "save" itself is a
// legitimate, explicit choice -- not the default -- so it must be listed
// here too, or a model genuinely choosing to save would be wrongly tagged
// as a substitution.
func normalizeDefenderAction(action string) (string, bool) {
	switch action {
	case "place", "upgrade", "place_slow_zone", "research", "invest", "save":
		return action, false
	default:
		return "save", true
	}
}

// normalizeAttackerAction returns the validated action and whether the
// input had to be replaced with the "save" default. Same reasoning as
// normalizeDefenderAction: "save" is explicit, not a default.
func normalizeAttackerAction(action string) (string, bool) {
	switch action {
	case "spawn", "wave", "ability", "invest", "save":
		return action, false
	default:
		return "save", true
	}
}

// normalizePosition returns the resolved position and whether raw had to be
// replaced with (defaultY, defaultX) because it was missing or malformed.
func normalizePosition(raw interface{}, defaultY, defaultX int) ([]interface{}, bool) {
	y, x := parseDecisionPosition(raw, defaultY, defaultX)
	return []interface{}{float64(y), float64(x)}, !positionProvided(raw)
}

// positionProvided reports whether raw is a usable two-element position,
// without applying any default. Used only to detect (for provenance) when
// normalizePosition had to substitute its own value.
func positionProvided(raw interface{}) bool {
	pos, ok := raw.([]interface{})
	if !ok || len(pos) < 2 {
		return false
	}
	_, okY := toIntFromAny(pos[0])
	_, okX := toIntFromAny(pos[1])
	return okY && okX
}

func isValidTowerType(t string) bool {
	switch t {
	case "basic", "sniper", "splash", "buffer":
		return true
	default:
		return false
	}
}

func isValidEnemyType(t string) bool {
	switch t {
	case "basic", "fast", "tank", "shielded", "healer":
		return true
	default:
		return false
	}
}

func isValidResearchTech(t string) bool {
	switch t {
	case "economy", "range", "control":
		return true
	default:
		return false
	}
}

func isValidAttackerAbility(t string) bool {
	switch t {
	case "surge", "shield_burst", "reinforce_wave":
		return true
	default:
		return false
	}
}
