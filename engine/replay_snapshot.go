package engine

import (
	"encoding/json"
	"fmt"
)

// SnapshotTower is a static tower marker reconstructed from a replay stream.
type SnapshotTower struct {
	Pos       Position `json:"pos"`
	TowerType string   `json:"tower_type"`
}

// ReplaySnapshot is the game state reconstructed by applying replay events up
// to (but not including) a given index. Fields are derived purely from the
// event stream, so values the stream does not carry (for example defender
// lives before the first breach) are reported as unknown.
type ReplaySnapshot struct {
	Index           int             `json:"index"`
	Tick            int64           `json:"tick"`
	Wave            int             `json:"wave"`
	Towers          []SnapshotTower `json:"towers"`
	EnemiesSpawned  map[string]int  `json:"enemies_spawned"`
	WavesLaunched   int             `json:"waves_launched"`
	Breaches        int             `json:"breaches"`
	Kills           int             `json:"kills"`
	Decisions       int             `json:"decisions"`
	RejectedActions int             `json:"rejected_actions"`
	ProviderErrors  int             `json:"provider_errors"`
	DefenderLives   int             `json:"defender_lives"` // -1 until a breach reveals it
	GameOver        bool            `json:"game_over"`
	Winner          string          `json:"winner"`
	WinReason       string          `json:"win_reason"`
	HasMap          bool            `json:"has_map"`
	MapHeight       int             `json:"map_height,omitempty"`
	MapWidth        int             `json:"map_width,omitempty"`
	MapPaths        [][]Position    `json:"map_paths,omitempty"`
	MapObstacles    []Position      `json:"map_obstacles,omitempty"`
	BreachPoints    []Position      `json:"breach_points,omitempty"`
	// Truncated is true when the walk (events[0:index]) passed a
	// ReplayTruncated marker -- i.e. the underlying stream is missing events
	// that happened before this point because MaxReplayEvents forced them
	// out. When true, every count and list above this comment (Towers,
	// EnemiesSpawned, WavesLaunched, Breaches, Kills, Decisions, ...) is a
	// floor, not a fact: the true values could be higher, and placements
	// from the discarded window are simply gone. A caller MUST check this
	// field before treating the snapshot as an accurate board -- it is the
	// only signal that distinguishes a truly empty early game from an early
	// game whose history was discarded.
	Truncated bool `json:"truncated"`
	// TruncatedEvents is the number of events known to have been discarded
	// before the point this snapshot was reconstructed to. Only meaningful
	// when Truncated is true.
	TruncatedEvents int `json:"truncated_events,omitempty"`
}

// ReconstructSnapshot walks the first `index` replay events and derives the
// board and tallies at that point. index is clamped to [0, len(events)].
func ReconstructSnapshot(events []ReplayEvent, index int) ReplaySnapshot {
	if index < 0 {
		index = 0
	}
	if index > len(events) {
		index = len(events)
	}

	snap := ReplaySnapshot{
		Index:          index,
		EnemiesSpawned: map[string]int{},
		DefenderLives:  -1,
	}

	for i := 0; i < index; i++ {
		ev := events[i]
		snap.Tick = ev.Tick

		switch ev.Type {
		case ReplayPlacement:
			if ev.Position != nil {
				towerType, _ := ev.Details["tower_type"].(string)
				snap.Towers = append(snap.Towers, SnapshotTower{Pos: *ev.Position, TowerType: towerType})
			}
		case ReplaySpawn:
			enemyType, _ := ev.Details["enemy_type"].(string)
			if enemyType == "" {
				enemyType = "unknown"
			}
			snap.EnemiesSpawned[enemyType]++
		case ReplayWave:
			snap.WavesLaunched++
			if w, ok := toIntFromAny(ev.Details["wave"]); ok {
				snap.Wave = w
			}
		case ReplayTick:
			if w, ok := toIntFromAny(ev.Details["wave"]); ok {
				snap.Wave = w
			}
		case ReplayBreach:
			snap.Breaches++
			if lives, ok := toIntFromAny(ev.Details["defender_lives"]); ok {
				snap.DefenderLives = lives
			}
			if ev.Position != nil {
				snap.BreachPoints = append(snap.BreachPoints, *ev.Position)
				if len(snap.BreachPoints) > 20 {
					snap.BreachPoints = snap.BreachPoints[len(snap.BreachPoints)-20:]
				}
			}
		case ReplayDamage:
			if health, ok := toIntFromAny(ev.Details["enemy_health"]); ok && health <= 0 {
				snap.Kills++
			}
		case ReplayDecision:
			snap.Decisions++
		case ReplayRejected:
			snap.RejectedActions++
		case ReplayProviderErr:
			snap.ProviderErrors++
		case ReplayMapInit:
			applyMapInit(&snap, ev.Details)
		case ReplayTruncated:
			snap.Truncated = true
			if count, ok := toIntFromAny(ev.Details["discarded_events"]); ok {
				snap.TruncatedEvents = count
			}
		case ReplayGameEnd:
			snap.GameOver = true
			if winner, ok := ev.Details["winner"].(string); ok && winner != "" {
				snap.Winner = winner
			} else {
				snap.Winner = ev.PlayerID
			}
			snap.WinReason = ev.Reason
		}
	}

	return snap
}

// TotalEnemiesSpawned returns the sum across all enemy types.
func (s ReplaySnapshot) TotalEnemiesSpawned() int {
	total := 0
	for _, n := range s.EnemiesSpawned {
		total += n
	}
	return total
}

// applyMapInit copies map layout data from a map_init event into the
// snapshot. A JSON round-trip normalizes both the typed in-memory payload and
// the []interface{} shape produced by loading replay JSON from disk.
func applyMapInit(snap *ReplaySnapshot, details map[string]interface{}) {
	raw, err := json.Marshal(details)
	if err != nil {
		return
	}
	var payload struct {
		MapHeight int       `json:"map_height"`
		MapWidth  int       `json:"map_width"`
		Paths     [][][]int `json:"paths"`
		Obstacles [][]int   `json:"obstacles"`
	}
	if json.Unmarshal(raw, &payload) != nil || payload.MapWidth <= 0 || payload.MapHeight <= 0 {
		return
	}
	snap.MapHeight = payload.MapHeight
	snap.MapWidth = payload.MapWidth
	snap.MapPaths = make([][]Position, len(payload.Paths))
	for i, path := range payload.Paths {
		pts := make([]Position, 0, len(path))
		for _, pt := range path {
			if len(pt) == 2 {
				pts = append(pts, Position{Y: pt[0], X: pt[1]})
			}
		}
		snap.MapPaths[i] = pts
	}
	snap.MapObstacles = make([]Position, 0, len(payload.Obstacles))
	for _, pt := range payload.Obstacles {
		if len(pt) == 2 {
			snap.MapObstacles = append(snap.MapObstacles, Position{Y: pt[0], X: pt[1]})
		}
	}
	snap.HasMap = true
}

// SummaryLines returns compact, render-ready lines describing the snapshot,
// suitable for the replay timeline sidebar.
func (s ReplaySnapshot) SummaryLines() []string {
	lives := "?"
	if s.DefenderLives >= 0 {
		lives = fmt.Sprintf("%d", s.DefenderLives)
	}
	lines := []string{
		fmt.Sprintf("Tick: %d", s.Tick),
		fmt.Sprintf("Wave: %d (launched %d)", s.Wave, s.WavesLaunched),
		fmt.Sprintf("Towers on board: %d", len(s.Towers)),
		fmt.Sprintf("Enemies spawned: %d", s.TotalEnemiesSpawned()),
		fmt.Sprintf("Kills: %d | Breaches: %d", s.Kills, s.Breaches),
		fmt.Sprintf("Defender lives: %s", lives),
		fmt.Sprintf("Decisions: %d | Rejected: %d | Errors: %d", s.Decisions, s.RejectedActions, s.ProviderErrors),
	}
	if s.GameOver {
		lines = append(lines, fmt.Sprintf("Result: %s wins (%s)", s.Winner, humanizeReason(s.WinReason)))
	}
	return lines
}
