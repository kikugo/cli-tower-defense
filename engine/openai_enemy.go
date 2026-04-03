package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// GetEnemyDecision allows OpenAI (ChatGPT) to act as attacker.
func (h *OpenAIHandler) GetEnemyDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	prompt := h.createEnemyPrompt(gameState)
	reqBody := map[string]interface{}{
		"model":       "o3",
		"messages":    []map[string]interface{}{{"role": "user", "content": prompt}},
		"temperature": 0.7,
		"max_tokens":  150,
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return getFallbackEnemyDecision(100), nil
	}

	var lastErr error
	for i := 0; i < 3; i++ {
		req, reqErr := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(reqJSON))
		if reqErr != nil {
			lastErr = reqErr
			continue
		}
		req.Header.Set("Authorization", "Bearer "+h.APIKey)
		req.Header.Set("Content-Type", "application/json")

		resp, callErr := h.Client.Do(req)
		if callErr != nil {
			lastErr = callErr
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			lastErr = fmt.Errorf("openai returned status %d", resp.StatusCode)
			continue
		}

		var result map[string]interface{}
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if decodeErr != nil {
			lastErr = decodeErr
			continue
		}
		content, ok := extractOpenAIChatContent(result)
		if !ok {
			lastErr = fmt.Errorf("openai response missing content")
			continue
		}
		return h.parseEnemyResponse(content)
	}
	if lastErr != nil {
		return getFallbackEnemyDecision(100), nil
	}
	return getFallbackEnemyDecision(100), nil
}

func (h *OpenAIHandler) createEnemyPrompt(gameState map[string]interface{}) string {
	// reuse Gemini's method by instantiating a temp GeminiHandler with same rng
	g := &GeminiHandler{AIHandler: h.AIHandler}
	return g.createEnemyPrompt(gameState)
}

func (h *OpenAIHandler) parseEnemyResponse(resp string) (map[string]interface{}, error) {
	g := &GeminiHandler{AIHandler: h.AIHandler}
	return g.parseEnemyResponse(resp)
}
