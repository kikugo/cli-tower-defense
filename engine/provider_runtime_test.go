package engine

import (
	"errors"
	"testing"
)

func TestProviderRetryAttemptsFromParams(t *testing.T) {
	cfg := ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{
			Params: map[string]float64{"retry_count": 5},
		},
	}
	if providerRetryAttempts(cfg) != 5 {
		t.Fatalf("expected retry count 5")
	}
}

func TestProviderErrorLabel(t *testing.T) {
	if providerErrorLabel(errors.New("status 503")) != "http_status" {
		t.Fatalf("expected http_status label")
	}
}

func TestCompletionTokenBudget(t *testing.T) {
	if got := completionTokenBudget(nil); got != 300 {
		t.Fatalf("expected default 300, got %d", got)
	}
	if got := completionTokenBudget(map[string]float64{"max_tokens": 500}); got != 500 {
		t.Fatalf("expected override 500, got %d", got)
	}
	if got := completionTokenBudget(map[string]float64{"max_tokens": 0}); got != 300 {
		t.Fatalf("expected zero override to fall back to 300, got %d", got)
	}
}
