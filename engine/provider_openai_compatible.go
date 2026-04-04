package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type OpenAICompatibleProvider struct {
	config ResolvedPlayerModelConfig
	client *http.Client
}

func NewOpenAICompatibleProvider(config ResolvedPlayerModelConfig) *OpenAICompatibleProvider {
	timeout := time.Duration(config.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &OpenAICompatibleProvider{
		config: config,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *OpenAICompatibleProvider) Name() string {
	return fmt.Sprintf("%s/%s", p.config.Provider, p.config.Model)
}

func (p *OpenAICompatibleProvider) GetTowerDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	prompt := (&OpenAIHandler{}).createTowerPrompt(gameState)
	content, err := p.getChatCompletion(prompt)
	if err != nil {
		return map[string]interface{}{"action": "none", "reason": "provider request failed"}, nil
	}
	return (&OpenAIHandler{}).parseTowerResponse(content)
}

func (p *OpenAICompatibleProvider) GetEnemyDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	prompt := (&GeminiHandler{}).createEnemyPrompt(gameState)
	content, err := p.getChatCompletion(prompt)
	if err != nil {
		return getFallbackEnemyDecision(100), nil
	}
	return (&GeminiHandler{}).parseEnemyResponse(content)
}

func (p *OpenAICompatibleProvider) getChatCompletion(prompt string) (string, error) {
	temperature := 0.7
	maxTokens := 150.0
	if v, ok := p.config.Params["temperature"]; ok {
		temperature = v
	}
	if v, ok := p.config.Params["max_tokens"]; ok {
		maxTokens = v
	}

	reqBody := map[string]interface{}{
		"model": p.config.Model,
		"messages": []map[string]interface{}{
			{"role": "user", "content": prompt},
		},
		"temperature": temperature,
		"max_tokens":  int(maxTokens),
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, reqErr := http.NewRequest("POST", p.config.BaseURL, bytes.NewReader(reqJSON))
		if reqErr != nil {
			lastErr = reqErr
			continue
		}
		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
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
			lastErr = fmt.Errorf("openai compatible provider returned status %d", resp.StatusCode)
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
		content, ok := extractOpenAIChatContent(result)
		if !ok {
			lastErr = fmt.Errorf("openai compatible provider returned empty content")
			continue
		}
		return content, nil
	}

	return "", lastErr
}

