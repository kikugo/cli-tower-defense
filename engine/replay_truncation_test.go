package engine

import (
	"testing"
	"time"
)

// runScriptedDuelForTest mirrors RunScriptedDuel (engine/scripted_duel.go)
// but returns the live *Game instead of just a MatchResult. RunScriptedDuel
// throws the Game away once it has built a MatchResult, so it cannot be used
// here: proving the replay-reconstruction bug (and its fix) requires
// comparing a reconstructed snapshot against the actual, ground-truth board
// state (g.Towers) of the same match, which only the live Game exposes.
func runScriptedDuelForTest(t *testing.T, cfg ScriptedDuelConfig, maxReplayEvents int) *Game {
	t.Helper()
	resolved := ResolvedMatchConfig{
		Player1: ResolvedPlayerModelConfig{PlayerModelConfig: PlayerModelConfig{
			Provider: ProviderScripted, Model: cfg.DefenderScript, APIKeyEnv: "NONE",
		}},
		Player2: ResolvedPlayerModelConfig{PlayerModelConfig: PlayerModelConfig{
			Provider: ProviderScripted, Model: cfg.AttackerScript, APIKeyEnv: "NONE",
		}},
	}
	g := NewGameFromResolvedConfig(resolved)
	g.Balance = cfg.Balance
	g.ApplyRuleset(cfg.Ruleset)
	if maxReplayEvents > 0 {
		g.MaxReplayEvents = maxReplayEvents
	}
	if cfg.Seed != 0 {
		g.SetRandomSeed(cfg.Seed)
	}
	g.PauseBetweenTurns = false
	g.AIDecisionInterval[g.Player1] = 0
	g.AIDecisionInterval[g.Player2] = 0

	maxTicks := cfg.MaxTicks
	if maxTicks <= 0 {
		maxTicks = 400
	}
	ticks := 0
	deadline := time.Now().Add(60 * time.Second)
	for ticks < maxTicks && !g.GameOver && time.Now().Before(deadline) {
		if g.AIThinking[g.Player1] || g.AIThinking[g.Player2] {
			g.HandleAIDecisions()
			time.Sleep(10 * time.Microsecond)
			continue
		}
		g.UpdateGameState()
		g.HandleAIDecisions()
		ticks++
	}
	g.ResolveTimeout()
	return g
}

// liveTowerSnapshot captures the ground-truth board straight from the live
// Game, in placement order, in the same shape ReconstructSnapshot produces.
func liveTowerSnapshot(g *Game) []SnapshotTower {
	out := make([]SnapshotTower, 0, len(g.Towers))
	for _, tw := range g.Towers {
		out = append(out, SnapshotTower{Pos: tw.Pos, TowerType: tw.TowerType})
	}
	return out
}

// TestReconstructSnapshotFlagsTruncationOnRealMatch reproduces the audited
// bug on a real, scripted match: seed 11, defender_baseline vs
// attacker_baseline, the shipped defaults (3000 ticks, 30 waves via
// DefaultArenaRuleset). MaxReplayEvents is set low (40, rather than the
// shipped default of 10000) purely so the real trimming path fires within a
// fast unit test instead of requiring the multi-thousand-tick run the audit
// used -- the mechanism under test (recordReplayEvent's cap enforcement) is
// identical either way.
//
// Before the fix: trimming silently dropped the early event window (bar
// map_init), so a full reconstruction of g.ReplayEvents produced a board
// with fewer towers than the live match actually placed, with no field
// anywhere indicating the result was incomplete. This test's reference to
// ReplaySnapshot.Truncated does not even compile against that code, which
// is itself the clearest demonstration that the guarantee did not exist --
// see the report for the captured `go vet`/`go test` failure output from
// temporarily reverting engine/replay.go, engine/replay_snapshot.go,
// engine/match_result.go and engine/core.go.
func TestReconstructSnapshotFlagsTruncationOnRealMatch(t *testing.T) {
	cfg := ScriptedDuelConfig{
		Seed:           11,
		MaxTicks:       3000,
		Ruleset:        DefaultArenaRuleset(),
		Balance:        DefaultBalanceConfig(),
		DefenderScript: "defender_baseline",
		AttackerScript: "attacker_baseline",
	}
	g := runScriptedDuelForTest(t, cfg, 40)

	if !g.ReplayTruncated {
		t.Fatalf("expected this match (small MaxReplayEvents, long real run) to truncate the replay stream")
	}
	liveTowers := liveTowerSnapshot(g)
	if len(liveTowers) == 0 {
		t.Fatalf("expected the live match to have placed at least one tower")
	}

	full := ReconstructSnapshot(g.ReplayEvents, len(g.ReplayEvents))

	// The core bug: reconstructing from a truncated stream does not, and
	// with this fix still does not, recover the discarded placements. What
	// changes is that the caller is no longer left to discover that the
	// hard way -- Truncated must be true, and the board must indeed be
	// incomplete relative to the live game's actual state (proving the flag
	// is not a false alarm on a board that happens to still be correct).
	if !full.Truncated {
		t.Fatalf("expected reconstruction of a truncated stream to report Truncated=true")
	}
	if full.TruncatedEvents <= 0 {
		t.Fatalf("expected TruncatedEvents > 0, got %d", full.TruncatedEvents)
	}
	if len(full.Towers) >= len(liveTowers) {
		t.Fatalf("expected the truncated reconstruction to be missing towers the live game actually placed: reconstructed=%d live=%d -- if these are equal the repro no longer demonstrates the bug", len(full.Towers), len(liveTowers))
	}

	// A caller reconstructing only up to map_init (index 1) has not walked
	// past the truncation marker yet, so what it received is not wrong --
	// Truncated must reflect that precisely, not blanket-flag every
	// snapshot merely because the underlying stream is truncated somewhere
	// past the requested window.
	mapOnly := ReconstructSnapshot(g.ReplayEvents, 1)
	if mapOnly.Truncated {
		t.Fatalf("did not expect Truncated before the truncation marker is reached")
	}
	if !mapOnly.HasMap {
		t.Fatalf("expected map_init to still be preserved and reconstructable")
	}
}

// TestReconstructSnapshotUntruncatedMatchesLiveBoardExactly is the
// regression half of the fix: a match that never comes close to
// MaxReplayEvents must reconstruct exactly as before -- byte-accurate
// against the live board, and never flagged Truncated.
func TestReconstructSnapshotUntruncatedMatchesLiveBoardExactly(t *testing.T) {
	rs := DefaultArenaRuleset()
	rs.MaxWaves = 3
	cfg := ScriptedDuelConfig{
		Seed:           11,
		MaxTicks:       400,
		Ruleset:        rs,
		Balance:        DefaultBalanceConfig(),
		DefenderScript: "defender_baseline",
		AttackerScript: "attacker_baseline",
	}
	// MaxReplayEvents left at the engine default (10000): this short match
	// comes nowhere near it.
	g := runScriptedDuelForTest(t, cfg, 0)

	if g.ReplayTruncated {
		t.Fatalf("did not expect this short match to truncate the replay stream (got %d events, cap %d)", len(g.ReplayEvents), g.MaxReplayEvents)
	}

	liveTowers := liveTowerSnapshot(g)
	if len(liveTowers) == 0 {
		t.Fatalf("expected the live match to have placed at least one tower")
	}

	full := ReconstructSnapshot(g.ReplayEvents, len(g.ReplayEvents))
	if full.Truncated {
		t.Fatalf("did not expect Truncated on a stream that was never trimmed")
	}
	if full.TruncatedEvents != 0 {
		t.Fatalf("expected TruncatedEvents 0, got %d", full.TruncatedEvents)
	}
	if len(full.Towers) != len(liveTowers) {
		t.Fatalf("reconstructed tower count %d does not match live tower count %d", len(full.Towers), len(liveTowers))
	}
	for i, want := range liveTowers {
		got := full.Towers[i]
		if got.Pos != want.Pos || got.TowerType != want.TowerType {
			t.Fatalf("tower %d mismatch: reconstructed %+v, live %+v", i, got, want)
		}
	}

	result := g.BuildMatchResult()
	if result.ReplayTruncated {
		t.Fatalf("expected MatchResult.ReplayTruncated=false for an untruncated match")
	}
}
