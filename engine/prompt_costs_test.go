package engine

import (
	"strings"
	"testing"
)

func TestTowerPromptUsesBalanceCosts(t *testing.T) {
	g := NewGame("test", "test")
	st := g.Balance.Towers["basic"]
	st.Cost = 123
	g.Balance.Towers["basic"] = st
	g.Resources[g.Defender] = 500
	prompt := (&OpenAIHandler{}).createTowerPrompt(g.getPlayerGameState(g.Defender, "defender"))
	if !strings.Contains(prompt, "basic (123)") {
		t.Fatalf("expected prompt to carry balance cost 123:\n%s", prompt)
	}
}

func TestEnemyPromptUsesBalanceCosts(t *testing.T) {
	g := NewGame("test", "test")
	st := g.Balance.Enemies["tank"]
	st.SpawnCost = 77
	g.Balance.Enemies["tank"] = st
	prompt := (&GeminiHandler{}).createEnemyPrompt(g.getPlayerGameState(g.Attacker, "attacker"))
	if !strings.Contains(prompt, "tank (77)") {
		t.Fatalf("expected prompt to carry balance spawn cost 77:\n%s", prompt)
	}
}
