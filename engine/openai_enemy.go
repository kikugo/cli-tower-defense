package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// GetEnemyDecision allows OpenAI (ChatGPT) to act as attacker.
func (h *OpenAIHandler) GetEnemyDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	fmt.Println("\n=== ChatGPT (attacker) ===")

	prompt := h.createEnemyPrompt(gameState)
	reqBody := map[string]interface{}{
		"model":       "o3",
		"messages":    []map[string]interface{}{{"role": "user", "content": prompt}},
		"temperature": 0.7,
		"max_tokens":  150,
	}
	reqJSON, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqJSON))
	req.Header.Set("Authorization", "Bearer "+h.APIKey)
	req.Header.Set("Content-Type", "application/json")

	var result map[string]interface{}
	var err error
	for i := 0; i < 3; i++ {
		resp, e := h.Client.Do(req)
		if e != nil {
			err = e
			continue
		}
		defer resp.Body.Close()
		if json.NewDecoder(resp.Body).Decode(&result) == nil {
			err = nil
			break
		}
	}
	if err != nil {
		// fallback
		return getFallbackEnemyDecision(int(gameState["resources"].(map[string]interface{})["chatgpt"].(int))), nil
	}
	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return getFallbackEnemyDecision(gameState["resources"].(map[string]interface{})["chatgpt"].(int)), nil
	}
	content := choices[0].(map[string]interface{})["message"].(map[string]interface{})["content"].(string)
	return h.parseEnemyResponse(content)
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
