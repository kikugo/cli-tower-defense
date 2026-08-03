package main

// Tests for board_viewport.go: T2.2 in the task brief. renderBoard is the
// live board's viewport renderer -- at full width it must reproduce the
// pre-rewrite board render byte-for-byte (verified against
// testdata/board_full_width.golden, captured from the unmodified code
// before this refactor via `diff` against a scratch capture, see the task
// report for the exact command), and at every other width it must still
// produce exactly rect.h rows of exactly rect.w columns.

import (
	"os"
	"strings"
	"testing"

	eng "tower-defense/engine"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderBoardByteIdenticalAtFullWidth(t *testing.T) {
	g := newScriptedGame(t, "o3", "gpt-4")

	want, err := os.ReadFile("testdata/board_full_width.golden")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	rows := renderBoard(g, rect{w: boardMaxW, h: boardMaxH}, 0, false)
	got := strings.Join(rows, "\n")

	if got != string(want) {
		t.Fatalf("renderBoard at full width (rc={%d,%d}) is not byte-identical to the pre-rewrite board.\ngot:\n%s\nwant:\n%s", boardMaxW, boardMaxH, got, string(want))
	}
}

func TestRenderBoardExactDimensions(t *testing.T) {
	g := newScriptedGame(t, "o3", "gpt-4")

	for w := 40; w <= boardMaxW; w++ {
		rc := rect{w: w, h: boardMaxH}
		rows := renderBoard(g, rc, 0, false)
		if len(rows) != rc.h {
			t.Fatalf("w=%d: got %d rows, want %d", w, len(rows), rc.h)
		}
		for i, row := range rows {
			if gotW := lipgloss.Width(row); gotW != rc.w {
				t.Fatalf("w=%d row %d: width %d, want %d (%q)", w, i, gotW, rc.w, row)
			}
		}
	}
}

func TestRenderBoardHeightVariants(t *testing.T) {
	g := newScriptedGame(t, "o3", "gpt-4")
	for _, h := range []int{0, 1, 2, 13, 14, 15, 16} {
		rc := rect{w: 84, h: h}
		rows := renderBoard(g, rc, 0, false)
		if len(rows) != h {
			t.Fatalf("h=%d: got %d rows, want %d", h, len(rows), h)
		}
		for i, row := range rows {
			if gotW := lipgloss.Width(row); gotW != rc.w {
				t.Fatalf("h=%d row %d: width %d, want %d", h, i, gotW, rc.w)
			}
		}
	}
}

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
