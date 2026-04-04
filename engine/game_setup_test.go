package engine

import "testing"

func TestNewGameFromResolvedConfigUsesConfiguredModels(t *testing.T) {
	game := NewGameFromResolvedConfig(ResolvedMatchConfig{
		Player1: ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{
				Provider:       ProviderOpenAICompatible,
				Model:          "model-a",
				APIKeyEnv:      "KEY_A",
				BaseURL:        "https://example.invalid/v1/chat/completions",
				TimeoutSeconds: 5,
			},
			APIKey: "key-a",
		},
		Player2: ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{
				Provider:       ProviderGeminiNative,
				Model:          "model-b",
				APIKeyEnv:      "KEY_B",
				BaseURL:        "https://example.invalid/v1beta/models/model-b:generateContent",
				TimeoutSeconds: 5,
			},
			APIKey: "key-b",
		},
	})

	if game.ModelNames[game.Player1] != "model-a" {
		t.Fatalf("expected player1 model-a, got %s", game.ModelNames[game.Player1])
	}
	if game.ModelNames[game.Player2] != "model-b" {
		t.Fatalf("expected player2 model-b, got %s", game.ModelNames[game.Player2])
	}
}

