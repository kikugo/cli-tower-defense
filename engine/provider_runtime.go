package engine

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

func providerRetryAttempts(config ResolvedPlayerModelConfig) int {
	retries := 3
	if raw, ok := config.Params["retry_count"]; ok {
		if int(raw) > 0 {
			retries = int(raw)
		}
	}
	return retries
}

// defaultTemperature is the sampling temperature used by both live
// providers (see getChatCompletion / generateContent) when params.temperature
// is not set. Exported resolution logic lives in resolvedTemperature so the
// manifest can record the value that was actually used, including this
// default -- rather than leaving it implicit.
const defaultTemperature = 0.7

func resolvedTemperature(params map[string]float64) float64 {
	if v, ok := params["temperature"]; ok {
		return v
	}
	return defaultTemperature
}

func providerErrorLabel(err error) string {
	if err == nil {
		return "none"
	}
	var netErr net.Error
	switch {
	case errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	case strings.Contains(strings.ToLower(err.Error()), "status"):
		return "http_status"
	case strings.Contains(strings.ToLower(err.Error()), "decode"):
		return "decode"
	default:
		return "provider_error"
	}
}

func wrapProviderError(providerName, operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s %s failed: %w", providerName, operation, err)
}

// defaultCompletionTokens is sized so reasoning models still emit a JSON
// decision after spending hidden tokens on reasoning. Profiles override via
// params.max_tokens.
const defaultCompletionTokens = 300

func completionTokenBudget(params map[string]float64) int {
	if v, ok := params["max_tokens"]; ok && v > 0 {
		return int(v)
	}
	return defaultCompletionTokens
}
