package engine

import "time"

type ReplayEventType string

const (
	ReplayTick        ReplayEventType = "tick"
	ReplayDecision    ReplayEventType = "decision"
	ReplayOutcome     ReplayEventType = "outcome"
	ReplaySpawn       ReplayEventType = "spawn"
	ReplayWave        ReplayEventType = "wave"
	ReplayPlacement   ReplayEventType = "placement"
	ReplayDamage      ReplayEventType = "damage"
	ReplayBreach      ReplayEventType = "breach"
	ReplayResource    ReplayEventType = "resource"
	ReplayGameEnd     ReplayEventType = "game_end"
	ReplayRejected    ReplayEventType = "rejected"
	ReplayProviderErr ReplayEventType = "provider_error"
	ReplayMapInit     ReplayEventType = "map_init"
	// ReplayTruncated is a synthetic marker event. recordReplayEvent plants
	// one the first time MaxReplayEvents forces it to discard events, and
	// keeps updating it in place on every later trim. It carries no board
	// state -- it exists purely so a consumer of the raw stream (chiefly
	// ReconstructSnapshot) can tell a truncated stream from a complete one.
	// Without it, a reconstruction that replays from the start of a
	// truncated stream would silently produce a board missing whatever the
	// discarded events described, with nothing anywhere saying so.
	ReplayTruncated ReplayEventType = "truncated"
)

type ReplayEvent struct {
	Tick     int64                  `json:"tick"`
	Time     time.Time              `json:"time"`
	Type     ReplayEventType        `json:"type"`
	PlayerID string                 `json:"player_id,omitempty"`
	Model    string                 `json:"model,omitempty"`
	Role     string                 `json:"role,omitempty"`
	Action   string                 `json:"action,omitempty"`
	Position *Position              `json:"position,omitempty"`
	Amount   int                    `json:"amount,omitempty"`
	Reason   string                 `json:"reason,omitempty"`
	Details  map[string]interface{} `json:"details,omitempty"`
}

type MatchResult struct {
	Winner          string                    `json:"winner"`
	WinnerModel     string                    `json:"winner_model"`
	WinReason       string                    `json:"win_reason"`
	Ticks           int64                     `json:"ticks"`
	Waves           int                       `json:"waves"`
	MaxWaves        int                       `json:"max_waves"`
	Defender        string                    `json:"defender"`
	Attacker        string                    `json:"attacker"`
	Models          map[string]string         `json:"models"`
	Lives           map[string]int            `json:"lives"`
	Score           map[string]int            `json:"score"`
	NormalizedScore map[string]float64        `json:"normalized_score"`
	ScoreBreakdown  map[string]ScoreBreakdown `json:"score_breakdown"`
	ActionCounters  map[string]int            `json:"action_counters"`
	RejectedActions map[string]int            `json:"rejected_actions"`
	// DecisionSources, ModelAuthoredShare and ProvenanceVersion record who
	// actually made each decision -- a model, or one of the engine's own
	// substitutions (parser fallback, provider failure, normalizer default).
	// ProvenanceVersion is 0 (its Go zero value) on every MatchResult built
	// before this field existed, including every pre-existing replay and
	// manifest on disk. That is deliberate: see ModelAuthored, whose (float64,
	// bool) return distinguishes "0% authored" from "not measured" specifically
	// so an old, provenance-less result is never silently read as authored.
	DecisionSources    map[string]int     `json:"decision_sources,omitempty"`
	ModelAuthoredShare map[string]float64 `json:"model_authored_share,omitempty"`
	ProvenanceVersion  int                `json:"provenance_version,omitempty"`
	ProviderErrors     map[string]int     `json:"provider_errors"`
	ProviderCalls      map[string]int     `json:"provider_calls"`
	ProviderLatency    map[string]float64 `json:"provider_latency_ms_avg"`
	TokenUsage         map[string]int     `json:"token_usage"`
	CostMicros         map[string]int64   `json:"cost_micros"`
	DurationMillis     int64              `json:"duration_millis"`
	ReplayEvents       int                `json:"replay_events"`
	// ReplayTruncated is true when MaxReplayEvents forced the recorded event
	// stream to drop events mid-match. The stream still carries a
	// ReplayTruncated marker event in that case (see ReconstructSnapshot),
	// so this field is a convenience for callers that only have the
	// MatchResult and not the raw event slice.
	ReplayTruncated bool `json:"replay_truncated,omitempty"`
	// Strata records what the match actually turned out to be -- realised
	// lane count, map type, and balance version -- as opposed to what a
	// ruleset requested. A ruleset with map_type: "" tells you nothing about
	// how many lanes were generated; Strata["lanes"] does. Sweep tooling
	// should report per-stratum using these values, never blend across them.
	Strata map[string]string `json:"strata,omitempty"`
}

func (g *Game) recordReplayEvent(event ReplayEvent) {
	if g == nil {
		return
	}
	event.Tick = g.TickCount
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	if event.Model == "" && event.PlayerID != "" {
		event.Model = g.ModelNames[event.PlayerID]
	}
	g.ReplayEvents = append(g.ReplayEvents, event)
	if g.MaxReplayEvents > 0 && len(g.ReplayEvents) > g.MaxReplayEvents {
		g.trimReplayEvents()
	}
}

// trimReplayEvents enforces MaxReplayEvents once the stream grows past the
// cap. map_init (the board layout) is always preserved, exactly as before.
//
// What is new: whatever else gets discarded is folded into a ReplayTruncated
// marker event kept right after map_init (or at the front, if map_init
// hasn't been recorded yet), instead of just vanishing. The marker itself is
// updated in place on every later trim rather than re-inserted, so this
// stays O(1) per call no matter how long the match runs past the cap.
// ReconstructSnapshot flags Truncated the moment it walks past the marker,
// so a caller reconstructing a window that touches the gap always knows the
// board it got back cannot be trusted -- see replay_snapshot.go.
func (g *Game) trimReplayEvents() {
	n := len(g.ReplayEvents)
	if g.MaxReplayEvents <= 0 || n <= g.MaxReplayEvents {
		return
	}

	prefixLen := 0
	if g.ReplayEvents[0].Type == ReplayMapInit {
		prefixLen = 1
	}
	haveMarker := prefixLen < n && g.ReplayEvents[prefixLen].Type == ReplayTruncated

	firstVictim := prefixLen
	if haveMarker {
		firstVictim++
	}

	reserved := prefixLen + 1 // +1 for the marker itself
	if reserved > g.MaxReplayEvents {
		// MaxReplayEvents is too small to hold map_init plus a marker (not
		// reachable with any realistic cap -- the default is 10000). Fall
		// back to a plain trim so the cap is never violated; the discard
		// signal is lost in this degenerate case only.
		g.ReplayEvents = g.ReplayEvents[n-g.MaxReplayEvents:]
		return
	}

	keepTail := g.MaxReplayEvents - reserved
	victimEnd := n - keepTail
	if victimEnd < firstVictim {
		victimEnd = firstVictim
	}

	discardedNow := victimEnd - firstVictim
	firstDroppedTick := g.ReplayEvents[firstVictim].Tick

	marker := ReplayEvent{Type: ReplayTruncated, Tick: firstDroppedTick}
	totalDiscarded := discardedNow
	if haveMarker {
		prevMarker := g.ReplayEvents[prefixLen]
		marker.Tick = prevMarker.Tick
		if prev, ok := toIntFromAny(prevMarker.Details["discarded_events"]); ok {
			totalDiscarded += prev
		}
	}
	marker.Details = map[string]interface{}{
		"discarded_events": totalDiscarded,
	}

	rest := g.ReplayEvents[victimEnd:]
	kept := make([]ReplayEvent, 0, prefixLen+1+len(rest))
	kept = append(kept, g.ReplayEvents[:prefixLen]...)
	kept = append(kept, marker)
	kept = append(kept, rest...)
	g.ReplayEvents = kept
	g.ReplayTruncated = true
}

// recordMapInitEvent captures the board layout once so replays are
// self-contained and the viewer can redraw the map.
func (g *Game) recordMapInitEvent() {
	paths := make([][][]int, len(g.Paths))
	for i, path := range g.Paths {
		pts := make([][]int, len(path))
		for j, pos := range path {
			pts[j] = []int{pos.Y, pos.X}
		}
		paths[i] = pts
	}
	obstacles := make([][]int, len(g.Obstacles))
	for i, pos := range g.Obstacles {
		obstacles[i] = []int{pos.Y, pos.X}
	}
	g.recordReplayEvent(ReplayEvent{
		Type: ReplayMapInit,
		Details: map[string]interface{}{
			"map_height": g.MapHeight,
			"map_width":  g.MapWidth,
			"paths":      paths,
			"obstacles":  obstacles,
		},
	})
	g.mapInitRecorded = true
}
