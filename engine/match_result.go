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
		// ProvenanceVersion 2 marks every match built by this code as having
		// both decision-source tracking (version 1) and engine-assist
		// counting (added alongside it here) -- see ModelAuthored and
		// EngineAssistTotal for why that matters. It was bumped from 1
		// rather than left alone specifically so a MatchResult built by the
		// intermediate code (decision sources only, no assist counts) is
		// not misread by EngineAssistTotal as having recorded zero assists.
		DecisionSources:    copyIntMap(g.DecisionSources),
		EngineAssistCounts: copyIntMap(g.EngineAssists),
		BreachCount:        g.BreachCount,
		AuthoredSaveCounts: copyIntMap(g.AuthoredSaves),
		DecisionsResolved:  copyIntMap(g.DecisionsResolved),
		LeakWindow:         append([]bool(nil), g.LeakWindow...),
		WaveSummaries:      g.buildWaveSummaries(),
		ProvenanceVersion:  3,
		ProviderErrors:     copyIntMap(g.ProviderErrors),
		ProviderCalls:      copyIntMap(g.ProviderCalls),
		ProviderLatency:    averageLatencyByPlayer(g.ProviderLatencyMS, g.ProviderCalls),
		TokenUsage:         copyIntMap(g.ProviderTokenUsage),
		CostMicros:         copyInt64Map(g.ProviderCostMicros),
		DurationMillis:     duration.Milliseconds(),
		ReplayEvents:       len(g.ReplayEvents),
		ReplayTruncated:    g.ReplayTruncated,
		Strata:             g.matchStrata(),
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

// AuthorshipState is the third state ModelAuthored's (value, ok) pair cannot
// express. ModelAuthored collapses two genuinely different situations into
// (0, false): a MatchResult that never recorded provenance at all, and one
// that recorded it faithfully but has not resolved a single countable
// decision yet (a match one tick old, or a player whose every turn so far was
// a forced save). Both are "no percentage to show", which is why the boolean
// is right for callers that only need to know whether to print a number --
// but the UI renders them as different words ("authored unknown" against
// "authored none yet"), and a renderer that had to tell them apart could only
// do it by reading ProvenanceVersion itself, duplicating the rule this file
// owns. ModelAuthoredState hands the distinction over directly.
type AuthorshipState int

const (
	// AuthorshipUntracked: this MatchResult predates decision-provenance
	// recording (ProvenanceVersion == 0). Nothing is known about who authored
	// anything. Render as unknown, never as zero.
	AuthorshipUntracked AuthorshipState = iota
	// AuthorshipNoDecisions: provenance is being recorded, and it has counted
	// exactly zero decisions for this player so far. Render as "none yet" --
	// an honest statement that the match has not produced data, not that the
	// model authored nothing.
	AuthorshipNoDecisions
	// AuthorshipMeasured: the share is real. A 0.0 in this state means the
	// model genuinely authored none of its resolved decisions, which is a
	// finding, not a gap.
	AuthorshipMeasured
)

// ModelAuthoredState reports the share of playerID's recorded decisions that
// came directly from a model response, plus which of the three states above
// that share is in. This is the full-fidelity accessor; ModelAuthored is a
// two-state view of the same computation, kept because most callers only need
// "is there a number to print".
//
// The denominator deliberately excludes turns skipped because "save" was the
// only legal action: such a turn was never put to the model at all, so it is
// neither an authored decision nor a substitution the engine made on the
// model's behalf. Counting it would understate authorship for a model that is
// simply playing through a low-resource stretch of the match -- exactly the
// distortion this metric exists to avoid. See SourceSkippedForcedSave.
func (r MatchResult) ModelAuthoredState(playerID string) (float64, AuthorshipState) {
	if r.ProvenanceVersion == 0 {
		return 0, AuthorshipUntracked
	}
	prefix := playerID + ":"
	total, modelCount := 0, 0
	for key, count := range r.DecisionSources {
		if key != playerID && !strings.HasPrefix(key, prefix) {
			continue
		}
		if key == prefix+string(SourceSkippedForcedSave) {
			continue
		}
		total += count
		if key == prefix+string(SourceModel) {
			modelCount += count
		}
	}
	if total == 0 {
		return 0, AuthorshipNoDecisions
	}
	return float64(modelCount) / float64(total), AuthorshipMeasured
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
//
// It reports ok==false for BOTH untracked provenance and tracked-but-empty
// provenance. That collapse is intentional and unchanged; a caller that needs
// to tell those apart calls ModelAuthoredState instead.
func (r MatchResult) ModelAuthored(playerID string) (float64, bool) {
	share, state := r.ModelAuthoredState(playerID)
	return share, state == AuthorshipMeasured
}

// EngineAssistTotal reports how many times the engine acted on playerID's
// behalf via applyAdaptivePressure, summed across every AssistBranch. The
// bool return mirrors ModelAuthored exactly and for the same reason: a
// MatchResult built before assist counting existed -- ProvenanceVersion < 2,
// which covers both the Go zero value (0) and the intermediate value (1)
// used while only decision-source tracking existed -- always returns
// (0, false), never (0, true). A match recorded before this feature must
// read as "assists unknown," never as "assists: zero," because an assist
// that fired and left the old code's counters untouched is indistinguishable
// from one that never fired at all. See EngineAssistCounts.
func (r MatchResult) EngineAssistTotal(playerID string) (int, bool) {
	if r.ProvenanceVersion < 2 {
		return 0, false
	}
	prefix := playerID + ":"
	total := 0
	for key, count := range r.EngineAssistCounts {
		if key == playerID || strings.HasPrefix(key, prefix) {
			total += count
		}
	}
	return total, true
}

// AuthoredSaves reports how many of playerID's resolved decisions were a
// save counted the same way Game.NoopStreak counts them (see AuthoredSaves'
// field doc on Game), and total, the denominator: every decision resolved
// for playerID regardless of action, source, or outcome. The two are
// counted by construction to use the identical rule NoopStreak uses, so
// "authored of total" is a valid read on how often a save-streak assist was
// armed by the player's own choice rather than an engine substitution. The
// bool return mirrors EngineAssistTotal: a MatchResult built before this
// field existed (ProvenanceVersion < 3) always returns (0, 0, false), never
// (0, 0, true) -- absence of provenance is never evidence of zero saves.
func (r MatchResult) AuthoredSaves(playerID string) (authored int, total int, ok bool) {
	if r.ProvenanceVersion < 3 {
		return 0, 0, false
	}
	return r.AuthoredSaveCounts[playerID], r.DecisionsResolved[playerID], true
}

// RecentLeaks reports how many of the last LeakWindowSize resolved enemies
// (killed or leaked, across the whole board) leaked through, and how many
// resolutions the window currently holds. full is true once at least
// LeakWindowSize resolutions have ever been recorded; before that, "leaked N
// of M" would be a fabricated ratio over a window that has not filled yet,
// which is exactly why the design renders "leaked none yet" in that case
// instead -- a caller checks full to choose between the two, the same way a
// caller checks ok on ModelAuthored or EngineAssistTotal. This does not gate
// on ProvenanceVersion: an old MatchResult with no LeakWindow recorded
// simply decodes it as an empty slice, which already reads as "window=0,
// full=false" -- an honest "not enough data", not a false "zero leaked".
func (r MatchResult) RecentLeaks() (leaked int, window int, full bool) {
	window = len(r.LeakWindow)
	for _, l := range r.LeakWindow {
		if l {
			leaked++
		}
	}
	full = window >= LeakWindowSize
	return leaked, window, full
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
