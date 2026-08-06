package main

// Tests for what survives of board_viewport.go after the redesign cutover:
// the panning primitives (boardViewportWidth, clampPan, autoFollowPanX),
// which renderFramedBoardV2 and renderMapPaneV2 now share.
//
// The byte-identical-to-the-pre-rewrite-board test that used to live here
// went with renderBoard itself. Its golden (testdata/board_full_width.golden)
// pinned the OLD design's glyphs and borders, so keeping it would have meant
// keeping the old renderer alive purely to satisfy a snapshot of a design
// that no longer ships.

import (
	"testing"

	eng "tower-defense/engine"
)

func TestClampPanClampsAtBothEnds(t *testing.T) {
	cases := []struct {
		pan, mapWidth, viewportW, want int
	}{
		{-50, 80, 40, 0},
		{0, 80, 40, 0},
		{40, 80, 40, 40},
		{1000, 80, 40, 40},
		{20, 80, 40, 20},
		{5, 80, 80, 0}, // viewport == map: no panning possible
	}
	for _, c := range cases {
		if got := clampPan(c.pan, c.mapWidth, c.viewportW); got != c.want {
			t.Fatalf("clampPan(%d,%d,%d)=%d want %d", c.pan, c.mapWidth, c.viewportW, got, c.want)
		}
	}
}

func TestAutoFollowPanXTracksLeadingEnemy(t *testing.T) {
	g := eng.NewGame("", "")
	g.MapWidth = 80
	g.Enemies = []*eng.Enemy{
		{Entity: eng.Entity{Pos: eng.Position{Y: 2, X: 10}}, PathIndex: 5},
		{Entity: eng.Entity{Pos: eng.Position{Y: 2, X: 60}}, PathIndex: 40}, // leading
	}
	viewportW := 40
	panX := autoFollowPanX(g, viewportW)
	if panX < 0 || panX > g.MapWidth-viewportW {
		t.Fatalf("autoFollowPanX returned out-of-range pan %d", panX)
	}
	// The leading enemy (x=60) should be inside the panned viewport.
	if 60 < panX || 60 >= panX+viewportW {
		t.Fatalf("autoFollowPanX pan=%d does not keep the leading enemy (x=60) in view [%d,%d)", panX, panX, panX+viewportW)
	}
}

func TestAutoFollowPanXNoEnemiesOrFullWidth(t *testing.T) {
	g := eng.NewGame("", "")
	g.MapWidth = 80
	if got := autoFollowPanX(g, 80); got != 0 {
		t.Fatalf("viewport==map should never pan, got %d", got)
	}
	if got := autoFollowPanX(g, 40); got != 0 {
		t.Fatalf("no enemies should default to pan 0, got %d", got)
	}
}
