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
	"fmt"
	"strings"

	eng "tower-defense/engine"

	"github.com/charmbracelet/lipgloss"
)

// buildLiveGrid draws the full MapHeight x MapWidth rune grid plus the
// lookups needed to style it, exactly as the pre-rewrite View() did.
func buildLiveGrid(g *eng.Game, showRange bool) (grid [][]rune, towerAt map[string]*eng.Tower, enemyAt map[string]*eng.Enemy, particleAt map[string]*eng.Particle) {
	grid = make([][]rune, g.MapHeight)
	for y := 0; y < g.MapHeight; y++ {
		grid[y] = make([]rune, g.MapWidth)
		for x := range grid[y] {
			grid[y][x] = ' '
		}
	}

	for _, path := range g.Paths {
		for i, pos := range path {
			if pos.Y >= 0 && pos.Y < len(grid) && pos.X >= 0 && pos.X < g.MapWidth {
				char := '·'
				if i > 0 && i < len(path)-1 {
					prev := path[i-1]
					next := path[i+1]
					if prev.Y == next.Y {
						char = '─'
					} else if prev.X == next.X {
						char = '│'
					} else {
						char = '┼'
					}
				}
				for _, sz := range g.SlowZones {
					if sz.Pos.Y == pos.Y && sz.Pos.X == pos.X {
						char = '≋'
						break
					}
				}
				grid[pos.Y][pos.X] = char
			}
		}
	}

	for _, obs := range g.Obstacles {
		if obs.Y >= 0 && obs.Y < len(grid) && obs.X >= 0 && obs.X < g.MapWidth {
			grid[obs.Y][obs.X] = '⬡'
		}
	}

	towerAt = make(map[string]*eng.Tower)
	for _, t := range g.Towers {
		glyph, ok := towerGlyphs[t.TowerType]
		if !ok {
			glyph = '^'
		}
		y, x := t.Pos.Y, t.Pos.X
		if y >= 0 && y < len(grid) && x >= 0 && x < g.MapWidth {
			grid[y][x] = glyph
			key := fmt.Sprintf("%d,%d", y, x)
			towerAt[key] = t
		}
	}

	enemyAt = make(map[string]*eng.Enemy, len(g.Enemies))
	for _, e := range g.Enemies {
		key := fmt.Sprintf("%d,%d", e.Pos.Y, e.Pos.X)
		enemyAt[key] = e
		if e.Pos.Y >= 0 && e.Pos.Y < len(grid) && e.Pos.X >= 0 && e.Pos.X < g.MapWidth {
			grid[e.Pos.Y][e.Pos.X] = e.Char
		}
	}

	for _, p := range g.Particles {
		if p.Pos.Y >= 0 && p.Pos.Y < len(grid) && p.Pos.X >= 0 && p.Pos.X < g.MapWidth {
			grid[p.Pos.Y][p.Pos.X] = p.Char
		}
	}

	if showRange {
		for _, t := range g.Towers {
			for y2 := 0; y2 < g.MapHeight; y2++ {
				for x2 := 0; x2 < g.MapWidth; x2++ {
					dy := y2 - t.Pos.Y
					dx := x2 - t.Pos.X
					if dx*dx+dy*dy <= t.Range*t.Range {
						if grid[y2][x2] == ' ' {
							grid[y2][x2] = '•'
						}
					}
				}
			}
		}
	}

	particleAt = make(map[string]*eng.Particle)
	for _, p := range g.Particles {
		key := fmt.Sprintf("%d,%d", p.Pos.Y, p.Pos.X)
		particleAt[key] = p
	}

	return grid, towerAt, enemyAt, particleAt
}

// styleLiveGridRow renders columns [colStart, colStart+cols) of grid row y,
// applying exactly the same per-glyph styling switch the pre-rewrite View()
// applied to the whole row. Restricting the loop to a column range is what
// makes this a panning viewport rather than a re-implementation: every
// glyph/style decision is identical, only which x values get visited
// changes.
func styleLiveGridRow(grid [][]rune, y, colStart, cols int, towerAt map[string]*eng.Tower, enemyAt map[string]*eng.Enemy, particleAt map[string]*eng.Particle) string {
	var b strings.Builder
	row := grid[y]
	end := colStart + cols
	if end > len(row) {
		end = len(row)
	}
	for x := colStart; x < end; x++ {
		r := row[x]
		key := fmt.Sprintf("%d,%d", y, x)
		if p, ok := particleAt[key]; ok {
			b.WriteString(particleStyle[p.Color].Render(string(p.Char)))
			continue
		}

		switch r {
		case '·', '─', '│', '┼':
			b.WriteString(pathStyle.Render(string(r)))
		case '≋':
			b.WriteString(slowZoneStyle.Render(string(r)))
		case '⬡':
			b.WriteString(obstacleStyle.Render(string(r)))
		case '^', '⊕', '⌖', 'B':
			glyphType := towerGlyphType[r]
			style := towerColor[glyphType]
			if t, ok := towerAt[key]; ok && t.Level > 0 {
				style = style.Copy().Bold(true).Underline(true)
			}
			b.WriteString(style.Render(string(r)))
		case 'o', '>', '□', 'S', 'H':
			e := enemyAt[key]
			style := enemyColorByType["basic"]
			if e != nil {
				style = enemyColorByType[e.EnemyType]
				ratio := 1.0
				if e.MaxHealth > 0 {
					ratio = float64(e.Health) / float64(e.MaxHealth)
				}
				if ratio > 0.7 {
					style = enemyColorGreen
				} else if ratio > 0.3 {
					style = enemyColorYellow
				} else {
					style = enemyColorRed
				}
			}
			b.WriteString(style.Render(string(r)))
		case '•':
			b.WriteString(pathStyle.Render("•"))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// towerGlyphType is the inverse of towerGlyphs (board_render.go), used to
// look up a tower's color style from its board glyph.
var towerGlyphType = map[rune]string{'^': "basic", '⊕': "splash", '⌖': "sniper", 'B': "buffer"}

// slowZoneStyle matches the inline style the pre-rewrite View() used for the
// '≋' slow-zone glyph (color 39, cyan-blue).
var slowZoneStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

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

// renderBoard renders the live game board into exactly rc.h rows of exactly
// rc.w columns each (invariant #5: every pane renders exactly its allotted
// rows). At rc.w >= boardMaxW (84) the full 80-column map fits and panX is
// irrelevant (clampPan forces it to 0); the output there is byte-identical
// to the pre-rewrite board render. Below that, a panX-column window
// (typically from autoFollowPanX) is shown with a pan indicator appended to
// the top border when the view is truncated.
func renderBoard(g *eng.Game, rc rect, panX int, showRange bool) []string {
	if rc.h <= 0 || rc.w <= 0 {
		return blankRows(rc.h, rc.w)
	}

	grid, towerAt, enemyAt, particleAt := buildLiveGrid(g, showRange)

	vw := boardViewportWidth(g.MapWidth, rc.w)
	vh := rc.h - 2
	if vh > g.MapHeight {
		vh = g.MapHeight
	}
	if vh < 0 {
		vh = 0
	}
	panX = clampPan(panX, g.MapWidth, vw)

	rows := make([]string, vh)
	for y := 0; y < vh; y++ {
		rows[y] = styleLiveGridRow(grid, y, panX, vw, towerAt, enemyAt, particleAt)
	}

	bordered := uiBorder.Render(strings.Join(rows, "\n"))
	out := strings.Split(bordered, "\n")
	if vw < g.MapWidth && len(out) > 0 {
		out[0] = panIndicator(out[0], panX, vw, g.MapWidth, rc.w)
	}

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

// panIndicator overlays a short "<n cols hidden>" marker into the board's
// top border row when the viewport doesn't show the whole map, so a panned
// view is visibly distinguishable from a truncated one. It only replaces
// trailing space it can safely reclaim within the border row's own width, so
// it never changes the row's rendered width.
func panIndicator(topBorder string, panX, vw, mapWidth, rcW int) string {
	hiddenLeft := panX
	hiddenRight := mapWidth - vw - panX
	if hiddenLeft <= 0 && hiddenRight <= 0 {
		return topBorder
	}
	marker := fmt.Sprintf(" pan %d-%d/%d ", panX, panX+vw, mapWidth)
	if len(marker) >= rcW || rcW <= 4 {
		return topBorder
	}
	// Splice the marker into the border line without changing its total
	// display width: keep the leftmost and rightmost border cells, replace
	// interior fill.
	runes := []rune(topBorder)
	if len(runes) < 4 {
		return topBorder
	}
	markerRunes := []rune(marker)
	start := 2
	end := start + len(markerRunes)
	if end > len(runes)-1 {
		end = len(runes) - 1
	}
	markerRunes = markerRunes[:end-start]
	copy(runes[start:end], markerRunes)
	return string(runes)
}
