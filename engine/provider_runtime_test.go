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

