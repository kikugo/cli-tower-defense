package engine

import "fmt"

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
