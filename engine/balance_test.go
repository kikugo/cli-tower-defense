package engine

import "testing"

func TestComputeBalanceHashStableAcrossRepeatedCalls(t *testing.T) {
	cfg := DefaultBalanceConfig()
	first := ComputeBalanceHash(cfg)
	if first == "" {
		t.Fatalf("expected non-empty hash")
	}
	// Go randomises map iteration order per call, so the only way this test
	// catches a regression to "iterate the map directly, no sort" is by
	// hashing the SAME config many times and requiring identical output
	// every time -- a single repeat has a real chance of getting lucky.
	for i := 0; i < 200; i++ {
		if got := ComputeBalanceHash(cfg); got != first {
			t.Fatalf("hash unstable on iteration %d: got %q, want %q", i, got, first)
		}
	}
}

func TestComputeBalanceHashStableAcrossFreshConfigInstances(t *testing.T) {
	// Two independently-constructed configs with identical content (not the
	// same map underneath) must hash identically -- this is what "stable
	// across runs and processes" means in practice.
	a := ComputeBalanceHash(DefaultBalanceConfig())
	b := ComputeBalanceHash(DefaultBalanceConfig())
	if a != b {
		t.Fatalf("expected identical hashes for identical fresh configs, got %q vs %q", a, b)
	}
}

func TestComputeBalanceHashChangesOnTowerStatChange(t *testing.T) {
	base := DefaultBalanceConfig()
	baseHash := ComputeBalanceHash(base)

	mutated := DefaultBalanceConfig()
	st := mutated.Towers["basic"]
	st.Damage++
	mutated.Towers["basic"] = st

	if got := ComputeBalanceHash(mutated); got == baseHash {
		t.Fatalf("expected hash to change when a tower stat changes, both %q", got)
	}
}

func TestComputeBalanceHashChangesOnEnemyStatChange(t *testing.T) {
	base := DefaultBalanceConfig()
	baseHash := ComputeBalanceHash(base)

	mutated := DefaultBalanceConfig()
	st := mutated.Enemies["basic"]
	st.Health++
	mutated.Enemies["basic"] = st

	if got := ComputeBalanceHash(mutated); got == baseHash {
		t.Fatalf("expected hash to change when an enemy stat changes, both %q", got)
	}
}

func TestComputeBalanceHashChangesOnBreachBountyChange(t *testing.T) {
	base := DefaultBalanceConfig()
	baseHash := ComputeBalanceHash(base)

	mutated := DefaultBalanceConfig()
	mutated.BreachResourceBounty++

	if got := ComputeBalanceHash(mutated); got == baseHash {
		t.Fatalf("expected hash to change when breach resource bounty changes, both %q", got)
	}
}

func TestComputeBalanceHashChangesOnBreachScoreChange(t *testing.T) {
	base := DefaultBalanceConfig()
	baseHash := ComputeBalanceHash(base)

	mutated := DefaultBalanceConfig()
	mutated.BreachScore++

	if got := ComputeBalanceHash(mutated); got == baseHash {
		t.Fatalf("expected hash to change when breach score changes, both %q", got)
	}
}

// TestComputeBalanceHashIgnoresVersionLabel is the whole point of this hash:
// BalanceVersion is a hand-written string (see DefaultBalanceConfig) that
// balance_sweep.go's applyBalanceOverride never updates, so two configs
// with materially different stats but the same Version string must be
// distinguishable -- and two configs with the same stats but different
// Version strings must NOT look different, because the hash describes
// content, not the label.
func TestComputeBalanceHashIgnoresVersionLabel(t *testing.T) {
	a := DefaultBalanceConfig()
	a.Version = "v2"
	b := DefaultBalanceConfig()
	b.Version = "totally-different-label"

	if ComputeBalanceHash(a) != ComputeBalanceHash(b) {
		t.Fatalf("expected hash to ignore Version, got different hashes for configs differing only in Version")
	}
}

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
	// A kill pays score, never resources. Paying spendable resources for kills
	// let the defender fund its next tower from the attacker's spawn spending,
	// which compounded into a 92% defender win rate over 40 scripted seeds.
	if got := g.Resources[g.Defender] - resBefore; got != 0 {
		t.Fatalf("a kill must not award resources, defender gained %d", got)
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
