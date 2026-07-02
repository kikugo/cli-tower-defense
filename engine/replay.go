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
	ProviderErrors  map[string]int            `json:"provider_errors"`
	ProviderCalls   map[string]int            `json:"provider_calls"`
	ProviderLatency map[string]float64        `json:"provider_latency_ms_avg"`
	TokenUsage      map[string]int            `json:"token_usage"`
	CostMicros      map[string]int64          `json:"cost_micros"`
	DurationMillis  int64                     `json:"duration_millis"`
	ReplayEvents    int                       `json:"replay_events"`
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
		if g.ReplayEvents[0].Type == ReplayMapInit {
			// Keep the map layout; trim the oldest events after it.
			overflow := len(g.ReplayEvents) - g.MaxReplayEvents
			g.ReplayEvents = append(g.ReplayEvents[:1], g.ReplayEvents[1+overflow:]...)
		} else {
			g.ReplayEvents = g.ReplayEvents[len(g.ReplayEvents)-g.MaxReplayEvents:]
		}
	}
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
