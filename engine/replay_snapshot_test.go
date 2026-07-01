package engine

import "testing"

func sampleReplayEvents() []ReplayEvent {
	return []ReplayEvent{
		{Tick: 1, Type: ReplayTick, Details: map[string]interface{}{"wave": 0}},
		{Tick: 2, Type: ReplayPlacement, Position: &Position{Y: 5, X: 10}, Details: map[string]interface{}{"tower_type": "basic"}},
		{Tick: 3, Type: ReplaySpawn, Details: map[string]interface{}{"enemy_type": "fast"}},
		{Tick: 3, Type: ReplaySpawn, Details: map[string]interface{}{"enemy_type": "fast"}},
		{Tick: 4, Type: ReplayWave, Details: map[string]interface{}{"wave": 1}},
		{Tick: 5, Type: ReplayDamage, Details: map[string]interface{}{"enemy_health": 10}}, // hit, not killed
		{Tick: 6, Type: ReplayDamage, Details: map[string]interface{}{"enemy_health": 0}},  // kill
		{Tick: 7, Type: ReplayPlacement, Position: &Position{Y: 6, X: 12}, Details: map[string]interface{}{"tower_type": "sniper"}},
		{Tick: 8, Type: ReplayBreach, Details: map[string]interface{}{"defender_lives": 19}},
		{Tick: 9, Type: ReplayGameEnd, PlayerID: "p2", Reason: "defender_lives_depleted", Details: map[string]interface{}{"winner": "p2", "wave": 1}},
	}
}

func TestReconstructSnapshotFull(t *testing.T) {
	events := sampleReplayEvents()
	snap := ReconstructSnapshot(events, len(events))

	if len(snap.Towers) != 2 {
		t.Fatalf("expected 2 towers, got %d", len(snap.Towers))
	}
	if snap.Towers[0].TowerType != "basic" || snap.Towers[1].TowerType != "sniper" {
		t.Fatalf("unexpected tower types: %+v", snap.Towers)
	}
	if snap.EnemiesSpawned["fast"] != 2 {
		t.Fatalf("expected 2 fast enemies, got %d", snap.EnemiesSpawned["fast"])
	}
	if snap.TotalEnemiesSpawned() != 2 {
		t.Fatalf("expected 2 total enemies, got %d", snap.TotalEnemiesSpawned())
	}
	if snap.Kills != 1 {
		t.Fatalf("expected 1 kill, got %d", snap.Kills)
	}
	if snap.Breaches != 1 {
		t.Fatalf("expected 1 breach, got %d", snap.Breaches)
	}
	if snap.DefenderLives != 19 {
		t.Fatalf("expected 19 defender lives, got %d", snap.DefenderLives)
	}
	if snap.Wave != 1 {
		t.Fatalf("expected wave 1, got %d", snap.Wave)
	}
	if !snap.GameOver || snap.Winner != "p2" || snap.WinReason != "defender_lives_depleted" {
		t.Fatalf("unexpected end state: over=%v winner=%s reason=%s", snap.GameOver, snap.Winner, snap.WinReason)
	}
	if snap.Tick != 9 {
		t.Fatalf("expected tick 9, got %d", snap.Tick)
	}
}

func TestReconstructSnapshotPartial(t *testing.T) {
	events := sampleReplayEvents()
	// After 3 events: one tick, one placement, one spawn.
	snap := ReconstructSnapshot(events, 3)
	if len(snap.Towers) != 1 {
		t.Fatalf("expected 1 tower at index 3, got %d", len(snap.Towers))
	}
	if snap.TotalEnemiesSpawned() != 1 {
		t.Fatalf("expected 1 enemy at index 3, got %d", snap.TotalEnemiesSpawned())
	}
	if snap.GameOver {
		t.Fatalf("did not expect game over at index 3")
	}
	if snap.DefenderLives != -1 {
		t.Fatalf("expected unknown lives before any breach, got %d", snap.DefenderLives)
	}
}

func TestReconstructSnapshotClampsIndex(t *testing.T) {
	events := sampleReplayEvents()
	if snap := ReconstructSnapshot(events, -5); snap.Index != 0 || len(snap.Towers) != 0 {
		t.Fatalf("expected empty snapshot for negative index, got %+v", snap)
	}
	if snap := ReconstructSnapshot(events, 999); snap.Index != len(events) {
		t.Fatalf("expected index clamped to %d, got %d", len(events), snap.Index)
	}
}

func TestSnapshotSummaryLines(t *testing.T) {
	snap := ReconstructSnapshot(sampleReplayEvents(), 3)
	lines := snap.SummaryLines()
	if len(lines) == 0 {
		t.Fatalf("expected summary lines")
	}
	// lives unknown at this point should render as "?"
	found := false
	for _, ln := range lines {
		if ln == "Defender lives: ?" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unknown lives line, got %v", lines)
	}
}
