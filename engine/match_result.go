package engine

import "time"

func (r MatchResult) Player1() string {
	if _, ok := r.Models["p1"]; ok {
		return "p1"
	}
	return r.Defender
}

func (r MatchResult) Player2() string {
	if _, ok := r.Models["p2"]; ok {
		return "p2"
	}
	return r.Attacker
}

func (g *Game) BuildMatchResult() MatchResult {
	if g == nil {
		return MatchResult{}
	}
	duration := time.Since(g.StartedAt)
	if g.StartedAt.IsZero() {
		duration = 0
	}
	baseForScoring := MatchResult{
		Winner:          g.Winner,
		MaxWaves:        g.MaxWaves,
		Waves:           g.Wave,
		Score:           g.Score,
		Lives:           g.Lives,
		RejectedActions: g.RejectedActions,
		ProviderErrors:  g.ProviderErrors,
	}
	p1Breakdown := BuildScoreBreakdown(baseForScoring, g.Player1)
	p2Breakdown := BuildScoreBreakdown(baseForScoring, g.Player2)

	return MatchResult{
		Winner:      g.Winner,
		WinnerModel: g.ModelNames[g.Winner],
		WinReason:   g.inferWinReason(),
		Ticks:       g.TickCount,
		Waves:       g.Wave,
		MaxWaves:    g.MaxWaves,
		Defender:    g.Defender,
		Attacker:    g.Attacker,
		Models:      copyStringMap(g.ModelNames),
		Lives:       copyIntMap(g.Lives),
		Score:       copyIntMap(g.Score),
		NormalizedScore: map[string]float64{
			g.Player1: p1Breakdown.Normalized,
			g.Player2: p2Breakdown.Normalized,
		},
		ScoreBreakdown: map[string]ScoreBreakdown{
			g.Player1: p1Breakdown,
			g.Player2: p2Breakdown,
		},
		ActionCounters:  copyIntMap(g.ActionCounters),
		RejectedActions: copyIntMap(g.RejectedActions),
		ProviderErrors:  copyIntMap(g.ProviderErrors),
		ProviderCalls:   copyIntMap(g.ProviderCalls),
		ProviderLatency: averageLatencyByPlayer(g.ProviderLatencyMS, g.ProviderCalls),
		TokenUsage:      copyIntMap(g.ProviderTokenUsage),
		CostMicros:      copyInt64Map(g.ProviderCostMicros),
		DurationMillis:  duration.Milliseconds(),
		ReplayEvents:    len(g.ReplayEvents),
	}
}

func (g *Game) inferWinReason() string {
	if !g.GameOver {
		return "incomplete"
	}
	if g.Winner == g.Defender {
		return "max_waves_cleared"
	}
	if g.Winner == g.Attacker {
		return "defender_lives_depleted"
	}
	return "unknown"
}

func copyIntMap(src map[string]int) map[string]int {
	dst := make(map[string]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyStringMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyInt64Map(src map[string]int64) map[string]int64 {
	dst := make(map[string]int64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func averageLatencyByPlayer(totalMS map[string]int64, calls map[string]int) map[string]float64 {
	dst := make(map[string]float64, len(totalMS))
	for playerID, total := range totalMS {
		n := calls[playerID]
		if n <= 0 {
			dst[playerID] = 0
			continue
		}
		dst[playerID] = float64(total) / float64(n)
	}
	return dst
}
