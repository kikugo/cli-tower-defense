package engine

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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

