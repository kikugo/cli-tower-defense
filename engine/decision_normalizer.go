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
		normalized["action"] = normalizeDefenderAction(action)
		switch normalized["action"] {
		case "place":
			towerType, _ := decision["tower_type"].(string)
			if !isValidTowerType(towerType) {
				towerType = "basic"
			}
			normalized["tower_type"] = towerType
			normalized["position"] = normalizePosition(decision["position"], 2, 2)
		case "upgrade":
			if id, ok := toIntFromAny(decision["tower_id"]); ok {
				normalized["tower_id"] = id
			} else {
				normalized["tower_id"] = -1
			}
		case "place_slow_zone":
			normalized["position"] = normalizePosition(decision["position"], -1, -1)
		}
	default:
		normalized["action"] = normalizeAttackerAction(action)
		switch normalized["action"] {
		case "spawn":
			enemyType, _ := decision["enemy_type"].(string)
			if !isValidEnemyType(enemyType) {
				enemyType = "basic"
			}
			normalized["enemy_type"] = enemyType
		}
	}

	return normalized
}

func normalizeDefenderAction(action string) string {
	switch action {
	case "place", "upgrade", "place_slow_zone", "invest":
		return action
	default:
		return "save"
	}
}

func normalizeAttackerAction(action string) string {
	switch action {
	case "spawn", "wave", "invest":
		return action
	default:
		return "save"
	}
}

func normalizePosition(raw interface{}, defaultY, defaultX int) []interface{} {
	y, x := parseDecisionPosition(raw, defaultY, defaultX)
	return []interface{}{float64(y), float64(x)}
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

