package main

import (
	"strings"

	eng "tower-defense/engine"

	"github.com/charmbracelet/lipgloss"
)

// towerGlyphs maps tower types to their board glyphs. Shared by the live view
// and the replay board.
var towerGlyphs = map[string]rune{"basic": '^', "splash": '⊕', "sniper": '⌖', "buffer": 'B'}

var (
	breachStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	highlightStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true)
	obstacleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

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
				grid[pos.Y][pos.X] = '·'
			}
		}
	}
	for _, pos := range snap.MapObstacles {
		if inBounds(pos) {
			grid[pos.Y][pos.X] = '⬡'
		}
	}
	for _, t := range snap.Towers {
		glyph, ok := towerGlyphs[t.TowerType]
		if !ok {
			glyph = '^'
		}
		if inBounds(t.Pos) {
			grid[t.Pos.Y][t.Pos.X] = glyph
		}
	}
	for _, pos := range snap.BreachPoints {
		if inBounds(pos) {
			grid[pos.Y][pos.X] = '✗'
		}
	}
	if highlight != nil && inBounds(*highlight) {
		grid[highlight.Y][highlight.X] = '◉'
	}
	return grid
}

// styleSnapshotGridRow renders columns [colStart, colStart+cols) of grid row
// y using the same per-glyph switch renderSnapshotBoard applies to a whole
// row -- factored out so both the legacy whole-board render and the
// viewport-bounded renderReplayBoard share one styling definition.
func styleSnapshotGridRow(grid [][]rune, y, colStart, cols int) string {
	var b strings.Builder
	row := grid[y]
	end := colStart + cols
	if end > len(row) {
		end = len(row)
	}
	for x := colStart; x < end; x++ {
		r := row[x]
		switch r {
		case '·':
			b.WriteString(pathStyle.Render(string(r)))
		case '⬡':
			b.WriteString(obstacleStyle.Render(string(r)))
		case '^', '⊕', '⌖', 'B':
			b.WriteString(towerColor[towerGlyphType[r]].Render(string(r)))
		case '✗':
			b.WriteString(breachStyle.Render(string(r)))
		case '◉':
			b.WriteString(highlightStyle.Render(string(r)))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// renderSnapshotBoard styles the grid rows for the replay viewer at full
// width, with no viewport/panning. Kept for reference and for any caller
// that wants the whole board regardless of terminal size; the live replay
// view uses the budget-bounded renderReplayBoard below instead.
func renderSnapshotBoard(snap eng.ReplaySnapshot, highlight *eng.Position) string {
	grid := buildSnapshotGrid(snap, highlight)
	if grid == nil {
		return ""
	}
	rows := make([]string, len(grid))
	for y := range grid {
		rows[y] = styleSnapshotGridRow(grid, y, 0, len(grid[y]))
	}
	return uiBorder.Render(strings.Join(rows, "\n"))
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
