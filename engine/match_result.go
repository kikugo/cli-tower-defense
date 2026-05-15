package engine

import "time"

func (g *Game) BuildMatchResult() MatchResult {
	if g == nil {
		return MatchResult{}
	}
	duration := time.Since(g.StartedAt)
	if g.StartedAt.IsZero() {
		duration = 0
	}

	return MatchResult{
		Winner:          g.Winner,
		WinnerModel:     g.ModelNames[g.Winner],
		WinReason:       g.inferWinReason(),
		Ticks:           g.TickCount,
		Waves:           g.Wave,
		MaxWaves:        g.MaxWaves,
		Defender:        g.Defender,
		Attacker:        g.Attacker,
		Models:          copyStringMap(g.ModelNames),
		Lives:           copyIntMap(g.Lives),
		Score:           copyIntMap(g.Score),
		ActionCounters:  copyIntMap(g.ActionCounters),
		RejectedActions: copyIntMap(g.RejectedActions),
		ProviderErrors:  copyIntMap(g.ProviderErrors),
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

