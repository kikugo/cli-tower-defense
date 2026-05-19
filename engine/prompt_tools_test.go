package engine

import (
	"strings"
	"testing"
)

func TestTowerPromptIncludesAllDefenderTools(t *testing.T) {
	h := &OpenAIHandler{}
	gameState := map[string]interface{}{
		"resources":      map[string]interface{}{"p1": 300},
		"income":         map[string]interface{}{"p1": 5},
		"wave":           1,
		"paths_count":    1,
		"active_enemies": 0,
		"wave_queue":     0,
		"lives":          map[string]interface{}{"p1": 20},
		"towers":         []interface{}{},
		"enemies":        []interface{}{},
	}

	prompt := h.createTowerPrompt(gameState)
	required := []string{
		`"action": "place"`,
		`"action": "upgrade"`,
		`"action": "place_slow_zone"`,
		`"action": "invest"`,
		`"action": "save"`,
		"Your available tools this turn:",
		"Current objective:",
		"Legal action schema:",
	}
	for _, needle := range required {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing: %s", needle)
		}
	}
}

func TestEnemyPromptIncludesAllAttackerTools(t *testing.T) {
	h := &GeminiHandler{}
	gameState := map[string]interface{}{
		"resources":      map[string]interface{}{"p2": 300},
		"income":         map[string]interface{}{"p2": 5},
		"wave":           1,
		"paths_count":    2,
		"active_enemies": 0,
		"wave_queue":     0,
		"lives":          map[string]interface{}{"p1": 20},
		"towers":         []interface{}{},
		"enemies":        []interface{}{},
	}

	prompt := h.createEnemyPrompt(gameState)
	required := []string{
		`"action": "spawn"`,
		`"action": "wave"`,
		`"action": "invest"`,
		`"action": "save"`,
		"Your available tools this turn:",
		"Current objective:",
		"Legal action schema:",
	}
	for _, needle := range required {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing: %s", needle)
		}
	}
}
