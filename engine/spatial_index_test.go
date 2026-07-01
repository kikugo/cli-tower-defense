package engine

import "testing"

func TestEnemySpatialIndexReturnsOnlyNearbyEnemies(t *testing.T) {
	g := NewGame("test", "test")
	closeEnemy := NewEnemy(5, 6, "basic", nil)
	farEnemy := NewEnemy(20, 70, "basic", nil)
	g.Enemies = []*Enemy{&closeEnemy, &farEnemy}
	g.rebuildEnemySpatialIndex()

	nearby := g.enemiesNear(Position{Y: 5, X: 5}, 3)
	if len(nearby) != 1 {
		t.Fatalf("expected exactly one nearby enemy, got %d", len(nearby))
	}
	if nearby[0] != &closeEnemy {
		t.Fatalf("expected close enemy from spatial index")
	}
}

func TestTowerAttackUsesSpatialIndexCandidates(t *testing.T) {
	g := NewGame("test", "test")
	tower := NewTower(5, 5, "basic", nil)
	g.Towers = []*Tower{&tower}
	closeEnemy := NewEnemy(5, 6, "basic", nil)
	farEnemy := NewEnemy(5, 40, "basic", nil)
	g.Enemies = []*Enemy{&closeEnemy, &farEnemy}
	g.rebuildEnemySpatialIndex()

	g.runTowerPhase()

	if closeEnemy.Health >= closeEnemy.MaxHealth {
		t.Fatalf("expected close enemy to be damaged")
	}
	if farEnemy.Health != farEnemy.MaxHealth {
		t.Fatalf("expected far enemy to be untouched")
	}
}
