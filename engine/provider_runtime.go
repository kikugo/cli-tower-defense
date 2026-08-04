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
//
// Measured against a realistic 635-token prompt, gemini-3-flash-preview (a
// thinking model) spent 982 hidden reasoning tokens before writing any
// output: at max=1024 that left only 38 tokens for the actual response,
// finishReason came back MAX_TOKENS, and the JSON was truncated mid-string
// and failed to parse. At max=4096 the same call spent 2959 tokens thinking,
// finished with STOP, and produced 76 tokens of output that parsed cleanly.
// A non-thinking model (gemini-2.5-flash-lite) used only 60 tokens and
// finished with STOP well under either cap. maxOutputTokens is a cap on
// spend, not a charge, so raising it costs nothing for models that do not
// think -- it only matters for the ones that do, and 4096 is the smallest
// power-of-two headroom the measurement above showed was enough.
const defaultCompletionTokens = 4096

func completionTokenBudget(params map[string]float64) int {
	if v, ok := params["max_tokens"]; ok && v > 0 {
		return int(v)
	}
	return defaultCompletionTokens
}
