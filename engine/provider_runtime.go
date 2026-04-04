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

