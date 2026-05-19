package engine

import "testing"

func TestSetRandomSeedProducesDeterministicPaths(t *testing.T) {
	g1 := NewGame("test", "test")
	g2 := NewGame("test", "test")

	g1.SetRandomSeed(42)
	g2.SetRandomSeed(42)

	if len(g1.Paths) != len(g2.Paths) {
		t.Fatalf("expected same path count for same seed")
	}
	for i := range g1.Paths {
		if len(g1.Paths[i]) != len(g2.Paths[i]) {
			t.Fatalf("expected same path length for path %d", i)
		}
		for j := range g1.Paths[i] {
			if g1.Paths[i][j] != g2.Paths[i][j] {
				t.Fatalf("path mismatch at [%d][%d]", i, j)
			}
		}
	}
}

func TestSetMapTypeStraightCreatesSingleStraightLane(t *testing.T) {
	g := NewGame("test", "test")
	g.SetMapType("straight")

	if len(g.Paths) != 1 {
		t.Fatalf("expected one path, got %d", len(g.Paths))
	}
	for _, pos := range g.Paths[0] {
		if pos.Y != g.MapHeight/2 {
			t.Fatalf("expected straight path y=%d, got %d", g.MapHeight/2, pos.Y)
		}
	}
}

func TestSetMapTypeForkedCreatesTwoLanes(t *testing.T) {
	g := NewGame("test", "test")
	g.SetMapType("forked")

	if len(g.Paths) != 2 {
		t.Fatalf("expected two paths, got %d", len(g.Paths))
	}
}
