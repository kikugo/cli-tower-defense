package engine

type ScoreBreakdown struct {
	RawScore           int     `json:"raw_score"`
	WinBonus           float64 `json:"win_bonus"`
	WaveComponent      float64 `json:"wave_component"`
	LivesComponent     float64 `json:"lives_component"`
	RejectPenalty      float64 `json:"reject_penalty"`
	ProviderErrPenalty float64 `json:"provider_error_penalty"`
	Normalized         float64 `json:"normalized"`
}

func BuildScoreBreakdown(result MatchResult, playerID string) ScoreBreakdown {
	raw := result.Score[playerID]
	waveRatio := 0.0
	if result.MaxWaves > 0 {
		waveRatio = float64(result.Waves) / float64(result.MaxWaves)
	}
	livesRatio := 0.0
	if result.Lives[playerID] > 0 {
		livesRatio = float64(result.Lives[playerID]) / 20.0
		if livesRatio > 1.0 {
			livesRatio = 1.0
		}
	}
	winBonus := 0.0
	if result.Winner == playerID {
		winBonus = 1.0
	}
	rejectPenalty := float64(totalByPlayerPrefix(result.RejectedActions, playerID)) * 0.02
	errPenalty := float64(totalByPlayerPrefix(result.ProviderErrors, playerID)) * 0.05
	normalized := (float64(raw) / 500.0) + winBonus + (waveRatio * 0.6) + (livesRatio * 0.4) - rejectPenalty - errPenalty

	return ScoreBreakdown{
		RawScore:           raw,
		WinBonus:           winBonus,
		WaveComponent:      waveRatio * 0.6,
		LivesComponent:     livesRatio * 0.4,
		RejectPenalty:      rejectPenalty,
		ProviderErrPenalty: errPenalty,
		Normalized:         normalized,
	}
}
