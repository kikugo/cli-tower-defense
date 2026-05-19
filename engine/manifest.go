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
	Ruleset     ArenaRuleset      `json:"ruleset"`
	GitCommit   string            `json:"git_commit,omitempty"`
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
	}
	return manifest
}
