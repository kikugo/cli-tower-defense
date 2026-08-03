package engine

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGeminiNativeProviderEnemyDecision(t *testing.T) {
	provider := NewGeminiNativeProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{
			Provider:       ProviderGeminiNative,
			Model:          "gemini-test",
			BaseURL:        "https://example.invalid/v1beta/models/model:generateContent",
			TimeoutSeconds: 5,
		},
		APIKey: "test-key",
	})
	provider.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := `{"candidates":[{"content":{"parts":[{"text":"{\"action\":\"spawn\",\"enemy_type\":\"fast\"}"}]}}]}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	decision, err := provider.GetEnemyDecision(map[string]interface{}{
		"resources":   map[string]interface{}{"p2": 100},
		"income":      map[string]interface{}{"p2": 5},
		"wave":        1,
		"paths_count": 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "spawn" {
		t.Fatalf("expected spawn action, got %v", decision["action"])
	}
}

// TestGeminiNativeProviderTowerDecisionReturnsErrorOnFailure guards P1.1 on
// the Gemini-native provider: the tower path used to swallow every failure
// into a nil error, silently keeping ProviderErrors unreachable.
func TestGeminiNativeProviderTowerDecisionReturnsErrorOnFailure(t *testing.T) {
	provider := NewGeminiNativeProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{
			Provider:       ProviderGeminiNative,
			Model:          "gemini-test",
			BaseURL:        "https://example.invalid/v1beta/models/model:generateContent",
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

// TestGeminiNativeProviderEnemyDecisionReturnsErrorOnFailure guards P1.5 on
// the Gemini-native provider: getFallbackEnemyDecision(100) always produced
// "spawn tank" on failure. The fallback must be "save" with the real error
// returned, never a resource-derived tactical choice.
func TestGeminiNativeProviderEnemyDecisionReturnsErrorOnFailure(t *testing.T) {
	provider := NewGeminiNativeProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{
			Provider:       ProviderGeminiNative,
			Model:          "gemini-test",
			BaseURL:        "https://example.invalid/v1beta/models/model:generateContent",
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
