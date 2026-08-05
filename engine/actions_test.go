package engine

import (
	"testing"
)

func TestUpdateGameState(t *testing.T) {
	g := NewGame("test", "test")
	g.AIEnabled = false

	// Test resource generation
	initialP1Res := g.Resources[g.Player1]
	initialP2Res := g.Resources[g.Player2]

	for i := 0; i < 10; i++ {
		g.UpdateGameState()
	}

	if g.Resources[g.Player1] <= initialP1Res {
		t.Errorf("Expected resource generation for player 1, got %d", g.Resources[g.Player1])
	}
	if g.Resources[g.Player2] <= initialP2Res {
		t.Errorf("Expected resource generation for player 2, got %d", g.Resources[g.Player2])
	}
}

func TestPlaceTower(t *testing.T) {
	g := NewGame("test", "test")

	// Test placing valid tower
	y, x := 2, 2
	success := g.placeTower(y, x, "basic")
	if !success {
		t.Errorf("Failed to place basic tower at [%d,%d]", y, x)
	}
	if len(g.Towers) != 1 {
		t.Errorf("Expected 1 tower, got %d", len(g.Towers))
	}

	// Test placing on same spot
	success = g.placeTower(y, x, "basic")
	if success {
		t.Errorf("Should not be able to place tower on same spot")
	}

	// Test placing on path
	py, px := g.Paths[0][0].Y, g.Paths[0][0].X
	success = g.placeTower(py, px, "basic")
	if success {
		t.Errorf("Should not be able to place tower on path at [%d,%d]", py, px)
	}
}

func TestPlaceSlowZone(t *testing.T) {
	g := NewGame("test", "test")

	// Test placing valid slow zone on path
	py, px := g.Paths[0][0].Y, g.Paths[0][0].X
	success := g.placeSlowZone(py, px)
	if !success {
		t.Errorf("Failed to place slow zone on path at [%d,%d]", py, px)
	}
	if len(g.SlowZones) != 1 {
		t.Errorf("Expected 1 slow zone, got %d", len(g.SlowZones))
	}

	// Test placing off path
	success = g.placeSlowZone(2, 2)
	if success {
		t.Errorf("Should not be able to place slow zone off path")
	}
}

func TestEnemyShield(t *testing.T) {
	// Create a shielded enemy
	en := NewEnemy(0, 0, "shielded", nil)
	tower := NewTower(0, 0, "basic", nil)
	tower.Damage = 10 // Basic damage

	// With shield = 2, damage should be 10 / (2+1) = 3
	hitEnemies := tower.Attack([]*Enemy{&en})
	if len(hitEnemies) == 0 {
		t.Fatalf("Tower failed to attack")
	}

	expectedHealth := en.MaxHealth - 3
	if en.Health != expectedHealth {
		t.Errorf("Expected health %d, got %d (Shield logic failure)", expectedHealth, en.Health)
	}
}

func TestSpawnWaveIncrementsWaveAndQueuesEnemies(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Attacker] = 1000
	initialWave := g.Wave

	if !g.spawnWave(false) {
		t.Fatalf("expected wave spawn to succeed")
	}
	if g.Wave != initialWave+1 {
		t.Fatalf("expected wave to increment from %d to %d, got %d", initialWave, initialWave+1, g.Wave)
	}
	if len(g.WaveQueue) == 0 {
		t.Fatalf("expected wave queue to contain enemies")
	}
}

func TestDefenderWinsWhenMaxWaveIsCleared(t *testing.T) {
	g := NewGame("test", "test")
	g.Wave = g.MaxWaves
	g.Enemies = nil
	g.WaveQueue = nil
	g.Lives[g.Defender] = 10

	g.UpdateGameState()

	if !g.GameOver {
		t.Fatalf("expected game over when max waves are cleared")
	}
	if g.Winner != g.Defender {
		t.Fatalf("expected defender %q to win, got %q", g.Defender, g.Winner)
	}
}

func TestAttackerWinsWhenEnemyBreachesFinalLife(t *testing.T) {
	g := NewGame("test", "test")
	path := g.Paths[0]
	last := path[len(path)-1]
	e := NewEnemy(last.Y, last.X, "basic", nil)
	e.PathID = 0
	e.PathIndex = len(path) - 1
	g.Enemies = []*Enemy{&e}
	g.Lives[g.Defender] = 1

	g.UpdateGameState()

	if !g.GameOver {
		t.Fatalf("expected game over when final life is breached")
	}
	if g.Winner != g.Attacker {
		t.Fatalf("expected attacker %q to win, got %q", g.Attacker, g.Winner)
	}
}

func TestSpawnWaveRespectsQueueCap(t *testing.T) {
	g := NewGame("test", "test")
	g.MaxWaveQueue = 5
	g.Resources[g.Attacker] = 1000

	if !g.spawnWave(false) {
		t.Fatalf("expected spawnWave success")
	}
	if len(g.WaveQueue) > g.MaxWaveQueue {
		t.Fatalf("wave queue exceeded cap: %d > %d", len(g.WaveQueue), g.MaxWaveQueue)
	}
}

// TestHealerAbilityHealsNearbyDamagedEnemy proves the heal ability itself is
// reachable: a damaged enemy standing next to a healer must gain health on
// the next tick. This is the mechanic attacker_healer exists to exercise --
// it is keyed on Enemy.EnemyType == "healer" (engine/actions.go), which a
// restatted "basic" spawned by attacker_baseline can never trigger, no
// matter what stats it carries.
func TestHealerAbilityHealsNearbyDamagedEnemy(t *testing.T) {
	g := NewGame("test", "test")
	path := g.Paths[0]
	start := path[0]

	healer := g.newEnemy(start.Y, start.X, "healer", nil)
	healer.PathID = 0

	damaged := g.newEnemy(start.Y, start.X, "basic", nil)
	damaged.PathID = 0
	damaged.Health = damaged.MaxHealth - 30
	if damaged.Health <= 0 {
		t.Fatalf("test setup: damaged enemy must start alive, got health %d", damaged.Health)
	}
	preHealHealth := damaged.Health

	g.Enemies = []*Enemy{&healer, &damaged}
	g.WaveQueue = nil // don't let a queued spawn interfere with this tick

	g.UpdateGameState()

	if damaged.Health <= preHealHealth {
		t.Fatalf("expected healer to heal nearby damaged enemy: health before=%d after=%d", preHealHealth, damaged.Health)
	}
}

// TestHealerAbilityHealsNearbyHealer confirms that, per the brief, an
// all-healer force heals itself: healers are enemies too, so a damaged
// healer standing next to another healer must also be healed. This is a
// legitimate composition, not an edge case to special-case away.
func TestHealerAbilityHealsNearbyHealer(t *testing.T) {
	g := NewGame("test", "test")
	path := g.Paths[0]
	start := path[0]

	healerA := g.newEnemy(start.Y, start.X, "healer", nil)
	healerA.PathID = 0

	healerB := g.newEnemy(start.Y, start.X, "healer", nil)
	healerB.PathID = 0
	healerB.Health = healerB.MaxHealth - 20
	preHealHealth := healerB.Health

	g.Enemies = []*Enemy{&healerA, &healerB}
	g.WaveQueue = nil

	g.UpdateGameState()

	if healerB.Health <= preHealHealth {
		t.Fatalf("expected a damaged healer to be healed by a nearby healer: before=%d after=%d", preHealHealth, healerB.Health)
	}
}
