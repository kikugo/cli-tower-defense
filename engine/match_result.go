package engine

import (
	"strconv"
	"strings"
	"time"
)

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

	result := MatchResult{
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
		// ProvenanceVersion 1 marks every match built by this code as having
		// decision-source tracking, however few or many substitutions it
		// contains -- see ModelAuthored for why that matters.
		DecisionSources:   copyIntMap(g.DecisionSources),
		ProvenanceVersion: 1,
		ProviderErrors:    copyIntMap(g.ProviderErrors),
		ProviderCalls:     copyIntMap(g.ProviderCalls),
		ProviderLatency:   averageLatencyByPlayer(g.ProviderLatencyMS, g.ProviderCalls),
		TokenUsage:        copyIntMap(g.ProviderTokenUsage),
		CostMicros:        copyInt64Map(g.ProviderCostMicros),
		DurationMillis:    duration.Milliseconds(),
		ReplayEvents:      len(g.ReplayEvents),
		ReplayTruncated:   g.ReplayTruncated,
		Strata:            g.matchStrata(),
	}
	result.ModelAuthoredShare = map[string]float64{}
	for _, p := range []string{g.Player1, g.Player2} {
		if share, ok := result.ModelAuthored(p); ok {
			result.ModelAuthoredShare[p] = share
		}
	}
	return result
}

// matchStrata records what the match actually turned out to be -- realised
// lane count, map type, and balance version -- as read off live game state,
// not off the ruleset that requested it. A ruleset's map_type can be "" (the
// seeded-random generator, which rolls 1 or 2 lanes at runtime) so the only
// way to know how many lanes a given match got is to count g.Paths after
// generation. The returned map is a fresh copy; it shares nothing with g.
func (g *Game) matchStrata() map[string]string {
	mapType := g.MapType
	if mapType == "" {
		mapType = "random"
	}
	return map[string]string{
		"lanes":    strconv.Itoa(len(g.Paths)),
		"map_type": mapType,
		"balance":  g.Balance.Version,
	}
}

// ModelAuthored reports the share of playerID's recorded decisions that came
// directly from a model response, as opposed to one the engine substituted
// on the model's behalf (a parser fallback, a provider failure, or a
// normalizer default). The bool return exists specifically to distinguish
// "0% authored" from "not measured": a MatchResult built before decision
// provenance was recorded -- ProvenanceVersion's Go zero value is 0, which is
// what every pre-existing replay and manifest on disk has -- always returns
// (0, false), never (0, true) and never (1, true). Absence of provenance is
// never evidence of authorship.
func (r MatchResult) ModelAuthored(playerID string) (float64, bool) {
	if r.ProvenanceVersion == 0 {
		return 0, false
	}
	prefix := playerID + ":"
	total, modelCount := 0, 0
	for key, count := range r.DecisionSources {
		if key != playerID && !strings.HasPrefix(key, prefix) {
			continue
		}
		// A turn skipped because "save" was the only legal action was never
		// put to the model at all, so it is neither an authored decision
		// nor a substitution the engine made on the model's behalf --
		// counting it in the denominator would understate authorship for a
		// model that is simply playing in a low-resource stretch of the
		// match, exactly the effect this metric exists to avoid. See
		// SourceSkippedForcedSave.
		if key == prefix+string(SourceSkippedForcedSave) {
			continue
		}
		total += count
		if key == prefix+string(SourceModel) {
			modelCount += count
		}
	}
	if total == 0 {
		return 0, false
	}
	return float64(modelCount) / float64(total), true
}

// DefenderHeld reports whether the defense succeeded: either an outright
// defender win, or the match ran out of ticks with defender lives intact
// (survival against sustained pressure). Used by balance sweeps and the
// band regression test.
func (r MatchResult) DefenderHeld() bool {
	if r.Winner == r.Defender && r.Winner != "" {
		return true
	}
	return r.Winner == "" && r.Lives[r.Defender] > 0
}

// ResolveTimeout ends a match that hit its tick limit. Surviving the full
// horizon is a defender win — the attacker failed to finish the job — so
// "winner: none" stalemates no longer exist for bounded runs.
func (g *Game) ResolveTimeout() {
	if g == nil || g.GameOver {
		return
	}
	g.GameOver = true
	if g.Lives[g.Defender] > 0 {
		g.Winner = g.Defender
		g.winReasonOverride = "defender_outlasted"
	} else {
		g.Winner = g.Attacker
		g.winReasonOverride = "defender_lives_depleted"
	}
	g.recordReplayEvent(ReplayEvent{
		Type:     ReplayGameEnd,
		PlayerID: g.Winner,
		Reason:   g.winReasonOverride,
		Details:  map[string]interface{}{"winner": g.Winner, "wave": g.Wave},
	})
}

func (g *Game) inferWinReason() string {
	if !g.GameOver {
		return "incomplete"
	}
	if g.winReasonOverride != "" {
		return g.winReasonOverride
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
