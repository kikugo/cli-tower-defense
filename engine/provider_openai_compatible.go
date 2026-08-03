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
	content, usage, err := p.getChatCompletion(prompt)
	if err != nil {
		// A network error must not become a gameplay action: no resource-
		// derived fallback, just "save" tagged as an engine substitution, and
		// -- unlike before -- the real error is returned. processPendingTurnResults
		// takes the error branch on a non-nil error and skips the turn
		// entirely rather than applying this decision.
		decision := map[string]interface{}{"action": "save", "reason": "provider request failed"}
		markDecisionSource(decision, SourceProviderFailure)
		return decision, err
	}
	decision, decErr := (&OpenAIHandler{}).parseTowerResponse(content)
	attachTokenUsage(decision, usage)
	return decision, decErr
}

func (p *OpenAICompatibleProvider) GetEnemyDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	prompt := (&GeminiHandler{}).createEnemyPrompt(gameState)
	content, usage, err := p.getChatCompletion(prompt)
	if err != nil {
		// Was getFallbackEnemyDecision(100), which -- since 100 >= 50 --
		// always produced "spawn tank" regardless of game state. A network
		// error is not a tactical choice; "save" is the only action with no
		// strategic content. See ARENA-AUDIT.md A1 / AUDIT-FOLLOWUP.md P1.5.
		decision := map[string]interface{}{"action": "save", "reason": "provider request failed"}
		markDecisionSource(decision, SourceProviderFailure)
		return decision, err
	}
	decision, decErr := (&GeminiHandler{}).parseEnemyResponse(content)
	attachTokenUsage(decision, usage)
	return decision, decErr
}

func (p *OpenAICompatibleProvider) getChatCompletion(prompt string) (string, tokenUsage, error) {
	temperature := 0.7
	if v, ok := p.config.Params["temperature"]; ok {
		temperature = v
	}
	maxTokens := completionTokenBudget(p.config.Params)

	reqBody := map[string]interface{}{
		"model": p.config.Model,
		"messages": []map[string]interface{}{
			{"role": "user", "content": prompt},
		},
		"temperature": temperature,
		"max_tokens":  maxTokens,
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", tokenUsage{}, wrapProviderError(p.Name(), "marshal request", err)
	}

	var lastErr error
	for attempt := 0; attempt < providerRetryAttempts(p.config); attempt++ {
		req, reqErr := http.NewRequest("POST", p.config.BaseURL, bytes.NewReader(reqJSON))
		if reqErr != nil {
			lastErr = wrapProviderError(p.Name(), "build request", reqErr)
			continue
		}
		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
		req.Header.Set("Content-Type", "application/json")
		for key, value := range p.config.Headers {
			req.Header.Set(key, value)
		}

		resp, callErr := p.client.Do(req)
		if callErr != nil {
			lastErr = wrapProviderError(p.Name(), "http call", callErr)
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = wrapProviderError(p.Name(), "http status", fmt.Errorf("status %d", resp.StatusCode))
			resp.Body.Close()
			continue
		}

		var result map[string]interface{}
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if decodeErr != nil {
			lastErr = wrapProviderError(p.Name(), "decode", decodeErr)
			continue
		}
		content, ok := extractOpenAIChatContent(result)
		if !ok {
			lastErr = wrapProviderError(p.Name(), "decode", fmt.Errorf("empty content"))
			continue
		}
		usage, _ := extractOpenAIUsage(result)
		return content, usage, nil
	}

	return "", tokenUsage{}, lastErr
}
