package engine

import (
	"strings"
	"testing"
)

func TestAttackerMenuHidesUnaffordableSchemas(t *testing.T) {
	state := map[string]interface{}{
		"affordable_actions": []string{"save"},
		"wave":               0,
	}
	menu := buildAttackerActionMenu(state)
	if !strings.Contains(menu, `"action": "save"`) {
		t.Fatalf("menu missing save template:\n%s", menu)
	}
	if strings.Contains(menu, `"action": "spawn"`) {
		t.Fatalf("broke attacker must not see a spawn template:\n%s", menu)
	}
	if !strings.Contains(menu, "NOT affordable yet") {
		t.Fatalf("menu missing unaffordable guidance:\n%s", menu)
	}
	if !strings.Contains(menu, "basic (20)") || !strings.Contains(menu, "wave (40)") {
		t.Fatalf("unaffordable line should price what to save toward:\n%s", menu)
	}
}

func TestAttackerMenuShowsAffordableSpawnTypesOnly(t *testing.T) {
	state := map[string]interface{}{
		"affordable_actions": []string{"save", "spawn:basic", "spawn:fast", "wave"},
		"wave":               2,
	}
	menu := buildAttackerActionMenu(state)
	if !strings.Contains(menu, `"enemy_type": "basic|fast"`) {
		t.Fatalf("spawn template should list only affordable types:\n%s", menu)
	}
	if !strings.Contains(menu, `"action": "wave"`) {
		t.Fatalf("wave template missing when affordable:\n%s", menu)
	}
	if strings.Contains(menu, `"action": "ability"`) {
		t.Fatalf("ability template must be hidden when not affordable:\n%s", menu)
	}
}

func TestDefenderMenuFiltersTowerTypes(t *testing.T) {
	state := map[string]interface{}{
		"affordable_actions": []string{"save", "place:basic", "place:splash", "invest"},
	}
	menu := buildDefenderActionMenu(state)
	if !strings.Contains(menu, `"tower_type": "basic|splash"`) {
		t.Fatalf("place template should list only affordable types:\n%s", menu)
	}
	if !strings.Contains(menu, "sniper tower (250)") {
		t.Fatalf("unaffordable sniper should be priced in guidance:\n%s", menu)
	}
	if strings.Contains(menu, `"action": "research"`) {
		t.Fatalf("research template must be hidden when unaffordable:\n%s", menu)
	}
}

func TestDefenderMenuListsAffordableUpgradeIDs(t *testing.T) {
	state := map[string]interface{}{
		"affordable_actions": []string{"save", "upgrade:2", "upgrade:5"},
	}
	menu := buildDefenderActionMenu(state)
	if !strings.Contains(menu, `"action": "upgrade"`) {
		t.Fatalf("upgrade template missing:\n%s", menu)
	}
	if !strings.Contains(menu, "2, 5") {
		t.Fatalf("affordable upgrade ids should be listed:\n%s", menu)
	}
}

func TestMenusFallBackToFullSchemaWithoutAffordability(t *testing.T) {
	// Callers that build synthetic states (tests, tools) get every schema.
	def := buildDefenderActionMenu(map[string]interface{}{})
	att := buildAttackerActionMenu(map[string]interface{}{"wave": 1})
	for _, needle := range []string{`"action": "place"`, `"action": "upgrade"`, `"action": "research"`} {
		if !strings.Contains(def, needle) {
			t.Fatalf("defender fallback missing %s:\n%s", needle, def)
		}
	}
	for _, needle := range []string{`"action": "spawn"`, `"action": "wave"`, `"action": "ability"`} {
		if !strings.Contains(att, needle) {
			t.Fatalf("attacker fallback missing %s:\n%s", needle, att)
		}
	}
}

func TestMenusHonorSuppressedAction(t *testing.T) {
	state := map[string]interface{}{
		"affordable_actions": []string{"save", "place:basic"},
		"suppressed_action":  "place",
	}
	menu := buildDefenderActionMenu(state)
	if strings.Contains(menu, `"action": "place"`) {
		t.Fatalf("suppressed action family must be hidden:\n%s", menu)
	}
	if !strings.Contains(menu, "BLOCKED") {
		t.Fatalf("menu should explain the suppression:\n%s", menu)
	}
}

func TestLivePromptsUseMenus(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Attacker] = 10 // broke: below every spawn cost
	prompt := (&GeminiHandler{}).createEnemyPrompt(g.getPlayerGameState(g.Attacker, "attacker"))
	if strings.Contains(prompt, `"action": "spawn"`) {
		t.Fatalf("live broke attacker still sees spawn schema:\n%s", prompt)
	}
	if !strings.Contains(prompt, "NOT affordable yet") {
		t.Fatalf("live prompt missing unaffordable guidance:\n%s", prompt)
	}
}
