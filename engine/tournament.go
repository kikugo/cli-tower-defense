package engine

type TournamentConfig struct {
	Name     string              `json:"name"`
	Seeds    []int64             `json:"seeds"`
	MaxTicks int                 `json:"max_ticks"`
	MaxWaves int                 `json:"max_waves"`
	Ruleset  *ArenaRuleset       `json:"ruleset,omitempty"`
	RoleSwap bool                `json:"role_swap"`
	Matchups []TournamentMatchup `json:"matchups"`
}

type TournamentMatchup struct {
	Name    string            `json:"name"`
	Player1 PlayerModelConfig `json:"player1"`
	Player2 PlayerModelConfig `json:"player2"`
}

type TournamentMatchResult struct {
	Matchup string      `json:"matchup"`
	Seed    int64       `json:"seed"`
	Swapped bool        `json:"swapped"`
	Result  MatchResult `json:"result"`
}

type TournamentScheduledRun struct {
	Seed    int64 `json:"seed"`
	Swapped bool  `json:"swapped"`
}

type TournamentReport struct {
	Name      string                  `json:"name"`
	Results   []TournamentMatchResult `json:"results"`
	Standings []TournamentStanding    `json:"standings"`
	Manifests []ArenaRunManifest      `json:"manifests,omitempty"`
}

type TournamentStanding struct {
	Model              string  `json:"model"`
	Matches            int     `json:"matches"`
	Wins               int     `json:"wins"`
	WinRate            float64 `json:"win_rate"`
	AverageScore       float64 `json:"average_score"`
	AverageWaveReached float64 `json:"average_wave_reached"`
	RejectedActions    int     `json:"rejected_actions"`
	ProviderErrors     int     `json:"provider_errors"`
}

func (c TournamentConfig) normalizedSeeds() []int64 {
	if len(c.Seeds) == 0 {
		return []int64{1}
	}
	return c.Seeds
}

func (c TournamentConfig) normalizedMaxTicks() int {
	if c.MaxTicks <= 0 {
		return 3000
	}
	return c.MaxTicks
}

func (c TournamentConfig) NormalizedSeedsForMain() []int64 {
	return c.normalizedSeeds()
}

func (c TournamentConfig) NormalizedMaxTicksForMain() int {
	return c.normalizedMaxTicks()
}

func BuildTournamentSchedule(config TournamentConfig) []TournamentScheduledRun {
	schedule := make([]TournamentScheduledRun, 0)
	for _, seed := range config.normalizedSeeds() {
		schedule = append(schedule, TournamentScheduledRun{Seed: seed})
		if config.RoleSwap {
			schedule = append(schedule, TournamentScheduledRun{Seed: seed, Swapped: true})
		}
	}
	return schedule
}

func BuildTournamentStandings(results []TournamentMatchResult) []TournamentStanding {
	type accum struct {
		standing TournamentStanding
		score    int
		waves    int
	}
	byModel := map[string]*accum{}

	for _, match := range results {
		for playerID, model := range match.Result.Models {
			a := byModel[model]
			if a == nil {
				a = &accum{standing: TournamentStanding{Model: model}}
				byModel[model] = a
			}
			a.standing.Matches++
			if match.Result.Winner == playerID {
				a.standing.Wins++
			}
			a.score += match.Result.Score[playerID]
			a.waves += match.Result.Waves
			a.standing.RejectedActions += totalByPlayerPrefix(match.Result.RejectedActions, playerID)
			a.standing.ProviderErrors += totalByPlayerPrefix(match.Result.ProviderErrors, playerID)
		}
	}

	standings := make([]TournamentStanding, 0, len(byModel))
	for _, a := range byModel {
		if a.standing.Matches > 0 {
			a.standing.WinRate = float64(a.standing.Wins) / float64(a.standing.Matches)
			a.standing.AverageScore = float64(a.score) / float64(a.standing.Matches)
			a.standing.AverageWaveReached = float64(a.waves) / float64(a.standing.Matches)
		}
		standings = append(standings, a.standing)
	}
	return standings
}

func totalByPlayerPrefix(values map[string]int, playerID string) int {
	total := 0
	prefix := playerID + ":"
	for key, value := range values {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			total += value
		}
	}
	return total
}
