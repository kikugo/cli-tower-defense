package engine

import (
	"strings"
	"testing"
)

func TestFormatAffordableActions(t *testing.T) {
	state := map[string]interface{}{"affordable_actions": []string{"save"}}
	if got := formatAffordableActions(state); got != "save (nothing else is affordable this turn)" {
		t.Fatalf("got %q", got)
	}
	state["affordable_actions"] = []string{"save", "spawn:basic", "wave"}
	if got := formatAffordableActions(state); got != "save, spawn:basic, wave" {
		t.Fatalf("got %q", got)
	}
	if got := formatAffordableActions(map[string]interface{}{}); got != "save" {
		t.Fatalf("missing key should degrade to save, got %q", got)
	}
}

func TestTowerPromptListsAffordableActions(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Defender] = 50
	state := g.getPlayerGameState(g.Defender, "defender")
	prompt := (&OpenAIHandler{}).createTowerPrompt(state)
	if !strings.Contains(prompt, "You can currently afford ONLY these actions:") {
		t.Fatalf("prompt missing affordability line:\n%s", prompt)
	}
	if !strings.Contains(prompt, "nothing else is affordable") {
		t.Fatalf("broke defender should see the only-save note:\n%s", prompt)
	}
}

func TestEnemyPromptListsAffordableActions(t *testing.T) {
	g := NewGame("test", "test")
	state := g.getPlayerGameState(g.Attacker, "attacker")
	prompt := (&GeminiHandler{}).createEnemyPrompt(state)
	if !strings.Contains(prompt, "You can currently afford ONLY these actions:") {
		t.Fatalf("prompt missing affordability line:\n%s", prompt)
	}
}

func TestParseTowerResponseRespectsSave(t *testing.T) {
	h := &OpenAIHandler{}
	got, err := h.parseTowerResponse(`{"action":"save","reason":"cannot afford anything"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["action"] != "save" {
		t.Fatalf("expected save preserved, got %v", got["action"])
	}
}
