package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// GetTowerDecision allows Gemini to act as defender when roles are swapped.
func (h *GeminiHandler) GetTowerDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	fmt.Println("\n=== Gemini's Turn (defender) ===")

	prompt := h.createTowerPrompt(gameState)
	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{{"parts": []map[string]string{{"text": prompt}}}},
	}
	reqJSON, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=%s", h.APIKey)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(reqJSON))
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
		return map[string]interface{}{"action": "save"}, nil
	}

	preds, ok := result["candidates"].([]interface{})
	if !ok || len(preds) == 0 {
		return map[string]interface{}{"action": "save"}, nil
	}
	content := preds[0].(map[string]interface{})["content"].(map[string]interface{})["parts"].([]interface{})[0].(map[string]interface{})["text"].(string)
	return h.parseTowerResponse(content)
}

func (h *GeminiHandler) createTowerPrompt(gameState map[string]interface{}) string {
	// Duplicate of OpenAI logic for now.
	enemies, _ := gameState["enemies"].([]interface{})
	towers, _ := gameState["towers"].([]interface{})
	wave := gameState["wave"].(int)
	resources := int(gameState["resources"].(map[string]interface{})["gemini"].(int))

	lines := []string{
		fmt.Sprintf("Wave: %d, Resources: %d", wave, resources),
		fmt.Sprintf("Existing towers: %d, Incoming enemies: %d", len(towers), len(enemies)),
		"Decide to place a tower or save resources.",
		"Respond with JSON like {\"action\":\"place|save\", \"tower_type\":\"basic\", \"position\":[y,x]}",
	}
	return strings.Join(lines, "\n")
}

func (h *GeminiHandler) parseTowerResponse(resp string) (map[string]interface{}, error) {
	// Reuse OpenAI parser by temp OpenAI handler
	o := &OpenAIHandler{AIHandler: h.AIHandler}
	return o.parseTowerResponse(resp)
}
