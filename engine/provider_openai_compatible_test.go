package engine

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// minimalProviderGameState is the smallest gameState map that
// createTowerPrompt/createEnemyPrompt can render without panicking on a
// missing type assertion (they read "paths_count" and "wave" as plain int).
// Shared by both provider test files' failure-path tests, which don't care
// about prompt content -- only about the error/fallback contract.
func minimalProviderGameState() map[string]interface{} {
	return map[string]interface{}{
		"resources":   map[string]interface{}{"p1": 100, "p2": 100},
		"income":      map[string]interface{}{"p1": 5, "p2": 5},
		"wave":        1,
		"paths_count": 1,
	}
}

func TestOpenAICompatibleProviderTowerDecision(t *testing.T) {
	provider := NewOpenAICompatibleProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{
			Provider:       ProviderOpenAICompatible,
			Model:          "test-model",
			BaseURL:        "https://example.invalid/v1/chat/completions",
			TimeoutSeconds: 5,
		},
		APIKey: "test-key",
	})
	provider.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := `{"choices":[{"message":{"content":"{\"action\":\"place\",\"tower_type\":\"basic\",\"position\":[1,2]}"}}]}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	decision, err := provider.GetTowerDecision(map[string]interface{}{
		"resources":   map[string]interface{}{"p1": 100},
		"income":      map[string]interface{}{"p1": 5},
		"wave":        1,
		"paths_count": 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "place" {
		t.Fatalf("expected place action, got %v", decision["action"])
	}
}

// TestOpenAICompatibleProviderTowerDecisionReturnsErrorOnFailure guards P1.1:
// both HTTP providers used to swallow every failure and return a nil error,
// which meant ProviderErrors, the provider_error replay event, and the error
// penalty in scoring were all unreachable dead code (ARENA-AUDIT.md A2).
func TestOpenAICompatibleProviderTowerDecisionReturnsErrorOnFailure(t *testing.T) {
	provider := NewOpenAICompatibleProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{
			Provider:       ProviderOpenAICompatible,
			Model:          "test-model",
			BaseURL:        "https://example.invalid/v1/chat/completions",
			TimeoutSeconds: 5,
			Params:         map[string]float64{"retry_count": 1},
		},
		APIKey: "test-key",
	})
	provider.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("connection refused")
		}),
	}

	decision, err := provider.GetTowerDecision(minimalProviderGameState())
	if err == nil {
		t.Fatalf("expected a non-nil error on HTTP failure, got nil")
	}
	if decision["action"] != "save" {
		t.Fatalf("expected the provider-failure fallback to be a plain save, got %v", decision["action"])
	}
}

// TestOpenAICompatibleProviderEnemyDecisionReturnsErrorOnFailure guards P1.5:
// getFallbackEnemyDecision(100) always produced "spawn tank" on failure,
// injecting a specific tactical choice a network error never earned. The
// fallback must be "save" with the real error returned.
func TestOpenAICompatibleProviderEnemyDecisionReturnsErrorOnFailure(t *testing.T) {
	provider := NewOpenAICompatibleProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{
			Provider:       ProviderOpenAICompatible,
			Model:          "test-model",
			BaseURL:        "https://example.invalid/v1/chat/completions",
			TimeoutSeconds: 5,
			Params:         map[string]float64{"retry_count": 1},
		},
		APIKey: "test-key",
	})
	provider.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("connection refused")
		}),
	}

	decision, err := provider.GetEnemyDecision(minimalProviderGameState())
	if err == nil {
		t.Fatalf("expected a non-nil error on HTTP failure, got nil")
	}
	if decision["action"] != "save" {
		t.Fatalf("expected the provider-failure fallback to be a plain save, not a tactical spawn, got %v", decision["action"])
	}
}
