package main

// Tests for render_board_v2.go: the board-pane/legend half of the Phase 2
// terminal UI redesign. See that file's top-of-file doc comment for the
// design rules these tests check, and for why the fit checks below split
// into two families: checkFits/frameDisplayWidth (reused unmodified from
// mockup_fit_test.go) for the ANSI-free bulk of this file's output, and a
// separate lipgloss.Width-based check for the one row that deliberately
// carries an ANSI escape -- the breach marker's reverse video (rule 4).

import (
	"fmt"
	"strings"
	"testing"

	eng "tower-defense/engine"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// --- seeded game state -------------------------------------------------

// buildSeededGameV2 builds a deterministic *eng.Game with a real, fixed
// 80x14 map (via the exported SetMapType, since generatePaths itself is
// unexported to package engine and unreachable from here), several tower
// types, several enemy types, a slow zone, and -- when withBreach is true
// -- a recorded breach event at the path's core end. This is the "one
// seeded game state" the task brief asks the final report to render at
// 160/100/80 columns, and it backs every fit/glyph/panning test below that
// needs real map content rather than an empty board.
func buildSeededGameV2(t *testing.T, withBreach bool) *eng.Game {
	t.Helper()
	g := eng.NewGame("", "")
	g.SetRandomSeed(7)
	g.SetMapType("switchback") // deterministic single path, no rng involved

	if len(g.Paths) == 0 || len(g.Paths[0]) < 10 {
		t.Fatalf("seeded game has no usable path: %+v", g.Paths)
	}
	path := g.Paths[0]
	at := func(i int) eng.Position {
		if i >= len(path) {
			i = len(path) - 1
		}
		return path[i]
	}

	g.Enemies = []*eng.Enemy{
		{Entity: eng.Entity{Pos: at(4)}, EnemyType: "basic"},
		{Entity: eng.Entity{Pos: at(9)}, EnemyType: "fast"},
		{Entity: eng.Entity{Pos: at(14)}, EnemyType: "tank"},
		{Entity: eng.Entity{Pos: at(19)}, EnemyType: "shielded"},
		{Entity: eng.Entity{Pos: at(24)}, EnemyType: "healer"},
	}
	g.SlowZones = []*eng.SlowZone{
		{Pos: at(29)},
	}

	free := freeCellsV2(g, 6)
	towerTypes := []string{"basic", "sniper", "splash", "buffer"}
	g.Towers = nil
	for i, pos := range free {
		g.Towers = append(g.Towers, &eng.Tower{
			Entity:    eng.Entity{Pos: pos},
			TowerType: towerTypes[i%len(towerTypes)],
			Level:     1,
		})
	}

	if withBreach {
		breachPos := path[len(path)-1]
		g.ReplayEvents = append(g.ReplayEvents, eng.ReplayEvent{
			Type:     eng.ReplayBreach,
			Position: &breachPos,
		})
	}

	return g
}

// freeCellsV2 returns up to n map cells that are on neither the path nor an
// obstacle, for placing test towers without colliding with the map's own
// terrain. It reads g.PathTileSet/g.ObstacleTileSet (exported fields,
// keyed "Y,X" -- the same format engine/core.go's unexported tileKey
// produces, confirmed by reading its definition) rather than duplicating
// engine placement-validity logic.
func freeCellsV2(g *eng.Game, n int) []eng.Position {
	var out []eng.Position
	for y := 0; y < g.MapHeight && len(out) < n; y++ {
		for x := 0; x < g.MapWidth && len(out) < n; x++ {
			key := fmt.Sprintf("%d,%d", y, x)
			if _, onPath := g.PathTileSet[key]; onPath {
				continue
			}
			if _, onObstacle := g.ObstacleTileSet[key]; onObstacle {
				continue
			}
			out = append(out, eng.Position{Y: y, X: x})
		}
	}
	return out
}

// fillMaxDensityV2 overwrites g.Towers/g.Enemies so every map cell is
// occupied: every path cell gets an enemy, every non-path cell gets a
// tower. Used by the "full board does not overflow" test (test 5 in the
// brief) -- it does not go through real game placement rules (which would
// reject most of these placements), because the point here is purely to
// stress the RENDERER at maximum glyph density, not to simulate a legal
// match state.
func fillMaxDensityV2(g *eng.Game) {
	pathSet := map[string]bool{}
	for _, path := range g.Paths {
		for _, p := range path {
			pathSet[fmt.Sprintf("%d,%d", p.Y, p.X)] = true
		}
	}
	towerTypes := []string{"basic", "sniper", "splash", "buffer"}
	enemyTypes := []string{"basic", "fast", "tank", "shielded", "healer"}

	g.Towers = nil
	g.Enemies = nil
	ti, ei := 0, 0
	for y := 0; y < g.MapHeight; y++ {
		for x := 0; x < g.MapWidth; x++ {
			pos := eng.Position{Y: y, X: x}
			if pathSet[fmt.Sprintf("%d,%d", y, x)] {
				g.Enemies = append(g.Enemies, &eng.Enemy{Entity: eng.Entity{Pos: pos}, EnemyType: enemyTypes[ei%len(enemyTypes)]})
				ei++
			} else {
				g.Towers = append(g.Towers, &eng.Tower{Entity: eng.Entity{Pos: pos}, TowerType: towerTypes[ti%len(towerTypes)]})
				ti++
			}
		}
	}
}

// --- test 1: fit at every size ------------------------------------------

// TestBoardV2FitsAtEverySize is test 1 from the task brief: sweeping the
// same width/height grid main_layout_v2_test.go's own tests use
// (sweepWidthsV2/sweepHeightsV2, same package), every pane this file owns
// renders exactly its computeLayoutV2 rect's rows and columns, verified via
// checkFits (mockup_fit_test.go). The seeded game carries no breach event,
// so this output is ANSI-free and checkFits' raw-rune width counting
// applies directly -- the ANSI exception is covered separately by
// TestBoardV2BreachMarkerRowWidth below.
func TestBoardV2FitsAtEverySize(t *testing.T) {
	g := buildSeededGameV2(t, false)

	for _, h := range sweepHeightsV2() {
		for _, w := range sweepWidthsV2() {
			l := computeLayoutV2(w, h)

			switch l.mode {
			case modeNotice:
				continue

			case modeCompact, modeMinimum:
				mapRows := renderMapPaneV2(g, rect{w: l.mapPane.w, h: l.mapPane.h}, 0)
				if err := checkFits(strings.Join(mapRows, "\n"), l.mapPane.w, l.mapPane.h); err != nil {
					t.Fatalf("w=%d h=%d mode=%v mapPane: %v", w, h, l.mode, err)
				}
				labelRows := renderLabelRowV2(g, rect{w: l.label.w, h: l.label.h}, sampleTrustStateV2())
				if err := checkFits(strings.Join(labelRows, "\n"), l.label.w, l.label.h); err != nil {
					t.Fatalf("w=%d h=%d mode=%v label: %v", w, h, l.mode, err)
				}

			case modeNarrow, modeMid, modeWide:
				bl, br := boardBottomBorderKeyHintsV2()
				boardRows := renderFramedBoardV2(g, rect{w: l.board.w, h: l.board.h}, 0, bl, br)
				if err := checkFits(strings.Join(boardRows, "\n"), l.board.w, l.board.h); err != nil {
					t.Fatalf("w=%d h=%d mode=%v board: %v", w, h, l.mode, err)
				}
				if l.legend.area() > 0 {
					legendRows := renderLegendV2(g, rect{w: l.legend.w, h: l.legend.h})
					if err := checkFits(strings.Join(legendRows, "\n"), l.legend.w, l.legend.h); err != nil {
						t.Fatalf("w=%d h=%d mode=%v legend: %v", w, h, l.mode, err)
					}
				}
			}
		}
	}
}

// --- test 2: no orphan dividers ------------------------------------------

// TestBoardV2NoOrphanDividers is test 2 from the task brief: the framed
// board's '│' column must sit in exactly the same display column on every
// row, across a spread of heights the layout can actually produce for the
// board pane (board.h ranges roughly 10-16 across the valid w/h grid --
// see computeLayoutV2's payRows(boardMaxH, ...) clamp). Heights below 10
// are deliberately excluded: checkNoOrphanDividers' heuristic ("a genuine
// box edge is used by many rows") false-positives on any box with fewer
// than 3 content rows purely because there ISN'T a third row to use the
// column yet, which is a property of very short boxes, not a jogged
// boundary -- not a case computeLayoutV2 ever actually hands this file for
// the board pane.
func TestBoardV2NoOrphanDividers(t *testing.T) {
	g := buildSeededGameV2(t, false)
	for _, h := range []int{10, 12, 14, 16} {
		bl, br := boardBottomBorderKeyHintsV2()
		rows := renderFramedBoardV2(g, rect{w: boardMaxW, h: h}, 0, bl, br)
		frame := strings.Join(rows, "\n")
		if err := checkNoOrphanDividers(frame); err != nil {
			t.Fatalf("h=%d: %v\nframe:\n%s", h, err, frame)
		}
	}
}

// --- test 3: every glyph is one display column ---------------------------

// TestBoardV2GlyphsAreOneColumn is test 3 from the task brief: every glyph
// this file's renderers can emit is exactly one display column
// (frameDisplayWidth), and none of the eight retired glyphs
// (retiredGlyphsV2) appears anywhere in a maximum-density render of either
// the board or the legend.
func TestBoardV2GlyphsAreOneColumn(t *testing.T) {
	glyphs := []rune{
		towerGlyphV2["basic"], towerGlyphV2["sniper"], towerGlyphV2["splash"], towerGlyphV2["buffer"],
		enemyGlyphV2["basic"], enemyGlyphV2["fast"], enemyGlyphV2["tank"], enemyGlyphV2["shielded"], enemyGlyphV2["healer"],
		rune(pathGlyphV2), rune(flowGlyphV2), rune(wallGlyphV2), rune(slowZoneGlyphV2), rune(breachGlyphV2),
	}
	for _, r := range glyphs {
		if w := frameDisplayWidth(string(r)); w != 1 {
			t.Fatalf("glyph %q is %d display columns, want 1", string(r), w)
		}
	}

	g := buildSeededGameV2(t, false)
	fillMaxDensityV2(g)

	bl, br := boardBottomBorderKeyHintsV2()
	boardRows := renderFramedBoardV2(g, rect{w: boardMaxW, h: boardMaxH}, 0, bl, br)
	boardFrame := strings.Join(boardRows, "\n")

	legendRows := renderLegendV2(g, rect{w: 90, h: 8})
	legendFrame := strings.Join(legendRows, "\n")

	narrowLegendRows := renderLegendV2(g, rect{w: 16, h: 16})
	narrowLegendFrame := strings.Join(narrowLegendRows, "\n")

	for _, retired := range retiredGlyphsV2 {
		if strings.ContainsRune(boardFrame, retired) {
			t.Fatalf("retired glyph %q found in board output", string(retired))
		}
		if strings.ContainsRune(legendFrame, retired) {
			t.Fatalf("retired glyph %q found in wide legend output", string(retired))
		}
		if strings.ContainsRune(narrowLegendFrame, retired) {
			t.Fatalf("retired glyph %q found in narrow legend output", string(retired))
		}
	}
}

// --- test 4: the map is never clipped at 80 columns -----------------------

// TestBoardV2MapNeverClippedAt80 is test 4 from the task brief: at exactly
// w=80 (minimum mode), renderMapPaneV2 must show the full, un-panned map --
// even when a large pan offset is requested, mapViewportWidth(80,80)==80
// forces clampPan's max pan to 0, and every one of the map's 80 columns is
// present.
func TestBoardV2MapNeverClippedAt80(t *testing.T) {
	g := buildSeededGameV2(t, false)
	l := computeLayoutV2(80, 24)
	if l.mode != modeMinimum {
		t.Fatalf("computeLayoutV2(80,24).mode = %v, want modeMinimum", l.mode)
	}
	if l.mapPane.w != 80 {
		t.Fatalf("mapPane.w = %d, want 80", l.mapPane.w)
	}

	got := renderMapPaneV2(g, rect{w: l.mapPane.w, h: l.mapPane.h}, 999) // a large requested pan must still clamp to 0

	grid := buildLiveGridV2(g)
	want := boardContentV2(grid, 0, g.MapWidth, l.mapPane.h)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d: map is clipped/panned at w=80\n got: %q\nwant: %q", i, got[i], want[i])
		}
	}
}

// --- test 5: a full board does not overflow --------------------------------

// TestBoardV2FullDensityNoOverflow is test 5 from the task brief: with
// every map cell occupied by a tower or an enemy (fillMaxDensityV2), every
// pane this file owns still renders exactly its own width/height.
func TestBoardV2FullDensityNoOverflow(t *testing.T) {
	g := buildSeededGameV2(t, false)
	fillMaxDensityV2(g)

	bl, br := boardBottomBorderKeyHintsV2()
	boardRows := renderFramedBoardV2(g, rect{w: boardMaxW, h: boardMaxH}, 0, bl, br)
	if err := checkFits(strings.Join(boardRows, "\n"), boardMaxW, boardMaxH); err != nil {
		t.Fatalf("full density framed board: %v", err)
	}

	mapRows := renderMapPaneV2(g, rect{w: 80, h: 14}, 0)
	if err := checkFits(strings.Join(mapRows, "\n"), 80, 14); err != nil {
		t.Fatalf("full density map pane: %v", err)
	}

	wideLegendRows := renderLegendV2(g, rect{w: 90, h: 8})
	if err := checkFits(strings.Join(wideLegendRows, "\n"), 90, 8); err != nil {
		t.Fatalf("legend (wide) at full density: %v", err)
	}
	narrowLegendRows := renderLegendV2(g, rect{w: 16, h: 16})
	if err := checkFits(strings.Join(narrowLegendRows, "\n"), 16, 16); err != nil {
		t.Fatalf("legend (narrow) at full density: %v", err)
	}
}

// --- the ANSI exception: breach reverse video -----------------------------

// TestBoardV2BreachMarkerRowWidth documents and checks the one deliberate
// ANSI exception this file makes (rule 4: breach markers must not depend
// on hue, so they use lipgloss Reverse rather than a Foreground colour).
// frameDisplayWidth/checkFits do not strip ANSI escapes -- they were built
// for the plain-text mockup fixtures -- so a row containing the breach
// marker is measured with lipgloss.Width instead, matching the convention
// board_viewport_test.go already uses for this project's other ANSI-
// bearing rendered rows (TestRenderBoardExactDimensions,
// TestRenderBoardHeightVariants).
func TestBoardV2BreachMarkerRowWidth(t *testing.T) {
	// go test runs with no attached TTY, so lipgloss's default renderer
	// auto-detects "no colour support" and Render() silently strips ALL
	// styling (Reverse included) -- a termenv/lipgloss environment quirk,
	// not something this file's code does differently at test time vs at
	// runtime in a real terminal. Forcing the ANSI profile for the
	// duration of this one test (restored via defer) is the minimal way to
	// prove the Reverse(true) call in styleGridRowV2 is actually wired up,
	// without changing global rendering behaviour for any other test in
	// this package.
	origProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	defer lipgloss.SetColorProfile(origProfile)

	g := buildSeededGameV2(t, true)

	bl, br := boardBottomBorderKeyHintsV2()
	rows := renderFramedBoardV2(g, rect{w: boardMaxW, h: boardMaxH}, 0, bl, br)
	if len(rows) != boardMaxH {
		t.Fatalf("got %d rows, want %d", len(rows), boardMaxH)
	}
	for i, row := range rows {
		if w := lipgloss.Width(row); w != boardMaxW {
			t.Fatalf("row %d: lipgloss.Width = %d, want %d (%q)", i, w, boardMaxW, row)
		}
	}

	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "\x1b[") {
		t.Fatalf("expected the breach marker to carry a reverse-video ANSI escape sequence, found none in:\n%s", joined)
	}
	if !strings.ContainsRune(joined, breachGlyphV2) {
		t.Fatalf("expected the breach glyph %q somewhere in the board output", string(rune(breachGlyphV2)))
	}
}

// --- demo: render the board at 160/100/80 columns --------------------------

// TestRenderBoardV2Demo is not a pass/fail invariant check -- it exists so
// `go test -run TestRenderBoardV2Demo -v` prints this file's board+legend
// rendering at the three fixture widths (160/100/80) from one seeded game
// state with towers, several enemy types, a slow zone, and a breach marker,
// per the task brief's final report requirement.
func TestRenderBoardV2Demo(t *testing.T) {
	g := buildSeededGameV2(t, true)

	render := func(w, h int) {
		l := computeLayoutV2(w, h)
		t.Logf("=== %dx%d (mode=%v) ===", w, h, l.mode)

		switch l.mode {
		case modeMinimum, modeCompact:
			for _, row := range renderLabelRowV2(g, rect{w: l.label.w, h: l.label.h}, sampleTrustStateV2()) {
				t.Logf("%s", row)
			}
			for _, row := range renderMapPaneV2(g, rect{w: l.mapPane.w, h: l.mapPane.h}, 0) {
				t.Logf("%s", row)
			}

		default: // narrow, mid, wide
			var bl, br string
			if l.mode == modeWide {
				bl, br = boardBottomBorderKeyHintsV2()
			} else {
				bl, br = boardBottomBorderTrustBandV2(sampleTrustStateV2())
			}
			boardRows := renderFramedBoardV2(g, rect{w: l.board.w, h: l.board.h}, 0, bl, br)

			var legendRows []string
			if l.legend.area() > 0 {
				legendRows = renderLegendV2(g, rect{w: l.legend.w, h: l.legend.h})
			}

			for i, row := range boardRows {
				line := row
				if i < len(legendRows) {
					line += " " + legendRows[i]
				}
				t.Logf("%s", line)
			}
		}
	}

	render(160, 50)
	render(100, 30)
	render(80, 24)
}

// sampleTrustStateV2 is the TrustState this file's tests pass wherever the
// board embeds the trust band. It is deliberately a state with something to
// SAY (assists on, two of them fired) rather than a zero value: a zero
// TrustState renders "ENGINE ASSIST UNKNOWN", which is a real state but a
// short one, and a fit test fed only short strings would not notice a border
// that overflows on a realistic one.
func sampleTrustStateV2() TrustState {
	return TrustState{
		AssistKnown:    true,
		AssistsEnabled: true,
		AssistCount:    2,
		AssistDetail:   "queued 4 enemies, fired 1 ability",

		ProvenanceKnown: true,
	}
}
