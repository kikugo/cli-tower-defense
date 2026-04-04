package engine

import (
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

