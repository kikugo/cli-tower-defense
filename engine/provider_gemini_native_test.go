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

// TestGeminiNativeProviderTowerDecisionTagsMaxTokensTruncation is Bug B's
// instrumentation half: when Gemini reports finishReason "MAX_TOKENS" -- the
// completion token budget cut the response off before the model finished,
// typically because a thinking model spent it all on hidden reasoning -- the
// resulting decision must be tagged with the distinct
// SourceParserFallbackTruncated, not the generic SourceParserUnparseable a
// truncated, unparseable response would otherwise get. The response text
// here is deliberately cut mid-string, mirroring the real
// `{"action": "place", "tower_type": "` truncation observed against
// gemini-3-flash-preview at a 1024-token cap.
func TestGeminiNativeProviderTowerDecisionTagsMaxTokensTruncation(t *testing.T) {
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
			body := `{"candidates":[{"content":{"parts":[{"text":"{\"action\": \"place\", \"tower_type\": \""}]},"finishReason":"MAX_TOKENS"}]}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	decision, err := provider.GetTowerDecision(minimalProviderGameState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src := takeDecisionSource(decision); src != SourceParserFallbackTruncated {
		t.Fatalf("expected SourceParserFallbackTruncated, got %q", src)
	}
}

// TestGeminiNativeProviderTowerDecisionMaxTokensButParsedStaysModelAuthored
// is the coordinator's flagged fix: a model can finish a complete, valid
// JSON object and then keep generating trailing prose until it hits the
// token cap. finishReason still comes back MAX_TOKENS, but the decision
// itself parsed cleanly and genuinely came from the model -- it must stay
// SourceModel (untagged) and keep counting toward the authored share, not
// be relabelled SourceParserFallbackTruncated just because the cap was hit
// somewhere after the object closed.
func TestGeminiNativeProviderTowerDecisionMaxTokensButParsedStaysModelAuthored(t *testing.T) {
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
			body := `{"candidates":[{"content":{"parts":[{"text":"{\"action\":\"place\",\"tower_type\":\"basic\",\"position\":[5,5]} and here are some further thoughts that ran past the cap and got cut off"}]},"finishReason":"MAX_TOKENS"}]}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	decision, err := provider.GetTowerDecision(minimalProviderGameState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "place" || decision["tower_type"] != "basic" {
		t.Fatalf("expected the cleanly-parsed decision, got %#v", decision)
	}
	if src := takeDecisionSource(decision); src != SourceModel {
		t.Fatalf("expected a cleanly-parsed decision to stay SourceModel despite MAX_TOKENS, got %q", src)
	}

	g := newProvenanceGame()
	g.applyDecision(g.Player1, "defender", decision)
	res := g.BuildMatchResult()
	share, ok := res.ModelAuthored(g.Player1)
	if !ok {
		t.Fatalf("expected ModelAuthored to report measured")
	}
	if share != 1 {
		t.Fatalf("expected a cleanly-parsed MAX_TOKENS decision to count as 100%% authored, got %v", share)
	}
}

// TestGeminiNativeProviderTowerDecisionMaxTokensExcludedFromModelAuthored
// confirms the truncation tag actually has teeth: it must count against the
// model-authored share exactly like the other substitution sources, per
// MatchResult.ModelAuthored (engine/match_result.go).
func TestGeminiNativeProviderTowerDecisionMaxTokensExcludedFromModelAuthored(t *testing.T) {
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
			body := `{"candidates":[{"content":{"parts":[{"text":"{\"action\": \"place\", \"tower_type\": \""}]},"finishReason":"MAX_TOKENS"}]}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	decision, err := provider.GetTowerDecision(minimalProviderGameState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	g := newProvenanceGame()
	g.applyDecision(g.Player1, "defender", decision)
	res := g.BuildMatchResult()
	share, ok := res.ModelAuthored(g.Player1)
	if !ok {
		t.Fatalf("expected ModelAuthored to report measured")
	}
	if share != 0 {
		t.Fatalf("expected a MAX_TOKENS-truncated decision to count as 0%% authored, got %v", share)
	}
}
