package engine

import (
	"encoding/json"
	"fmt"
	"os"
)

type ProviderType string

const (
	ProviderOpenAICompatible ProviderType = "openai_compatible"
	ProviderGeminiNative     ProviderType = "gemini_native"
	ProviderScripted         ProviderType = "scripted"
)

type PlayerModelConfig struct {
	Provider       ProviderType       `json:"provider"`
	Model          string             `json:"model"`
	APIKeyEnv      string             `json:"api_key_env"`
	BaseURL        string             `json:"base_url,omitempty"`
	TimeoutSeconds int                `json:"timeout_seconds,omitempty"`
	Headers        map[string]string  `json:"headers,omitempty"`
	Params         map[string]float64 `json:"params,omitempty"`
}

type MatchConfig struct {
	Player1 PlayerModelConfig `json:"player1"`
	Player2 PlayerModelConfig `json:"player2"`
}

type ResolvedPlayerModelConfig struct {
	PlayerModelConfig
	APIKey string
}

type ResolvedMatchConfig struct {
	Player1 ResolvedPlayerModelConfig
	Player2 ResolvedPlayerModelConfig
}

func DefaultMatchConfig() MatchConfig {
	return MatchConfig{
		Player1: PlayerModelConfig{
			Provider:       ProviderOpenAICompatible,
			Model:          "o3",
			APIKeyEnv:      "OPENAI_API_KEY",
			BaseURL:        "https://api.openai.com/v1/chat/completions",
			TimeoutSeconds: 20,
		},
		Player2: PlayerModelConfig{
			Provider:       ProviderGeminiNative,
			Model:          "gemini-2.5-pro",
			APIKeyEnv:      "GOOGLE_API_KEY",
			TimeoutSeconds: 20,
		},
	}
}

func LoadMatchConfigFromEnv() (MatchConfig, error) {
	rawJSON := os.Getenv("MODEL_MATCH_CONFIG")
	configPath := os.Getenv("MODEL_MATCH_CONFIG_PATH")

	switch {
	case rawJSON != "":
		return parseMatchConfigJSON(rawJSON)
	case configPath != "":
		raw, err := os.ReadFile(configPath)
		if err != nil {
			return MatchConfig{}, fmt.Errorf("read MODEL_MATCH_CONFIG_PATH: %w", err)
		}
		return parseMatchConfigJSON(string(raw))
	default:
		return DefaultMatchConfig(), nil
	}
}

func ResolveMatchConfig(config MatchConfig) (ResolvedMatchConfig, error) {
	if err := ValidateMatchConfig(config); err != nil {
		return ResolvedMatchConfig{}, err
	}

	p1Key := os.Getenv(config.Player1.APIKeyEnv)
	p2Key := os.Getenv(config.Player2.APIKeyEnv)
	if config.Player1.Provider != ProviderScripted && p1Key == "" {
		return ResolvedMatchConfig{}, fmt.Errorf("missing API key in env %q for player1", config.Player1.APIKeyEnv)
	}
	if config.Player2.Provider != ProviderScripted && p2Key == "" {
		return ResolvedMatchConfig{}, fmt.Errorf("missing API key in env %q for player2", config.Player2.APIKeyEnv)
	}

	return ResolvedMatchConfig{
		Player1: ResolvedPlayerModelConfig{
			PlayerModelConfig: normalizePlayerConfig(config.Player1),
			APIKey:            p1Key,
		},
		Player2: ResolvedPlayerModelConfig{
			PlayerModelConfig: normalizePlayerConfig(config.Player2),
			APIKey:            p2Key,
		},
	}, nil
}

func ValidateMatchConfig(config MatchConfig) error {
	if err := validatePlayerConfig("player1", config.Player1); err != nil {
		return err
	}
	if err := validatePlayerConfig("player2", config.Player2); err != nil {
		return err
	}
	return nil
}

func parseMatchConfigJSON(raw string) (MatchConfig, error) {
	var config MatchConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return MatchConfig{}, fmt.Errorf("invalid match config JSON: %w", err)
	}
	if err := ValidateMatchConfig(config); err != nil {
		return MatchConfig{}, err
	}
	return config, nil
}

func validatePlayerConfig(player string, config PlayerModelConfig) error {
	if config.Provider != ProviderOpenAICompatible && config.Provider != ProviderGeminiNative && config.Provider != ProviderScripted {
		return fmt.Errorf("%s provider must be %q, %q, or %q", player, ProviderOpenAICompatible, ProviderGeminiNative, ProviderScripted)
	}
	if config.Model == "" {
		return fmt.Errorf("%s model is required", player)
	}
	if config.APIKeyEnv == "" {
		return fmt.Errorf("%s api_key_env is required", player)
	}
	return nil
}

func normalizePlayerConfig(config PlayerModelConfig) PlayerModelConfig {
	copyCfg := config
	if copyCfg.TimeoutSeconds <= 0 {
		copyCfg.TimeoutSeconds = 20
	}
	if copyCfg.Provider == ProviderOpenAICompatible && copyCfg.BaseURL == "" {
		copyCfg.BaseURL = "https://api.openai.com/v1/chat/completions"
	}
	if copyCfg.Provider == ProviderGeminiNative && copyCfg.BaseURL == "" {
		copyCfg.BaseURL = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", copyCfg.Model)
	}
	if copyCfg.Headers == nil {
		copyCfg.Headers = map[string]string{}
	}
	if copyCfg.Params == nil {
		copyCfg.Params = map[string]float64{}
	}
	return copyCfg
}
