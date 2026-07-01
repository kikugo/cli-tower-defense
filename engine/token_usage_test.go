package engine

import "testing"

func TestExtractOpenAIUsage(t *testing.T) {
	result := map[string]interface{}{
		"usage": map[string]interface{}{
			"prompt_tokens":     float64(120),
			"completion_tokens": float64(30),
			"total_tokens":      float64(150),
		},
	}
	u, ok := extractOpenAIUsage(result)
	if !ok {
		t.Fatalf("expected usage to be parsed")
	}
	if u.Prompt != 120 || u.Completion != 30 || u.Total != 150 {
		t.Fatalf("unexpected usage: %+v", u)
	}
}

func TestExtractOpenAIUsageDerivesTotal(t *testing.T) {
	result := map[string]interface{}{
		"usage": map[string]interface{}{
			"prompt_tokens":     float64(100),
			"completion_tokens": float64(25),
		},
	}
	u, ok := extractOpenAIUsage(result)
	if !ok {
		t.Fatalf("expected usage to be parsed")
	}
	if u.Total != 125 {
		t.Fatalf("expected derived total 125, got %d", u.Total)
	}
}

func TestExtractGeminiUsage(t *testing.T) {
	result := map[string]interface{}{
		"usageMetadata": map[string]interface{}{
			"promptTokenCount":     float64(200),
			"candidatesTokenCount": float64(40),
			"totalTokenCount":      float64(240),
		},
	}
	u, ok := extractGeminiUsage(result)
	if !ok {
		t.Fatalf("expected usage to be parsed")
	}
	if u.Prompt != 200 || u.Completion != 40 || u.Total != 240 {
		t.Fatalf("unexpected usage: %+v", u)
	}
}

func TestExtractUsageMissing(t *testing.T) {
	if _, ok := extractOpenAIUsage(map[string]interface{}{}); ok {
		t.Fatalf("expected no usage for empty openai result")
	}
	if _, ok := extractGeminiUsage(map[string]interface{}{}); ok {
		t.Fatalf("expected no usage for empty gemini result")
	}
}

func TestAttachAndTakeTokenUsage(t *testing.T) {
	decision := map[string]interface{}{"action": "none"}
	attachTokenUsage(decision, tokenUsage{Prompt: 10, Completion: 5, Total: 15})
	if _, ok := decision[tokenUsageKey]; !ok {
		t.Fatalf("expected usage attached to decision")
	}
	u, ok := takeTokenUsage(decision)
	if !ok || u.Total != 15 {
		t.Fatalf("expected to take usage total 15, got %+v ok=%v", u, ok)
	}
	if _, ok := decision[tokenUsageKey]; ok {
		t.Fatalf("expected usage key stripped after take")
	}
}

func TestAttachTokenUsageSkipsEmpty(t *testing.T) {
	decision := map[string]interface{}{"action": "none"}
	attachTokenUsage(decision, tokenUsage{})
	if _, ok := decision[tokenUsageKey]; ok {
		t.Fatalf("expected empty usage not to be attached")
	}
}

func TestProcessPendingTurnResultsRecordsTokenUsage(t *testing.T) {
	g := NewGame("test", "test")
	g.PauseBetweenTurns = false
	g.GameOver = true // skip applyDecision side effects; telemetry is recorded first
	g.TokenPricing[g.Player1] = tokenPricing{InputPerMillion: 5, OutputPerMillion: 15}

	decision := map[string]interface{}{"action": "none"}
	attachTokenUsage(decision, tokenUsage{Prompt: 1000, Completion: 200, Total: 1200})
	g.pendingTurnResults <- turnResult{
		playerID: g.Player1,
		role:     "defender",
		decision: decision,
	}

	g.processPendingTurnResults()

	if g.ProviderTokenUsage[g.Player1] != 1200 {
		t.Fatalf("expected 1200 tokens recorded, got %d", g.ProviderTokenUsage[g.Player1])
	}
	// cost micros = prompt*inputPerMillion + completion*outputPerMillion
	//             = 1000*5 + 200*15 = 5000 + 3000 = 8000
	if g.ProviderCostMicros[g.Player1] != 8000 {
		t.Fatalf("expected 8000 cost micros, got %d", g.ProviderCostMicros[g.Player1])
	}
}

func TestTokenCostMicrosZeroWithoutPricing(t *testing.T) {
	g := NewGame("test", "test")
	if got := g.tokenCostMicros(g.Player1, tokenUsage{Prompt: 100, Completion: 100, Total: 200}); got != 0 {
		t.Fatalf("expected 0 cost without pricing, got %d", got)
	}
}
