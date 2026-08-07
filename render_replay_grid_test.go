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
	// The replay board draws from glyphs_v2.go, the same vocabulary the live
	// board uses -- so these are the redesign's characters, not the retired
	// '·' / '⌖' / '⬡' / '✗' / '◉' set this test used to pin.
	if grid[2][1] != pathGlyphV2 {
		t.Fatalf("expected path glyph at 2,1 got %q", grid[2][1])
	}
	if grid[1][1] != towerGlyph("sniper") {
		t.Fatalf("expected sniper glyph at 1,1 got %q", grid[1][1])
	}
	if grid[0][9] != wallGlyphV2 {
		t.Fatalf("expected wall glyph at 0,9 got %q", grid[0][9])
	}
	if grid[2][2] != breachGlyphV2 {
		t.Fatalf("expected breach glyph at 2,2 got %q", grid[2][2])
	}
	if grid[2][0] != highlightGlyphV2 {
		t.Fatalf("expected highlight glyph at 2,0 got %q", grid[2][0])
	}

	// Every glyph the replay board can draw must be one display column, the
	// same rule the live board holds to -- three of the retired ones were
	// not, which is why they went.
	for _, r := range []rune{pathGlyphV2, wallGlyphV2, breachGlyphV2, highlightGlyphV2, towerGlyph("sniper")} {
		if w := frameDisplayWidth(string(r)); w != 1 {
			t.Fatalf("replay glyph %q is %d display columns, want 1", string(r), w)
		}
	}
}

func TestBuildSnapshotGridWithoutMapReturnsNil(t *testing.T) {
	if grid := buildSnapshotGrid(eng.ReplaySnapshot{}, nil); grid != nil {
		t.Fatalf("expected nil grid without map data")
	}
}
