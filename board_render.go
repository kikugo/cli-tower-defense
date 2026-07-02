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

// renderSnapshotBoard styles the grid rows for the replay viewer.
func renderSnapshotBoard(snap eng.ReplaySnapshot, highlight *eng.Position) string {
	grid := buildSnapshotGrid(snap, highlight)
	if grid == nil {
		return ""
	}
	rows := make([]string, len(grid))
	for y, row := range grid {
		var b strings.Builder
		for _, r := range row {
			switch r {
			case '·':
				b.WriteString(pathStyle.Render(string(r)))
			case '⬡':
				b.WriteString(obstacleStyle.Render(string(r)))
			case '^', '⊕', '⌖', 'B':
				glyphType := map[rune]string{'^': "basic", '⊕': "splash", '⌖': "sniper", 'B': "buffer"}[r]
				b.WriteString(towerColor[glyphType].Render(string(r)))
			case '✗':
				b.WriteString(breachStyle.Render(string(r)))
			case '◉':
				b.WriteString(highlightStyle.Render(string(r)))
			default:
				b.WriteRune(r)
			}
		}
		rows[y] = b.String()
	}
	return uiBorder.Render(strings.Join(rows, "\n"))
}
