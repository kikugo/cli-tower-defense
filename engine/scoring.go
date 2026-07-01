package engine

// Score normalization weights. They sum to 1.0 so the weighted total, before
// penalties, lands in [0,1]. Penalties are then subtracted and the result is
// clamped back into [0,1], so Normalized is always a true normalized value.
const (
	scoreShareWeight = 0.35
	winBonusWeight   = 0.35
	waveWeight       = 0.20
	livesWeight      = 0.10

	rejectPenaltyPerAction = 0.02
	errorPenaltyPerError   = 0.05
	startingLivesReference = 20.0
)

type ScoreBreakdown struct {
	RawScore           int     `json:"raw_score"`
	ScoreShare         float64 `json:"score_share_component"`
	WinBonus           float64 `json:"win_bonus"`
	WaveComponent      float64 `json:"wave_component"`
	LivesComponent     float64 `json:"lives_component"`
	RejectPenalty      float64 `json:"reject_penalty"`
	ProviderErrPenalty float64 `json:"provider_error_penalty"`
	Normalized         float64 `json:"normalized"`
}

func BuildScoreBreakdown(result MatchResult, playerID string) ScoreBreakdown {
	raw := result.Score[playerID]

	// Score share compares the player against the opponent(s) in this match,
	// keeping the component bounded in [0,1] regardless of absolute score.
	totalScore := 0
	for _, s := range result.Score {
		if s > 0 {
			totalScore += s
		}
	}
	scoreShare := 0.0
	if totalScore > 0 && raw > 0 {
		scoreShare = float64(raw) / float64(totalScore)
	}

	waveRatio := 0.0
	if result.MaxWaves > 0 {
		waveRatio = clamp01(float64(result.Waves) / float64(result.MaxWaves))
	}
	livesRatio := clamp01(float64(result.Lives[playerID]) / startingLivesReference)

	winBonus := 0.0
	if result.Winner == playerID {
		winBonus = 1.0
	}

	rejectPenalty := float64(totalByPlayerPrefix(result.RejectedActions, playerID)) * rejectPenaltyPerAction
	errPenalty := float64(totalByPlayerPrefix(result.ProviderErrors, playerID)) * errorPenaltyPerError

	normalized := scoreShare*scoreShareWeight +
		winBonus*winBonusWeight +
		waveRatio*waveWeight +
		livesRatio*livesWeight -
		rejectPenalty -
		errPenalty

	return ScoreBreakdown{
		RawScore:           raw,
		ScoreShare:         scoreShare * scoreShareWeight,
		WinBonus:           winBonus * winBonusWeight,
		WaveComponent:      waveRatio * waveWeight,
		LivesComponent:     livesRatio * livesWeight,
		RejectPenalty:      rejectPenalty,
		ProviderErrPenalty: errPenalty,
		Normalized:         clamp01(normalized),
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
