package engine

// createEnemyPrompt and parseEnemyResponse below are unused by the live
// path (both live providers build the enemy prompt and parse enemy
// responses via GeminiHandler directly) but are kept as thin delegates for
// API symmetry. OpenAIHandler.GetEnemyDecision, the dead HTTP method that
// used to be their only caller, was deleted per AUDIT-FOLLOWUP-2.md's
// correction: "delete methods, not types."

func (h *OpenAIHandler) createEnemyPrompt(gameState map[string]interface{}) string {
	g := &GeminiHandler{}
	return g.createEnemyPrompt(gameState)
}

func (h *OpenAIHandler) parseEnemyResponse(resp string) (map[string]interface{}, error) {
	g := &GeminiHandler{}
	return g.parseEnemyResponse(resp)
}
