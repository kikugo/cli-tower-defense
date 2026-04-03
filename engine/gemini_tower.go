package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// GetTowerDecision allows Gemini to act as defender when roles are swapped.
func (h *GeminiHandler) GetTowerDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	prompt := h.createTowerPrompt(gameState)
	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{{"parts": []map[string]string{{"text": prompt}}}},
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return map[string]interface{}{"action": "save"}, nil
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=%s", h.APIKey)

	var lastErr error
	for i := 0; i < 3; i++ {
		req, reqErr := http.NewRequest("POST", url, bytes.NewReader(reqJSON))
		if reqErr != nil {
			lastErr = reqErr
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, callErr := h.Client.Do(req)
		if callErr != nil {
			lastErr = callErr
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			lastErr = fmt.Errorf("gemini returned status %d", resp.StatusCode)
			continue
		}

		var result map[string]interface{}
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if decodeErr != nil {
			lastErr = decodeErr
			continue
		}

		content, ok := extractGeminiContentText(result)
		if !ok {
			lastErr = fmt.Errorf("gemini response missing text content")
			continue
		}
		return h.parseTowerResponse(content)
	}
	if lastErr != nil {
		return map[string]interface{}{"action": "save"}, nil
	}
	return map[string]interface{}{"action": "save"}, nil
}

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
