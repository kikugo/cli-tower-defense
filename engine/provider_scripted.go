package engine

import (
	"fmt"
	"strconv"
	"strings"
)

type ScriptedProvider struct {
	config ResolvedPlayerModelConfig
}

func NewScriptedProvider(config ResolvedPlayerModelConfig) *ScriptedProvider {
	return &ScriptedProvider{config: config}
}

func (p *ScriptedProvider) Name() string {
	return fmt.Sprintf("%s/%s", p.config.Provider, p.config.Model)
}

func (p *ScriptedProvider) GetTowerDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	switch p.config.Model {
	case "defender_invest":
		return map[string]interface{}{"action": "invest", "reason": "scripted"}, nil
	case "defender_baseline":
		// Competent-but-simple defense: build coverage while placement is
		// affordable, upgrade once the board saturates, save when broke.
		affordable, _ := gameState["affordable_actions"].([]string)
		canPlace := false
		upgradeID := -1
		for _, action := range affordable {
			if action == "place:basic" {
				canPlace = true
			}
			if upgradeID < 0 && strings.HasPrefix(action, "upgrade:") {
				if id, err := strconv.Atoi(strings.TrimPrefix(action, "upgrade:")); err == nil {
					upgradeID = id
				}
			}
		}
		if canPlace {
			if candidates, ok := gameState["valid_tower_candidates"].([][]int); ok && len(candidates) > 0 {
				// Spread towers along the path instead of clustering at the
				// entrance: stride through the candidate list by how many
				// towers already exist.
				built := 0
				if towers, ok := gameState["towers"].([]interface{}); ok {
					built = len(towers)
				}
				idx := (built * 3) % len(candidates)
				return map[string]interface{}{
					"action": "place", "tower_type": "basic",
					"position": []interface{}{float64(candidates[idx][0]), float64(candidates[idx][1])},
					"reason":   "baseline: build coverage",
				}, nil
			}
		}
		if upgradeID >= 0 {
			return map[string]interface{}{"action": "upgrade", "tower_id": float64(upgradeID), "reason": "baseline: strengthen"}, nil
		}
		return map[string]interface{}{"action": "save", "reason": "baseline: save"}, nil
	default:
		if candidates, ok := gameState["valid_tower_candidates"].([][]int); ok && len(candidates) > 0 {
			return map[string]interface{}{
				"action":     "place",
				"tower_type": "basic",
				"position":   []interface{}{float64(candidates[0][0]), float64(candidates[0][1])},
				"reason":     "scripted",
			}, nil
		}
		return map[string]interface{}{"action": "save", "reason": "scripted"}, nil
	}
}

func (p *ScriptedProvider) GetEnemyDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	switch p.config.Model {
	case "attacker_spawn":
		return map[string]interface{}{"action": "spawn", "enemy_type": "basic", "reason": "scripted"}, nil
	default:
		if resources, ok := gameState["resources"].(map[string]interface{}); ok {
			for _, v := range resources {
				if r, ok := toIntFromAny(v); ok && r >= 260 {
					return map[string]interface{}{"action": "wave", "reason": "scripted"}, nil
				}
			}
		}
		return map[string]interface{}{"action": "spawn", "enemy_type": "basic", "reason": "scripted"}, nil
	}
}
