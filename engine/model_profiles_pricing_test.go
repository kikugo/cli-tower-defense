package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// TestProfileCatalogCarriesPricing proves that pricing set on a profile flows
// through the catalog, match-config resolution, and into the game's per-player
// pricing used for cost estimates.
func TestProfileCatalogCarriesPricing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	catalogJSON := `{
      "profiles": {
        "priced_defender": {
          "provider": "openai_compatible",
          "model": "gpt-test",
          "api_key_env": "PROFILE_PRICING_TEST_KEY",
          "base_url": "https://example.com/v1/chat/completions",
          "price_input_per_million": 0.5,
          "price_output_per_million": 1.5
        },
        "free_attacker": {
          "provider": "scripted",
          "model": "attacker_spawn",
          "api_key_env": "NONE"
        }
      }
    }`
	if err := os.WriteFile(path, []byte(catalogJSON), 0600); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	t.Setenv("PROFILE_PRICING_TEST_KEY", "sk-test")

	catalog, err := LoadModelProfileCatalog(path)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	cfg, err := BuildMatchConfigFromProfiles(catalog, "priced_defender", "free_attacker")
	if err != nil {
		t.Fatalf("build match config: %v", err)
	}
	resolved, err := ResolveMatchConfig(cfg)
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	g := NewGameFromResolvedConfig(resolved)

	pricing := g.TokenPricing[g.Player1]
	if pricing.InputPerMillion != 0.5 || pricing.OutputPerMillion != 1.5 {
		t.Fatalf("expected profile pricing to reach the game, got %+v", pricing)
	}

	// And the pricing must actually drive cost estimates.
	cost := g.tokenCostMicros(g.Player1, tokenUsage{Prompt: 1_000_000, Completion: 1_000_000, Total: 2_000_000})
	// 1e6 * 0.5 + 1e6 * 1.5 = 2_000_000 micros
	if cost != 2_000_000 {
		t.Fatalf("expected 2000000 cost micros, got %d", cost)
	}
}
