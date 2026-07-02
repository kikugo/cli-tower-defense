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

func TestRejectionStreakTracksConsecutiveRejections(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Defender] = 0 // every invest below is unaffordable

	g.applyDecision(g.Defender, "defender", map[string]interface{}{"action": "invest"})
	g.applyDecision(g.Defender, "defender", map[string]interface{}{"action": "invest"})
	if got := g.RejectionStreak[g.Defender]; got != 2 {
		t.Fatalf("expected streak 2 after two rejections, got %d", got)
	}
	g.applyDecision(g.Defender, "defender", map[string]interface{}{"action": "save"})
	if got := g.RejectionStreak[g.Defender]; got != 0 {
		t.Fatalf("expected streak reset after applied action, got %d", got)
	}
}

func TestRejectionFeedbackLineEscalates(t *testing.T) {
	if got := rejectionFeedbackLine(map[string]interface{}{}); got != "" {
		t.Fatalf("expected empty feedback without rejections, got %q", got)
	}
	once := map[string]interface{}{
		"your_rejection_streak":     1,
		"your_last_rejected_reason": "rejected:occupied_by_tower",
	}
	got := rejectionFeedbackLine(once)
	if !strings.Contains(got, "REJECTED") || !strings.Contains(got, "occupied_by_tower") {
		t.Fatalf("expected single-rejection warning, got %q", got)
	}
	repeated := map[string]interface{}{
		"your_rejection_streak":     3,
		"your_last_rejected_reason": "rejected:occupied_by_tower",
	}
	got = rejectionFeedbackLine(repeated)
	if !strings.Contains(got, "3") || !strings.Contains(got, "MUST") {
		t.Fatalf("expected escalated warning with streak count, got %q", got)
	}
}

func TestTowerPromptCarriesRejectionFeedback(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Defender] = 0
	g.applyDecision(g.Defender, "defender", map[string]interface{}{"action": "invest"})

	state := g.getPlayerGameState(g.Defender, "defender")
	prompt := (&OpenAIHandler{}).createTowerPrompt(state)
	if !strings.Contains(prompt, "REJECTED") {
		t.Fatalf("expected rejection feedback in prompt:\n%s", prompt)
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
