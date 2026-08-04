package engine

import "time"

type ArenaRunManifest struct {
	GeneratedAt string            `json:"generated_at"`
	RunType     string            `json:"run_type"`
	Seed        int64             `json:"seed"`
	MapType     string            `json:"map_type"`
	Swapped     bool              `json:"swapped"`
	MaxTicks    int               `json:"max_ticks"`
	MaxWaves    int               `json:"max_waves"`
	Models      map[string]string `json:"models"`
	// Providers records the resolved, non-secret provider configuration per
	// playerID (temperature, base URL, timeouts, retries, ...). It never
	// contains an API key -- see ProviderConfigRecord.
	Providers      map[string]ProviderConfigRecord `json:"providers,omitempty"`
	Ruleset        ArenaRuleset                    `json:"ruleset"`
	GitCommit      string                          `json:"git_commit,omitempty"`
	BalanceVersion string                          `json:"balance_version,omitempty"`
	// BalanceHash is a content-derived fingerprint of g.Balance (see
	// ComputeBalanceHash), unlike BalanceVersion which is a hand-written
	// label that does not change when a sweep or override retunes stats.
	// Two manifests with the same BalanceVersion but different BalanceHash
	// ran materially different games.
	BalanceHash string `json:"balance_hash,omitempty"`
}

func BuildRunManifest(runType string, g *Game, seed int64, swapped bool, maxTicks int, ruleset ArenaRuleset, gitCommit string) ArenaRunManifest {
	manifest := ArenaRunManifest{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		RunType:     runType,
		Seed:        seed,
		Swapped:     swapped,
		MaxTicks:    maxTicks,
		Ruleset:     ruleset,
		GitCommit:   gitCommit,
	}
	if g != nil {
		manifest.MapType = g.MapType
		manifest.MaxWaves = g.MaxWaves
		manifest.Models = copyStringMap(g.ModelNames)
		manifest.Providers = copyProviderConfigMap(g.ProviderConfigs)
		manifest.BalanceVersion = g.Balance.Version
		manifest.BalanceHash = ComputeBalanceHash(g.Balance)
	}
	return manifest
}

func copyProviderConfigMap(src map[string]ProviderConfigRecord) map[string]ProviderConfigRecord {
	if src == nil {
		return nil
	}
	dst := make(map[string]ProviderConfigRecord, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
