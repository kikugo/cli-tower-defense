package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildRunManifestIncludesCoreFields(t *testing.T) {
	g := NewGame("k1", "k2")
	r := DefaultArenaRuleset()
	m := BuildRunManifest("headless", g, 42, true, 1200, r, "abc123")
	if m.RunType != "headless" || m.Seed != 42 || !m.Swapped || m.MaxTicks != 1200 {
		t.Fatalf("unexpected manifest metadata: %#v", m)
	}
	if m.Models[g.Player1] == "" || m.Models[g.Player2] == "" {
		t.Fatalf("expected model names in manifest")
	}
	if m.GitCommit != "abc123" {
		t.Fatalf("expected git commit in manifest")
	}
}

func TestManifestCarriesBalanceVersion(t *testing.T) {
	g := NewGame("test", "test")
	m := BuildRunManifest("headless", g, 1, false, 100, DefaultArenaRuleset(), "")
	if m.BalanceVersion != DefaultBalanceConfig().Version {
		t.Fatalf("expected balance version %q, got %q", DefaultBalanceConfig().Version, m.BalanceVersion)
	}
}

// TestManifestCarriesBalanceHash is the fix for the "balance_version cannot
// detect drift" problem: BalanceVersion is a hand-written label that
// balance_sweep.go's applyBalanceOverride never updates when it rewrites
// tower/enemy stats, so two materially different games can carry the same
// version string. BalanceHash is derived from the actual config content and
// must match ComputeBalanceHash(g.Balance) exactly.
func TestManifestCarriesBalanceHash(t *testing.T) {
	g := NewGame("test", "test")
	m := BuildRunManifest("headless", g, 1, false, 100, DefaultArenaRuleset(), "")
	want := ComputeBalanceHash(g.Balance)
	if m.BalanceHash == "" {
		t.Fatalf("expected non-empty balance hash")
	}
	if m.BalanceHash != want {
		t.Fatalf("expected balance hash %q, got %q", want, m.BalanceHash)
	}

	// Retuning a stat (what applyBalanceOverride does per sweep candidate)
	// must move the hash even though nothing touches Version.
	st := g.Balance.Towers["basic"]
	st.Damage += 1000
	g.Balance.Towers["basic"] = st
	m2 := BuildRunManifest("headless", g, 1, false, 100, DefaultArenaRuleset(), "")
	if m2.BalanceVersion != m.BalanceVersion {
		t.Fatalf("expected BalanceVersion to stay %q (untouched by a stat retune), got %q", m.BalanceVersion, m2.BalanceVersion)
	}
	if m2.BalanceHash == m.BalanceHash {
		t.Fatalf("expected balance hash to change after retuning a tower stat, both %q", m2.BalanceHash)
	}
}

// TestManifestNeverContainsAPIKey is the most important test in this file:
// ResolvedPlayerModelConfig.APIKey must never reach a manifest, which is
// written to disk and meant to be shareable. This asserts against the
// marshalled JSON directly, not against struct fields, so it also catches a
// future field added to ProviderConfigRecord (or ArenaRunManifest) that
// accidentally embeds or serializes the secret.
func TestManifestNeverContainsAPIKey(t *testing.T) {
	const distinctiveKey = "sk-test-DO-NOT-LEAK-9f8a7b6c5d4e21"

	g := NewGameFromResolvedConfig(ResolvedMatchConfig{
		Player1: ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{
				Provider:       ProviderOpenAICompatible,
				Model:          "model-a",
				APIKeyEnv:      "KEY_A",
				BaseURL:        "https://example.invalid/v1/chat/completions",
				TimeoutSeconds: 5,
				Headers:        map[string]string{"X-Also-Secret": distinctiveKey},
			},
			APIKey: distinctiveKey,
		},
		Player2: ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{
				Provider:       ProviderGeminiNative,
				Model:          "model-b",
				APIKeyEnv:      "KEY_B",
				BaseURL:        "https://example.invalid/v1beta/models/model-b:generateContent",
				TimeoutSeconds: 5,
			},
			APIKey: distinctiveKey,
		},
	})

	m := BuildRunManifest("headless", g, 1, false, 100, DefaultArenaRuleset(), "abc123")
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if strings.Contains(string(raw), distinctiveKey) {
		t.Fatalf("manifest JSON leaked the API key/header secret:\n%s", raw)
	}
}

// TestManifestRecordsProviderConfig covers the "provider configuration is
// not recorded" problem: temperature (including the live-provider default
// of 0.7 when nothing overrides it), max_tokens, retry_count, base_url,
// timeout_seconds and provider type must round-trip into the manifest, per
// player, through JSON.
func TestManifestRecordsProviderConfig(t *testing.T) {
	g := NewGameFromResolvedConfig(ResolvedMatchConfig{
		Player1: ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{
				Provider:       ProviderOpenAICompatible,
				Model:          "model-a",
				APIKeyEnv:      "KEY_A",
				BaseURL:        "https://example.invalid/v1/chat/completions",
				TimeoutSeconds: 5,
				Params:         map[string]float64{"temperature": 0.2, "max_tokens": 500, "retry_count": 2},
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
				// No Params overrides: defaults must be recorded explicitly,
				// not left implicit.
			},
			APIKey: "key-b",
		},
	})

	m := BuildRunManifest("headless", g, 1, false, 100, DefaultArenaRuleset(), "")

	p1, ok := m.Providers[g.Player1]
	if !ok {
		t.Fatalf("expected provider record for player1")
	}
	if p1.Provider != ProviderOpenAICompatible || p1.Model != "model-a" {
		t.Fatalf("unexpected player1 provider record: %+v", p1)
	}
	if p1.BaseURL != "https://example.invalid/v1/chat/completions" {
		t.Fatalf("unexpected player1 base url: %s", p1.BaseURL)
	}
	if p1.TimeoutSeconds != 5 {
		t.Fatalf("unexpected player1 timeout: %d", p1.TimeoutSeconds)
	}
	if p1.Temperature != 0.2 {
		t.Fatalf("expected overridden temperature 0.2, got %v", p1.Temperature)
	}
	if p1.MaxTokens != 500 {
		t.Fatalf("expected overridden max_tokens 500, got %d", p1.MaxTokens)
	}
	if p1.RetryCount != 2 {
		t.Fatalf("expected overridden retry_count 2, got %d", p1.RetryCount)
	}

	p2, ok := m.Providers[g.Player2]
	if !ok {
		t.Fatalf("expected provider record for player2")
	}
	if p2.Provider != ProviderGeminiNative {
		t.Fatalf("unexpected player2 provider: %s", p2.Provider)
	}
	if p2.Temperature != defaultTemperature {
		t.Fatalf("expected default temperature %v when unset, got %v", defaultTemperature, p2.Temperature)
	}
	if p2.MaxTokens != defaultCompletionTokens {
		t.Fatalf("expected default max_tokens %d when unset, got %d", defaultCompletionTokens, p2.MaxTokens)
	}
	if p2.RetryCount != 3 {
		t.Fatalf("expected default retry_count 3 when unset, got %d", p2.RetryCount)
	}

	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	var decoded ArenaRunManifest
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if decoded.Providers[g.Player1].Temperature != 0.2 {
		t.Fatalf("expected overridden temperature to round-trip through JSON, got %v", decoded.Providers[g.Player1].Temperature)
	}
	if decoded.Providers[g.Player2].Temperature != defaultTemperature {
		t.Fatalf("expected default temperature %v to round-trip through JSON, got %v", defaultTemperature, decoded.Providers[g.Player2].Temperature)
	}
}
