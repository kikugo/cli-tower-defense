package main

// This file renders the LIVE match board into a viewport rect. The rune-grid
// construction below (paths, obstacles, towers, enemies, particles, range
// overlay, per-glyph colouring) is moved verbatim from the pre-rewrite
// View() -- the task brief calls that part "fine," only the composition
// around it was broken -- so at full width (rc.w >= boardMaxW) the output is
// byte-identical to what View() used to produce (see
// TestRenderBoardByteIdenticalAtFullWidth, checked against
// testdata/board_full_width.golden captured from the unmodified code before
// this refactor).
//
// What's new is the viewport: the simulation map stays fixed at 80x14
// (engine/core.go:636-638 -- map width sets path length, which sets how much
// of the path a range-5 tower covers, the dominant balance lever; making it
// responsive would make balance depend on window size). Instead, a WINDOW of
// up to 80 columns is rendered, panned horizontally and auto-following the
// leading enemy (highest PathIndex) when the pane is narrower than the map.

import (
	eng "tower-defense/engine"
)

// boardViewportWidth is how many map columns fit in a board pane of width
// rc.w: the pane spends 2 columns on the NormalBorder and 2 on uiBorder's
// Padding(0,1), and can never usefully show more than mapWidth columns.
func boardViewportWidth(mapWidth, rcW int) int {
	vw := rcW - 4
	if vw < 0 {
		vw = 0
	}
	if vw > mapWidth {
		vw = mapWidth
	}
	return vw
}

// clampPan keeps a horizontal pan offset within [0, mapWidth-viewportW].
func clampPan(pan, mapWidth, viewportW int) int {
	maxPan := mapWidth - viewportW
	if maxPan < 0 {
		maxPan = 0
	}
	if pan < 0 {
		return 0
	}
	if pan > maxPan {
		return maxPan
	}
	return pan
}

// autoFollowPanX computes the default pan offset: centered on the leading
// enemy (the one with the greatest path progress), clamped to the map. With
// no enemies on board, or a viewport wide enough to show the whole map, it
// returns 0 (no panning needed/possible).
func autoFollowPanX(g *eng.Game, viewportW int) int {
	if viewportW <= 0 || viewportW >= g.MapWidth {
		return 0
	}
	leadX := -1
	bestProgress := -1
	for _, e := range g.Enemies {
		if e.PathIndex > bestProgress {
			bestProgress = e.PathIndex
			leadX = e.Pos.X
		}
	}
	if leadX < 0 {
		return 0
	}
	return clampPan(leadX-viewportW/2, g.MapWidth, viewportW)
}
