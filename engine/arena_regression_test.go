package engine

import (
	"strings"
	"testing"
)

// runDeterministicMatch drives a fully synchronous scripted match (no async
// decision goroutines) so the outcome is reproducible for a given seed.
func runDeterministicMatch(seed int64, ticks int) *Game {
	g := NewGame("test", "test")
	g.SetMapType("straight")
	g.SetRandomSeed(seed)
	g.PauseBetweenTurns = false

	for i := 0; i < ticks && !g.GameOver; i++ {
		g.UpdateGameState()
		if g.GameOver {
			break
		}
		// Attacker always applies pressure; defender always fortifies.
		g.applyDecision(g.Attacker, "attacker", map[string]interface{}{
			"action":     "spawn",
			"enemy_type": "basic",
		})
		if g.GameOver {
			break
		}
		g.applyDecision(g.Defender, "defender", map[string]interface{}{
			"action":     "place",
			"tower_type": "basic",
			"position":   []interface{}{float64(g.MapHeight / 2), float64(3 + i%20)},
		})
	}
	return g
}

func TestArenaMatchIsDeterministic(t *testing.T) {
	g1 := runDeterministicMatch(99, 400)
	g2 := runDeterministicMatch(99, 400)

	r1 := g1.BuildMatchResult()
	r2 := g2.BuildMatchResult()

	if r1.Winner != r2.Winner {
		t.Fatalf("winner mismatch: %q vs %q", r1.Winner, r2.Winner)
	}
	if r1.Waves != r2.Waves {
		t.Fatalf("wave mismatch: %d vs %d", r1.Waves, r2.Waves)
	}
	if g1.TickCount != g2.TickCount {
		t.Fatalf("tick mismatch: %d vs %d", g1.TickCount, g2.TickCount)
	}
	if len(g1.ReplayEvents) != len(g2.ReplayEvents) {
		t.Fatalf("replay length mismatch: %d vs %d", len(g1.ReplayEvents), len(g2.ReplayEvents))
	}
	for _, p := range []string{g1.Defender, g1.Attacker} {
		if r1.Score[p] != r2.Score[p] {
			t.Fatalf("score mismatch for %s: %d vs %d", p, r1.Score[p], r2.Score[p])
		}
	}
}

func TestArenaExportPipelineCoheres(t *testing.T) {
	g := runDeterministicMatch(7, 500)
	result := g.BuildMatchResult()

	// MatchResult basics.
	if len(result.Models) != 2 {
		t.Fatalf("expected two models in result, got %d", len(result.Models))
	}
	if result.Defender == "" || result.Attacker == "" {
		t.Fatalf("expected defender/attacker set, got %+v", result)
	}
	if result.ReplayEvents != len(g.ReplayEvents) {
		t.Fatalf("replay count mismatch: result=%d game=%d", result.ReplayEvents, len(g.ReplayEvents))
	}

	// Markdown report.
	md := result.MarkdownReport()
	if !strings.Contains(md, "# Match Report") {
		t.Fatalf("markdown missing header:\n%s", md)
	}
	for _, model := range result.Models {
		if !strings.Contains(md, model) {
			t.Fatalf("markdown missing model %q", model)
		}
	}

	// Snapshot reconstruction must agree with the raw event stream.
	snap := ReconstructSnapshot(g.ReplayEvents, len(g.ReplayEvents))
	placements := 0
	spawns := 0
	for _, ev := range g.ReplayEvents {
		switch ev.Type {
		case ReplayPlacement:
			placements++
		case ReplaySpawn:
			spawns++
		}
	}
	if len(snap.Towers) != placements {
		t.Fatalf("snapshot towers %d != placement events %d", len(snap.Towers), placements)
	}
	if snap.TotalEnemiesSpawned() != spawns {
		t.Fatalf("snapshot spawns %d != spawn events %d", snap.TotalEnemiesSpawned(), spawns)
	}
	if len(g.ReplayEvents) > 0 && snap.Tick != g.ReplayEvents[len(g.ReplayEvents)-1].Tick {
		t.Fatalf("snapshot tick %d != last event tick %d", snap.Tick, g.ReplayEvents[len(g.ReplayEvents)-1].Tick)
	}
}

func TestArenaTournamentCSVFromResults(t *testing.T) {
	g := runDeterministicMatch(3, 300)
	result := g.BuildMatchResult()

	results := []TournamentMatchResult{
		{Matchup: "m", Seed: 3, Result: result},
	}
	standings := SortStandings(BuildTournamentStandings(results))
	csv := StandingsCSV(standings)

	lines := strings.Split(strings.TrimSpace(csv), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected header + at least one standing, got %d lines:\n%s", len(lines), csv)
	}
	if !strings.HasPrefix(lines[0], "rank,model,matches,wins,win_rate") {
		t.Fatalf("unexpected CSV header: %s", lines[0])
	}
}
