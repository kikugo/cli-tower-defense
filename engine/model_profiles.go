package engine

import (
	"encoding/json"
	"fmt"
	"os"
)

type ModelProfileCatalog struct {
	Profiles map[string]PlayerModelConfig `json:"profiles"`
}

func LoadModelProfileCatalog(path string) (ModelProfileCatalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ModelProfileCatalog{}, fmt.Errorf("read model profiles: %w", err)
	}
	var catalog ModelProfileCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return ModelProfileCatalog{}, fmt.Errorf("parse model profiles: %w", err)
	}
	if len(catalog.Profiles) == 0 {
		return ModelProfileCatalog{}, fmt.Errorf("model profiles catalog is empty")
	}
	for name, cfg := range catalog.Profiles {
		if err := validatePlayerConfig("profile "+name, cfg); err != nil {
			return ModelProfileCatalog{}, err
		}
	}
	return catalog, nil
}

func BuildMatchConfigFromProfiles(catalog ModelProfileCatalog, player1Profile, player2Profile string) (MatchConfig, error) {
	p1, ok := catalog.Profiles[player1Profile]
	if !ok {
		return MatchConfig{}, fmt.Errorf("unknown player1 profile %q", player1Profile)
	}
	p2, ok := catalog.Profiles[player2Profile]
	if !ok {
		return MatchConfig{}, fmt.Errorf("unknown player2 profile %q", player2Profile)
	}
	cfg := MatchConfig{Player1: p1, Player2: p2}
	if err := ValidateMatchConfig(cfg); err != nil {
		return MatchConfig{}, err
	}
	return cfg, nil
}
