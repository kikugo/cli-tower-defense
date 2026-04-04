package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type GeminiNativeProvider struct {
	config ResolvedPlayerModelConfig
	client *http.Client
}

func NewGeminiNativeProvider(config ResolvedPlayerModelConfig) *GeminiNativeProvider {
	timeout := time.Duration(config.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &GeminiNativeProvider{
		config: config,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *GeminiNativeProvider) Name() string {
	return fmt.Sprintf("%s/%s", p.config.Provider, p.config.Model)
}

func (p *GeminiNativeProvider) GetTowerDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	prompt := (&OpenAIHandler{}).createTowerPrompt(gameState)
	text, err := p.generateContent(prompt)
	if err != nil {
		return map[string]interface{}{"action": "save"}, nil
	}
	return (&OpenAIHandler{}).parseTowerResponse(text)
}

func (p *GeminiNativeProvider) GetEnemyDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	prompt := (&GeminiHandler{}).createEnemyPrompt(gameState)
	text, err := p.generateContent(prompt)
	if err != nil {
		return getFallbackEnemyDecision(100), nil
	}
	return (&GeminiHandler{}).parseEnemyResponse(text)
}

func (p *GeminiNativeProvider) generateContent(prompt string) (string, error) {
	temperature := 0.7
	maxTokens := 150.0
	if v, ok := p.config.Params["temperature"]; ok {
		temperature = v
	}
	if v, ok := p.config.Params["max_tokens"]; ok {
		maxTokens = v
	}

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     temperature,
			"maxOutputTokens": int(maxTokens),
		},
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s?key=%s", p.config.BaseURL, p.config.APIKey)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, reqErr := http.NewRequest("POST", url, bytes.NewReader(reqJSON))
		if reqErr != nil {
			lastErr = reqErr
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		for key, value := range p.config.Headers {
			req.Header.Set(key, value)
		}

		resp, callErr := p.client.Do(req)
		if callErr != nil {
			lastErr = callErr
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("gemini provider returned status %d", resp.StatusCode)
			resp.Body.Close()
			continue
		}

		var result map[string]interface{}
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if decodeErr != nil {
			lastErr = decodeErr
			continue
		}

		text, ok := extractGeminiContentText(result)
		if !ok {
			lastErr = fmt.Errorf("gemini provider returned empty text")
			continue
		}
		return text, nil
	}

	return "", lastErr
}

