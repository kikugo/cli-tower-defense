package engine

import "time"

type ReplayEventType string

const (
	ReplayTick        ReplayEventType = "tick"
	ReplayDecision    ReplayEventType = "decision"
	ReplaySpawn       ReplayEventType = "spawn"
	ReplayWave        ReplayEventType = "wave"
	ReplayPlacement   ReplayEventType = "placement"
	ReplayDamage      ReplayEventType = "damage"
	ReplayBreach      ReplayEventType = "breach"
	ReplayResource    ReplayEventType = "resource"
	ReplayGameEnd     ReplayEventType = "game_end"
	ReplayRejected    ReplayEventType = "rejected"
	ReplayProviderErr ReplayEventType = "provider_error"
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
		g.ReplayEvents = g.ReplayEvents[len(g.ReplayEvents)-g.MaxReplayEvents:]
	}
}
