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
		return scriptedDefenderBuild(gameState, "basic", "baseline"), nil
	case "defender_sniper":
		// Same build-coverage/upgrade/save logic as defender_baseline, but
		// commits exclusively to the sniper tower, so a sweep against it
		// isolates that tower's cost-efficiency from placement strategy.
		return scriptedDefenderBuild(gameState, "sniper", "sniper"), nil
	case "defender_splash":
		return scriptedDefenderBuild(gameState, "splash", "splash"), nil
	case "defender_buffer":
		return scriptedDefenderBuild(gameState, "buffer", "buffer"), nil
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
	case "attacker_shielded":
		// Spawns shielded (never basic/fast) whenever affordable, so a sweep
		// against this script isolates the shielded enemy's cost-efficiency
		// from the mixed-composition income cap that attacker_baseline is
		// bound by. Saves when shielded isn't affordable yet, rather than
		// falling back to a cheaper unit.
		affordable, _ := gameState["affordable_actions"].([]string)
		for _, action := range affordable {
			if action == "spawn:shielded" {
				return map[string]interface{}{"action": "spawn", "enemy_type": "shielded", "reason": "scripted: shielded"}, nil
			}
		}
		return map[string]interface{}{"action": "save", "reason": "scripted: save"}, nil
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

// scriptedDefenderBuild spreads towers of towerType along the path while
// placement is affordable, upgrades an existing tower once the board
// saturates, and saves when broke. Shared by defender_baseline and the
// single-tower-type sweep scripts (defender_sniper, defender_splash,
// defender_buffer) so they differ only in which tower they commit to.
func scriptedDefenderBuild(gameState map[string]interface{}, towerType, label string) map[string]interface{} {
	affordable, _ := gameState["affordable_actions"].([]string)
	canPlace := false
	upgradeID := -1
	placeAction := "place:" + towerType
	for _, action := range affordable {
		if action == placeAction {
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
				"action": "place", "tower_type": towerType,
				"position": []interface{}{float64(candidates[idx][0]), float64(candidates[idx][1])},
				"reason":   label + ": build coverage",
			}
		}
	}
	if upgradeID >= 0 {
		return map[string]interface{}{"action": "upgrade", "tower_id": float64(upgradeID), "reason": label + ": strengthen"}
	}
	return map[string]interface{}{"action": "save", "reason": label + ": save"}
}
