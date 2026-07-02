package engine

import "testing"

func TestDefaultBalanceConfigSanity(t *testing.T) {
	b := DefaultBalanceConfig()
	if b.Version == "" {
		t.Fatalf("expected version set")
	}
	for _, name := range []string{"basic", "sniper", "splash", "buffer"} {
		st, ok := b.Towers[name]
		if !ok || st.Cost <= 0 {
			t.Fatalf("tower %s missing or costless: %+v", name, st)
		}
	}
	for _, name := range []string{"basic", "fast", "tank", "shielded", "healer"} {
		st, ok := b.Enemies[name]
		if !ok || st.Health <= 0 || st.SpawnCost <= 0 {
			t.Fatalf("enemy %s missing or invalid: %+v", name, st)
		}
	}
}

func TestNewTowerHonorsGameBalance(t *testing.T) {
	g := NewGame("test", "test")
	st := g.Balance.Towers["basic"]
	st.Damage = 99
	g.Balance.Towers["basic"] = st
	tw := g.newTower(2, 2, "basic", nil)
	if tw.Damage != 99 {
		t.Fatalf("expected balance damage 99, got %d", tw.Damage)
	}
}

func TestNewEnemyHonorsGameBalance(t *testing.T) {
	g := NewGame("test", "test")
	st := g.Balance.Enemies["basic"]
	st.Health = 42
	g.Balance.Enemies["basic"] = st
	e := g.newEnemy(0, 0, "basic", nil)
	if e.Health != 42 || e.MaxHealth != 42 {
		t.Fatalf("expected balance health 42, got %d/%d", e.Health, e.MaxHealth)
	}
}

func TestBreachBountyHonorsBalance(t *testing.T) {
	g := NewGame("test", "test")
	g.SetMapType("straight")
	g.Balance.BreachResourceBounty = 0
	g.Balance.BreachScore = 7
	path := g.Paths[0]
	e := g.newEnemy(path[len(path)-1].Y, path[len(path)-1].X, "basic", nil)
	e.PathIndex = len(path) - 1
	g.Enemies = append(g.Enemies, &e)
	resBefore := g.Resources[g.Attacker]
	g.UpdateGameState()
	if g.Resources[g.Attacker] != resBefore {
		t.Fatalf("expected zero bounty, resources moved %d -> %d", resBefore, g.Resources[g.Attacker])
	}
	if g.Score[g.Attacker] != 7 {
		t.Fatalf("expected breach score 7, got %d", g.Score[g.Attacker])
	}
}

func TestTowersDoNotShootCorpses(t *testing.T) {
	g := NewGame("test", "test")
	g.SetMapType("straight")
	g.Towers = nil
	g.Enemies = nil

	y := g.MapHeight / 2
	// Two towers covering the same spot.
	t1 := g.newTower(y-1, 10, "basic", nil)
	t2 := g.newTower(y+1, 10, "basic", nil)
	g.Towers = append(g.Towers, &t1, &t2)

	dead := g.newEnemy(y, 10, "basic", nil)
	dead.Health = 0
	alive := g.newEnemy(y, 11, "basic", nil)
	g.Enemies = append(g.Enemies, &dead, &alive)

	g.rebuildEnemySpatialIndex()
	g.runTowerPhase()

	if alive.Health >= alive.MaxHealth {
		t.Fatalf("expected towers to damage the living enemy, health still %d", alive.Health)
	}
}

func TestKillRewardCountedOnce(t *testing.T) {
	g := NewGame("test", "test")
	g.SetMapType("straight")
	g.Towers = nil
	g.Enemies = nil
	g.Score[g.Defender] = 0
	resBefore := g.Resources[g.Defender]

	y := g.MapHeight / 2
	// Two overlapping high-damage towers; a 1 HP enemy dies to the first hit.
	st := g.Balance.Towers["basic"]
	st.Damage = 500
	g.Balance.Towers["basic"] = st
	t1 := g.newTower(y-1, 10, "basic", nil)
	t2 := g.newTower(y+1, 10, "basic", nil)
	g.Towers = append(g.Towers, &t1, &t2)

	victim := g.newEnemy(y, 10, "basic", nil)
	victim.Health = 1
	g.Enemies = append(g.Enemies, &victim)

	g.rebuildEnemySpatialIndex()
	g.runTowerPhase()

	reward := g.Balance.Enemies["basic"].Reward
	if got := g.Score[g.Defender]; got != reward {
		t.Fatalf("expected kill scored exactly once (%d), got %d", reward, got)
	}
	if got := g.Resources[g.Defender] - resBefore; got != reward {
		t.Fatalf("expected kill reward paid exactly once (%d), got %d", reward, got)
	}
}

func TestPlaceTowerRejectsUnknownType(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Defender] = 500
	if g.placeTower(2, 2, "custom") {
		t.Fatalf("custom must not be placeable (not in prompt schema)")
	}
	if g.placeTower(2, 2, "bogus") {
		t.Fatalf("bogus type must be rejected")
	}
}
