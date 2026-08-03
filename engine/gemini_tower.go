package engine

// createTowerPrompt and parseTowerResponse below are unused by the live
// path (both live providers build the tower prompt and parse tower
// responses via OpenAIHandler directly) but are kept as thin delegates for
// API symmetry. GeminiHandler.GetTowerDecision, the dead HTTP method that
// used to be their only caller, was deleted per AUDIT-FOLLOWUP-2.md's
// correction: "delete methods, not types."

func (h *GeminiHandler) createTowerPrompt(gameState map[string]interface{}) string {
	// reuse OpenAI's method
	o := &OpenAIHandler{AIHandler: h.AIHandler}
	return o.createTowerPrompt(gameState)
}

func (h *GeminiHandler) parseTowerResponse(resp string) (map[string]interface{}, error) {
	// Reuse OpenAI parser by temp OpenAI handler
	o := &OpenAIHandler{AIHandler: h.AIHandler}
	return o.parseTowerResponse(resp)
}
