package main

import (
	"strings"

	eng "tower-defense/engine"

	"github.com/charmbracelet/lipgloss"
)

// The replay board draws from the SAME glyph vocabulary as the live board
// (glyphs_v2.go). It used to keep its own table -- '⊕' for splash, '⌖' for
// sniper, '⬡' for a wall, '✗' for a breach -- which meant a replay of a
// match looked like a different game from the match itself, and several of
// those characters are two display cells wide in emoji-capable fonts, the
// exact defect the redesign's one-column rule exists to prevent.
//
// The replay screen's LAYOUT is still computeLayout's, not computeLayoutV2's:
// there is no replay mockup in testdata/mockups, so porting it would mean
// inventing a six-mode design with nothing to check it against. The glyphs
// and palette are shared; the pane arithmetic is not. That split is
// deliberate and is the one place the two designs still meet.

// highlightGlyphV2 marks the tile the currently-selected replay event is
// about. It is not in glyphs_v2.go because nothing else in the UI has a
// concept of a selected tile -- only the replay inspector does.
const highlightGlyphV2 = '@'

var highlightStyle = lipgloss.NewStyle().Reverse(true)

// buildSnapshotGrid draws the reconstructed board into a rune grid.
// Overlay order: path, obstacles, towers, breaches, highlight (topmost).
func buildSnapshotGrid(snap eng.ReplaySnapshot, highlight *eng.Position) [][]rune {
	if !snap.HasMap || snap.MapHeight <= 0 || snap.MapWidth <= 0 {
		return nil
	}
	grid := make([][]rune, snap.MapHeight)
	for y := range grid {
		grid[y] = make([]rune, snap.MapWidth)
		for x := range grid[y] {
			grid[y][x] = ' '
		}
	}
	inBounds := func(p eng.Position) bool {
		return p.Y >= 0 && p.Y < snap.MapHeight && p.X >= 0 && p.X < snap.MapWidth
	}
	for _, path := range snap.MapPaths {
		for _, pos := range path {
			if inBounds(pos) {
				grid[pos.Y][pos.X] = pathGlyphV2
			}
		}
	}
	for _, pos := range snap.MapObstacles {
		if inBounds(pos) {
			grid[pos.Y][pos.X] = wallGlyphV2
		}
	}
	for _, t := range snap.Towers {
		if inBounds(t.Pos) {
			grid[t.Pos.Y][t.Pos.X] = towerGlyph(t.TowerType)
		}
	}
	for _, pos := range snap.BreachPoints {
		if inBounds(pos) {
			grid[pos.Y][pos.X] = breachGlyphV2
		}
	}
	if highlight != nil && inBounds(*highlight) {
		grid[highlight.Y][highlight.X] = highlightGlyphV2
	}
	return grid
}

// styleSnapshotGridRow renders columns [colStart, colStart+cols) of grid row
// y through the shared palette (glyphStyleV2), so a tower is the defender's
// colour in a replay for the same reason it is in a live match. The
// event-highlight marker is the one glyph with no role in that palette, so
// it is handled first and drawn in reverse video -- an SGR attribute, which
// survives a 16-colour terminal where a hue would not.
func styleSnapshotGridRow(grid [][]rune, y, colStart, cols int) string {
	var b strings.Builder
	row := grid[y]
	end := colStart + cols
	if end > len(row) {
		end = len(row)
	}
	for x := colStart; x < end; x++ {
		r := row[x]
		if r == highlightGlyphV2 {
			b.WriteString(highlightStyle.Render(string(r)))
			continue
		}
		b.WriteString(glyphStyleV2(r).Render(string(r)))
	}
	return b.String()
}

// renderReplayBoard renders a reconstructed replay snapshot's board into
// exactly rc.h rows of exactly rc.w columns, the same viewport/pad contract
// renderBoard (board_viewport.go) applies to the live board. It pans to
// center the event's highlighted position (if any) so the pane always shows
// the tile the current event is about, and degrades to blank padded rows
// when the snapshot carries no map (early replay events, before map_init).
func renderReplayBoard(snap eng.ReplaySnapshot, highlight *eng.Position, rc rect) []string {
	grid := buildSnapshotGrid(snap, highlight)
	if grid == nil || rc.h <= 0 || rc.w <= 0 {
		return blankRows(rc.h, rc.w)
	}

	mapH := len(grid)
	mapW := 0
	if mapH > 0 {
		mapW = len(grid[0])
	}

	vw := boardViewportWidth(mapW, rc.w)
	vh := rc.h - 2
	if vh > mapH {
		vh = mapH
	}
	if vh < 0 {
		vh = 0
	}

	panX := 0
	if highlight != nil {
		panX = clampPan(highlight.X-vw/2, mapW, vw)
	}

	rows := make([]string, vh)
	for y := 0; y < vh; y++ {
		rows[y] = styleSnapshotGridRow(grid, y, panX, vw)
	}
	bordered := uiBorder.Render(strings.Join(rows, "\n"))
	out := strings.Split(bordered, "\n")

	final := make([]string, rc.h)
	for i := 0; i < rc.h; i++ {
		if i < len(out) {
			final[i] = padCells(out[i], rc.w)
		} else {
			final[i] = padCells("", rc.w)
		}
	}
	return final
}
