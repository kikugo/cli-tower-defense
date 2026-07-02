package main

import (
	"testing"

	eng "tower-defense/engine"
)

func TestBuildSnapshotGridPlacesGlyphs(t *testing.T) {
	snap := eng.ReplaySnapshot{
		HasMap:    true,
		MapHeight: 5,
		MapWidth:  10,
		MapPaths: [][]eng.Position{
			{{Y: 2, X: 0}, {Y: 2, X: 1}, {Y: 2, X: 2}},
		},
		MapObstacles: []eng.Position{{Y: 0, X: 9}},
		Towers: []eng.SnapshotTower{
			{Pos: eng.Position{Y: 1, X: 1}, TowerType: "sniper"},
		},
		BreachPoints: []eng.Position{{Y: 2, X: 2}},
	}
	grid := buildSnapshotGrid(snap, &eng.Position{Y: 2, X: 0})
	if grid == nil {
		t.Fatalf("expected grid")
	}
	if grid[2][1] != '·' {
		t.Fatalf("expected path glyph at 2,1 got %q", grid[2][1])
	}
	if grid[1][1] != '⌖' {
		t.Fatalf("expected sniper glyph at 1,1 got %q", grid[1][1])
	}
	if grid[0][9] != '⬡' {
		t.Fatalf("expected obstacle glyph at 0,9 got %q", grid[0][9])
	}
	if grid[2][2] != '✗' {
		t.Fatalf("expected breach glyph at 2,2 got %q", grid[2][2])
	}
	if grid[2][0] != '◉' {
		t.Fatalf("expected highlight glyph at 2,0 got %q", grid[2][0])
	}
}

func TestBuildSnapshotGridWithoutMapReturnsNil(t *testing.T) {
	if grid := buildSnapshotGrid(eng.ReplaySnapshot{}, nil); grid != nil {
		t.Fatalf("expected nil grid without map data")
	}
}
