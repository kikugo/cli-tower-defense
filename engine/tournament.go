package engine

type TournamentConfig struct {
	Name     string              `json:"name"`
	Seeds    []int64             `json:"seeds"`
	MaxTicks int                 `json:"max_ticks"`
	MaxWaves int                 `json:"max_waves"`
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

type TournamentReport struct {
	Name    string                  `json:"name"`
	Results []TournamentMatchResult `json:"results"`
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
