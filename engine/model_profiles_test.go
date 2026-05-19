package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildMatchConfigFromProfiles(t *testing.T) {
	c := ModelProfileCatalog{
		Profiles: map[string]PlayerModelConfig{
			"p1": {Provider: ProviderOpenAICompatible, Model: "m1", APIKeyEnv: "K1"},
			"p2": {Provider: ProviderGeminiNative, Model: "m2", APIKeyEnv: "K2"},
		},
	}
	cfg, err := BuildMatchConfigFromProfiles(c, "p1", "p2")
	if err != nil {
		t.Fatalf("expected profiles to resolve: %v", err)
	}
	if cfg.Player1.Model != "m1" || cfg.Player2.Model != "m2" {
		t.Fatalf("unexpected models: %#v", cfg)
	}
}

func TestLoadModelProfileCatalog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	content := `{"profiles":{"fast":{"provider":"openai_compatible","model":"a","api_key_env":"OPENAI_API_KEY"}}}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	catalog, err := LoadModelProfileCatalog(path)
	if err != nil {
		t.Fatalf("expected catalog to load: %v", err)
	}
	if _, ok := catalog.Profiles["fast"]; !ok {
		t.Fatalf("expected profile fast")
	}
}
