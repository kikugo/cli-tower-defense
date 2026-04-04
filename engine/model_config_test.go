package engine

import "testing"

func TestDefaultMatchConfigIsValid(t *testing.T) {
	cfg := DefaultMatchConfig()
	if err := ValidateMatchConfig(cfg); err != nil {
		t.Fatalf("default config should validate, got: %v", err)
	}
}

func TestValidateMatchConfigRejectsBadProvider(t *testing.T) {
	cfg := DefaultMatchConfig()
	cfg.Player1.Provider = "bad_provider"

	if err := ValidateMatchConfig(cfg); err == nil {
		t.Fatalf("expected bad provider validation error")
	}
}

func TestParseMatchConfigJSON(t *testing.T) {
	raw := `{
		"player1": {"provider":"openai_compatible","model":"gpt-4.1-mini","api_key_env":"KEY1"},
		"player2": {"provider":"gemini_native","model":"gemini-2.5-flash","api_key_env":"KEY2"}
	}`

	cfg, err := parseMatchConfigJSON(raw)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if cfg.Player1.Model != "gpt-4.1-mini" {
		t.Fatalf("unexpected player1 model: %s", cfg.Player1.Model)
	}
	if cfg.Player2.Provider != ProviderGeminiNative {
		t.Fatalf("unexpected player2 provider: %s", cfg.Player2.Provider)
	}
}

