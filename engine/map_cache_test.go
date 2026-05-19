package engine

import "testing"

func TestPathAndObstacleTileSetsAreRebuilt(t *testing.T) {
	g := NewGame("test", "test")
	g.SetMapType("straight")
	if len(g.PathTileSet) == 0 {
		t.Fatalf("expected non-empty path tile cache")
	}

	before := len(g.ObstacleTileSet)
	g.SetRandomSeed(42)
	after := len(g.ObstacleTileSet)
	if after == 0 {
		t.Fatalf("expected obstacle tile cache to be populated")
	}
	if before == after && len(g.Obstacles) == 0 {
		t.Fatalf("expected obstacles to be generated after reseed")
	}
}

func TestCanPlaceTowerAtUsesCachedPathAndObstacleTiles(t *testing.T) {
	g := NewGame("test", "test")
	g.SetMapType("straight")

	pathPos := g.Paths[0][0]
	if ok, reason := g.canPlaceTowerAt(pathPos.Y, pathPos.X); ok || reason != "on_path" {
		t.Fatalf("expected on_path rejection, got ok=%v reason=%s", ok, reason)
	}

	if len(g.Obstacles) == 0 {
		t.Fatalf("expected generated obstacles")
	}
	obs := g.Obstacles[0]
	if ok, reason := g.canPlaceTowerAt(obs.Y, obs.X); ok || reason != "on_obstacle" {
		t.Fatalf("expected on_obstacle rejection, got ok=%v reason=%s", ok, reason)
	}
}
