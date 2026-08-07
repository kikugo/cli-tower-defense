package main

// This file is the board-pane and legend half of the Phase 2 terminal UI
// redesign (see main_layout_v2.go for the layout half). It renders the
// three panes this phase assigns to this file:
//
//   - layout.board    -- the framed 84-column board (narrow/mid/wide modes)
//   - layout.mapPane  -- the borderless 80-column map (minimum/compact modes)
//   - layout.label    -- the borderless map's one-row title (minimum/compact)
//   - layout.legend   -- the glyph legend (mid/wide modes only)
//
// Nothing in this file is wired into main.go's View() yet -- exactly like
// computeLayoutV2 in main_layout_v2.go, the cutover happens in a later
// phase once every pane's v2 renderer exists. These functions are meant to
// be called by that future integration code (and, in the meantime, by this
// file's own tests and TestRenderBoardV2Demo, which prints the board at the
// three fixture sizes for visual review).
//
// --- Rule 1: ownership is the primary encoding, not type -------------------
//
// Every glyph belongs to a side, and the glyph CLASS alone says which:
// punctuation (^ ! * +) is always a tower, lowercase letters (o f t s h) are
// always an enemy. No ANSI colour is applied to glyphs in this phase (that
// is Phase 3) -- the one deliberate exception is the breach marker's reverse
// video (rule 4 below), which is a monochrome-safe SGR attribute, not a
// colour. The code is structured so Phase 3 can add a Foreground() call at
// each glyph's single point of styling (styleGridRowV2) without touching
// layout, panning, or glyph-selection logic at all.
//
// --- Rule 2: every glyph is exactly one display column ----------------------
//
// The old glyph set (⬡ ⌖ ⊕ ≋ □ ✗ ♥ ⛁) is retired -- several of those render
// two cells wide in emoji-capable fonts and break the grid. TestBoardV2NoRetiredGlyphs
// asserts none of them appear anywhere in this file's output; every glyph
// this file DOES use (^ ! * + o f t s h . > # ~ X) is a single ASCII byte,
// hence trivially one display column, which TestBoardV2GlyphsAreOneColumn
// checks via frameDisplayWidth anyway so that guarantee is verified, not
// just asserted by inspection.
//
// --- Rule 3: the path is texture, not structure ------------------------------
//
// Where the pre-rewrite board drew direction-sensitive path fragments
// (┼─┼), this file draws a single dim '.' with a '>' flow tick every
// flowTickIntervalV2 tiles along each path -- a texture that survives
// monochrome and never competes visually with the units standing on it.
//
// --- Rule 4: breach markers do not depend on hue -----------------------------
//
// A breach renders as 'X' in reverse video (styleBreachV2, an SGR attribute
// plus an additive red). The attribute survives at profile ANSI -- a
// 16-colour or genuinely monochrome terminal -- where a hue-only alert would
// be lost.
//
// It does NOT survive at profile Ascii: termenv drops every escape sequence
// there, attributes included, and the marker renders as a bare 'X'. An
// earlier version of this comment claimed otherwise; TestBreachSurvivesMonochrome
// measures both cases. The design still holds, for a different reason than
// the one originally given: profile Ascii means plain text was asked for,
// and 'X' is used for nothing else in the glyph vocabulary, so the character
// carries the alert on its own. The test asserts that uniqueness rather than
// leaving it to inspection.
//
// Since Phase 3 every glyph on the board carries a colour (see
// styleGridRowV2), so board rows must be measured with lipgloss.Width, never
// with mockup_fit_test.go's frameDisplayWidth -- that helper was built for
// the plain-text mockup fixtures and counts escape bytes as columns. This is
// the same convention board_viewport_test.go already uses.
//
// --- Rule 5 / coupling with the header agent ---------------------------------
//
// The board's top border always carries "SPAWN >>" and "CORE n/m" (this
// file's own data, not the header agent's). The BOTTOM border and the
// minimum/compact label row are where the trust band ("ENGINE HELPED 2x",
// "ENGINE ASSIST OFF", ...) appears when the header pane doesn't have a row
// of its own for it -- see testdata/mockups/100x30.txt line 19 (mid mode's
// board bottom border literally reads "ENGINE HELPED 2x ... authored
// 84/91, measured") and testdata/mockups/trust-80.txt / 80x24.txt (the
// borderless label row packs the same information in). Wide mode's header
// has a dedicated row for this (160x50.txt line 4), so wide's board bottom
// border instead carries key hints (160x50.txt line 20: "? legend ...
// r range"); narrow mode's header (2 rows, same budget as
// minimum/compact's) has no room for its own trust row either, so this file
// treats narrow the same as mid -- an inference, not something confirmed by
// a fixture (no narrow-width mockup exists), and called out here explicitly
// per the task brief's "anything that turned out to be false" ask.
//
// render_header_v2.go did not exist when this file was first written, so the
// trust band arrived here as trustBandTextV2, a package-level func variable
// taking an *eng.Game and defaulting to returning "". That seam is now
// CLOSED: render_header_v2.go owns the trust band's vocabulary (TrustState
// and its assistLabel/provenanceLabel), and this file calls TrustBandLabel
// with a TrustState the caller supplies.
//
// The func-variable indirection is gone rather than rewired, and the reason
// matters. It let this file build and test alone while two agents worked in
// parallel, but it also meant the board could silently render an EMPTY trust
// band forever if nobody ever assigned the variable -- a UI whose entire
// purpose is to disclose engine intervention, defaulting to disclosing
// nothing, with no compile error to say so. Taking a TrustState as a
// parameter makes the caller pass one or fail to build.

import (
	"fmt"
	"strings"

	eng "tower-defense/engine"
)

// --- glyphs ---------------------------------------------------------------
//
// The glyph vocabulary (towerGlyphV2, enemyGlyphV2, the terrain constants,
// retiredGlyphsV2) now lives in glyphs_v2.go, shared with render_feed_v2.go
// and the legend. Rules 1 and 2 above are stated in full there.

// flowTickIntervalV2 controls how often a '>' flow tick punctuates an
// otherwise plain '.' path run (rule 3): texture, not structure. It lives
// here rather than in glyphs_v2.go because it is about how THIS file draws
// a path, not about what any glyph means -- no other renderer has a path to
// punctuate.
const flowTickIntervalV2 = 5

// --- coupling seam with render_header_v2.go --------------------------------

// (The trust band's text now comes from render_header_v2.go's
// TrustBandLabel, called with a TrustState the caller passes in -- see the
// top-of-file note on why the old func-variable seam was removed rather than
// rewired.)

// --- grid construction ------------------------------------------------------

// inBoundsV2 reports whether p is a valid cell of a h x w grid.
func inBoundsV2(p eng.Position, h, w int) bool {
	return p.Y >= 0 && p.Y < h && p.X >= 0 && p.X < w
}

// recentBreachPositionsV2 collects breach positions from g.ReplayEvents,
// capped at the last 20 -- the same capped-history convention
// engine/replay_snapshot.go's snapshot reconstruction uses for its own
// BreachPoints field, so a live board's breach markers behave the same way
// a replay's eventually would once replay support is wired to this
// renderer. This reads engine/replay.go's exported ReplayEvent/ReplayBreach
// (already used elsewhere in this package, e.g. view_render.go's
// isMoveFeedEvent) rather than any new engine API.
func recentBreachPositionsV2(g *eng.Game) []eng.Position {
	var out []eng.Position
	for _, ev := range g.ReplayEvents {
		if ev.Type == eng.ReplayBreach && ev.Position != nil {
			out = append(out, *ev.Position)
		}
	}
	if len(out) > 20 {
		out = out[len(out)-20:]
	}
	return out
}

// buildLiveGridV2 draws the full MapHeight x MapWidth rune grid using the
// redesign's glyph set. Overlay order (back to front): path/flow ticks,
// slow zones, obstacles, towers, enemies, breach markers -- enemies sit on
// top of towers/terrain because "units are the story" (rule 3), and breach
// markers sit on top of everything because they are the one alert that must
// never be obscured.
// overlayTowerRangesV2 paints rangeGlyphV2 on every EMPTY cell within some
// tower's firing range. Backs the 'r' key.
//
// It runs after the terrain layers and before towers/enemies/breaches, and
// only writes to cells still holding a space, so the overlay can never hide
// a unit, a path or a breach marker -- it fills the gaps between them, which
// is exactly the question "what does my defence actually cover" asks.
//
// Euclidean distance, matching enemiesNear in the engine: a range overlay
// that disagreed with the engine's own targeting would be worse than none.
func overlayTowerRangesV2(grid [][]rune, g *eng.Game) {
	for _, t := range g.Towers {
		r := t.Range
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if dy*dy+dx*dx > r*r {
					continue
				}
				p := eng.Position{Y: t.Pos.Y + dy, X: t.Pos.X + dx}
				if !inBoundsV2(p, g.MapHeight, g.MapWidth) {
					continue
				}
				if grid[p.Y][p.X] == ' ' {
					grid[p.Y][p.X] = rangeGlyphV2
				}
			}
		}
	}
}

func buildLiveGridV2(g *eng.Game) [][]rune {
	grid := make([][]rune, g.MapHeight)
	for y := range grid {
		grid[y] = make([]rune, g.MapWidth)
		for x := range grid[y] {
			grid[y][x] = ' '
		}
	}

	for _, path := range g.Paths {
		for i, pos := range path {
			if !inBoundsV2(pos, g.MapHeight, g.MapWidth) {
				continue
			}
			ch := rune(pathGlyphV2)
			if i > 0 && i%flowTickIntervalV2 == 0 {
				ch = flowGlyphV2
			}
			grid[pos.Y][pos.X] = ch
		}
	}

	for _, sz := range g.SlowZones {
		if inBoundsV2(sz.Pos, g.MapHeight, g.MapWidth) {
			grid[sz.Pos.Y][sz.Pos.X] = slowZoneGlyphV2
		}
	}

	for _, obs := range g.Obstacles {
		if inBoundsV2(obs, g.MapHeight, g.MapWidth) {
			grid[obs.Y][obs.X] = wallGlyphV2
		}
	}

	for _, t := range g.Towers {
		if inBoundsV2(t.Pos, g.MapHeight, g.MapWidth) {
			grid[t.Pos.Y][t.Pos.X] = towerGlyph(t.TowerType)
		}
	}

	for _, e := range g.Enemies {
		if inBoundsV2(e.Pos, g.MapHeight, g.MapWidth) {
			grid[e.Pos.Y][e.Pos.X] = enemyGlyph(e.EnemyType)
		}
	}

	for _, pos := range recentBreachPositionsV2(g) {
		if inBoundsV2(pos, g.MapHeight, g.MapWidth) {
			grid[pos.Y][pos.X] = breachGlyphV2
		}
	}

	return grid
}

// styleGridRowV2 renders columns [colStart, colStart+cols) of grid row y,
// applying the Phase 3 palette: each glyph goes through glyphStyleV2
// (render_theme_v2.go), which colours it by ROLE -- defender, attacker,
// terrain, breach -- and nothing else.
//
// Consecutive glyphs sharing a style are batched into one Render call
// rather than styled individually. That is not a micro-optimisation: a
// styled 80-column row of terrain would otherwise carry 80 pairs of escape
// sequences, roughly 800 bytes per row and 12KB per frame, for a board
// that is mostly long runs of the same colour.
//
// Every row this produces can carry ANSI, so it must be measured with
// lipgloss.Width, never with mockup_fit_test.go's frameDisplayWidth (which
// counts escape bytes as columns). See TestBoardV2FitsAtEverySize.
func styleGridRowV2(grid [][]rune, y, colStart, cols int) string {
	row := grid[y]
	end := colStart + cols
	if end > len(row) {
		end = len(row)
	}
	if colStart >= end {
		return ""
	}

	var b strings.Builder
	runStart := colStart
	runStyle := glyphStyleV2(row[colStart])
	flush := func(upto int) {
		var run strings.Builder
		for x := runStart; x < upto; x++ {
			run.WriteRune(row[x])
		}
		b.WriteString(runStyle.Render(run.String()))
	}

	for x := colStart + 1; x < end; x++ {
		st := glyphStyleV2(row[x])
		if st.String() == runStyle.String() {
			continue
		}
		flush(x)
		runStart, runStyle = x, st
	}
	flush(end)
	return b.String()
}

// boardContentV2 returns vh rows of vw glyph columns each, panned to start
// at panX -- the bare map content, shared by the framed board renderer and
// the borderless map renderer below so panning/rendering logic exists in
// exactly one place.
func boardContentV2(grid [][]rune, panX, vw, vh int) []string {
	rows := make([]string, vh)
	for y := 0; y < vh; y++ {
		rows[y] = styleGridRowV2(grid, y, panX, vw)
	}
	return rows
}

// --- viewport width ----------------------------------------------------------

// mapViewportWidth is boardViewportWidth's (view_render.go) borderless
// counterpart: the minimum/compact map pane has no border or padding
// overhead (rule 6 -- the border is dropped entirely below narrow width),
// so every column of rcW is a usable map column, still capped at the map's
// own width. The panning math itself (clampPan, autoFollowPanX) is reused
// unchanged from view_render.go; only the "how many columns are available"
// calculation differs from the framed board's, which is why this is a
// separate function rather than a reimplementation of boardViewportWidth.
func mapViewportWidth(mapWidth, rcW int) int {
	if rcW < 0 {
		rcW = 0
	}
	if rcW > mapWidth {
		return mapWidth
	}
	return rcW
}

// --- borderless map pane (minimum/compact modes) ----------------------------

// renderMapPaneV2 renders the live board into layout.mapPane: no border
// chrome at all, exactly rc.h rows of exactly rc.w columns. At rc.w==80
// (minimum mode) mapViewportWidth returns the full 80-column map and
// clampPan collapses any panX to 0, so the map is never clipped there (test
// 4 in the brief); at rc.w<80 (compact mode) this pans exactly like the
// live board does, via the same clampPan/autoFollowPanX primitives.
func renderMapPaneV2(g *eng.Game, rc rect, panX int, showRange bool) []string {
	if rc.h <= 0 || rc.w <= 0 {
		return blankRows(rc.h, rc.w)
	}
	grid := buildLiveGridV2(g)
	if showRange {
		overlayTowerRangesV2(grid, g)
	}

	vw := mapViewportWidth(g.MapWidth, rc.w)
	vh := rc.h
	if vh > g.MapHeight {
		vh = g.MapHeight
	}
	panX = clampPan(panX, g.MapWidth, vw)

	rows := boardContentV2(grid, panX, vw, vh)

	final := make([]string, rc.h)
	for i := 0; i < rc.h; i++ {
		if i < len(rows) {
			final[i] = padCells(rows[i], rc.w)
		} else {
			final[i] = padCells("", rc.w)
		}
	}
	return final
}

// renderLabelRowV2 renders layout.label, the borderless map's one-row
// title: wave/tick, the trust band (via TrustBandLabel), the leaked
// summary, and the SPAWN>>/CORE markers, all packed into one row -- see
// testdata/mockups/trust-80.txt and 80x24.txt for the packed format this
// approximates. There is no per-test byte-exact content requirement for
// this row (the task brief's tests are fit/glyph/panning invariants, not
// content matches), so this builds a reasonable single line and lets
// truncateCells/padCells enforce the width contract regardless of how much
// of the line fits at the narrow end of compact mode's range (60 columns).
func renderLabelRowV2(g *eng.Game, rc rect, trust TrustState, maxTick int64) []string {
	if rc.h <= 0 {
		return blankRows(rc.h, rc.w)
	}
	if rc.w <= 0 {
		return blankRows(rc.h, rc.w)
	}

	defID := g.Defender
	// The tick ceiling comes from the CALLER, not from g: MaxTicks is a run
	// configuration, not simulation state, and there is no field on eng.Game
	// holding it. Passing 0 renders "no cap" rather than a fabricated "/0".
	wave := fmt.Sprintf("W%d/%d %s", g.Wave, g.MaxWaves, fmtTick(g.TickCount, maxTick))

	segments := []string{wave}
	if tb := TrustBandLabel(trust); tb != "" {
		segments = append(segments, tb)
	}
	segments = append(segments, fmt.Sprintf("SPAWN >> CORE %d/%d", g.Lives[defID], g.StartingLives))

	text := "─ " + strings.Join(segments, " ─ ") + " ─"
	row := padCells(truncateCells(text, rc.w), rc.w)

	rows := make([]string, rc.h)
	for i := range rows {
		if i == 0 {
			rows[i] = row
		} else {
			rows[i] = padCells("", rc.w)
		}
	}
	return rows
}

// --- titled dash rules -------------------------------------------------------

// runeColsV2 measures s in display columns, for the narrow purpose of
// titledRuleV2's own arithmetic below: every string this file ever passes
// to it is built from plain ASCII plus the single-column box-drawing dash
// '─' (never a combining mark or an East-Asian-Wide rune), so a plain rune
// count is exact here. This is deliberately NOT a reuse of
// mockup_fit_test.go's frameDisplayWidth -- that helper lives in a _test.go
// file and is unavailable to non-test code (go build ./... cannot see it);
// frameDisplayWidth is still the authority this file's own tests check
// titledRuleV2's OUTPUT against, so any mismatch between this narrow
// counting and the real contract would be caught there, not silently
// trusted here.
func runeColsV2(s string) int {
	return len([]rune(s))
}

// titledRuleV2 builds a single exact-w-column dash rule of the form
// "─ LEFT ── ... ── RIGHT ─", degrading gracefully (dropping RIGHT, then
// LEFT, then finally just returning w plain dashes) when w is too narrow
// for both labels. Used for the framed board's top/bottom border interiors
// and the wide legend's own divider row. The result is always finished
// with padCells so the exact-width guarantee holds regardless of any
// rounding in runeColsV2's measurement.
func titledRuleV2(w int, left, right string) string {
	if w <= 0 {
		return ""
	}
	const dash = "─"

	leftSeg := dash
	if left != "" {
		leftSeg = dash + " " + left + " "
	}
	rightSeg := dash
	if right != "" {
		rightSeg = " " + right + " " + dash
	}

	fillW := w - runeColsV2(leftSeg) - runeColsV2(rightSeg)
	if fillW < 0 {
		rightSeg = dash
		fillW = w - runeColsV2(leftSeg) - runeColsV2(rightSeg)
	}
	if fillW < 0 {
		leftSeg = dash
		fillW = w - runeColsV2(leftSeg) - runeColsV2(rightSeg)
	}
	if fillW < 0 {
		fillW = 0
	}

	row := leftSeg + strings.Repeat(dash, fillW) + rightSeg
	return padCells(row, w)
}

// --- framed board pane (narrow/mid/wide modes) ------------------------------

// renderFramedBoardV2 renders layout.board: the live map inside an 84-wide
// (boardMaxW) frame whose top border always carries "SPAWN >>"/"CORE n/m"
// and whose bottom border carries whatever the caller passes as
// bottomLeft/bottomRight -- key hints in wide mode, TrustBandLabel's
// output in mid/narrow mode per this file's top-of-file coupling note.
// Produces exactly rc.h rows of exactly rc.w columns, matching the
// contract every other pane renderer in this codebase follows.
func renderFramedBoardV2(g *eng.Game, rc rect, panX int, bottomLeft, bottomRight string, showRange bool) []string {
	if rc.h < 2 || rc.w < 2 {
		return blankRows(rc.h, rc.w)
	}

	grid := buildLiveGridV2(g)
	if showRange {
		overlayTowerRangesV2(grid, g)
	}

	vw := boardViewportWidth(g.MapWidth, rc.w)
	vh := rc.h - 2
	if vh > g.MapHeight {
		vh = g.MapHeight
	}
	if vh < 0 {
		vh = 0
	}
	panX = clampPan(panX, g.MapWidth, vw)
	content := boardContentV2(grid, panX, vw, vh)

	innerW := rc.w - 2 // between the two border columns
	defID := g.Defender
	topLeft := "SPAWN >>"
	topRight := fmt.Sprintf("CORE %d/%d", g.Lives[defID], g.StartingLives)

	out := make([]string, 0, rc.h)
	out = append(out, "┌"+titledRuleV2(innerW, topLeft, topRight)+"┐")
	for i := 0; i < vh; i++ {
		out = append(out, "│ "+padCells(content[i], vw)+" │")
	}
	out = append(out, "└"+titledRuleV2(innerW, bottomLeft, bottomRight)+"┘")

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

// boardBottomBorderKeyHintsV2 is the wide-mode bottom-border text (see
// testdata/mockups/160x50.txt line 20): key hints, not the trust band,
// since wide mode's header has a dedicated row for that already.
func boardBottomBorderKeyHintsV2() (left, right string) {
	return "? legend", "r range"
}

// boardBottomBorderTrustBandV2 is the mid/narrow-mode bottom-border text:
// the trust band (via TrustBandLabel), packed in because the header pane
// there has no row of its own for it -- see this file's top-of-file doc
// comment.
func boardBottomBorderTrustBandV2(trust TrustState) (left, right string) {
	return TrustBandLabel(trust), ""
}

// --- legend pane (mid/wide modes) --------------------------------------------

// legendWideMinWidthV2 is the rc.w threshold renderLegendV2 uses to decide
// between the narrow 16-column gutter legend (mid mode; see
// testdata/mockups/100x30.txt) and the wide, 3-column, cost-bearing legend
// (wide mode; see testdata/mockups/160x50.txt). The layout only ever hands
// this file 16 (mid) or >=60 (wide, since wide mode requires w>=145 and the
// legend column is w-85), so any threshold strictly between those two
// values works; 40 is simply a round number in the middle.
const legendWideMinWidthV2 = 40

// renderLegendV2 renders layout.legend: glyph-to-meaning for towers,
// enemies and terrain, with costs where the wide fixture shows them (the
// narrow gutter has no room for costs -- see testdata/mockups/100x30.txt,
// which omits them entirely).
func renderLegendV2(g *eng.Game, rc rect) []string {
	if rc.h <= 0 || rc.w <= 0 {
		return blankRows(rc.h, rc.w)
	}

	var lines []string
	if rc.w < legendWideMinWidthV2 {
		lines = legendNarrowLinesV2()
	} else {
		lines = legendWideLinesV2(g, rc.w)
	}

	out := make([]string, rc.h)
	for i := 0; i < rc.h; i++ {
		if i < len(lines) {
			out[i] = padCells(truncateCells(lines[i], rc.w), rc.w)
		} else {
			out[i] = padCells("", rc.w)
		}
	}
	return out
}

// legendNarrowLinesV2 is the mid-mode 16-column gutter legend: one glyph
// per line, no costs, grouped DEF towers / ATT enemies / TERRAIN exactly as
// testdata/mockups/100x30.txt shows it (16 lines, matching that fixture's
// board height).
func legendNarrowLinesV2() []string {
	return []string{
		"LEGEND",
		"DEF towers",
		fmt.Sprintf(" %c  basic", towerGlyphV2["basic"]),
		fmt.Sprintf(" %c  sniper", towerGlyphV2["sniper"]),
		fmt.Sprintf(" %c  splash", towerGlyphV2["splash"]),
		fmt.Sprintf(" %c  buffer", towerGlyphV2["buffer"]),
		"ATT enemies",
		fmt.Sprintf(" %c  grunt", enemyGlyphV2["basic"]),
		fmt.Sprintf(" %c  fast", enemyGlyphV2["fast"]),
		fmt.Sprintf(" %c  tank", enemyGlyphV2["tank"]),
		fmt.Sprintf(" %c  shielded", enemyGlyphV2["shielded"]),
		fmt.Sprintf(" %c  healer", enemyGlyphV2["healer"]),
		"TERRAIN",
		fmt.Sprintf(" %c  path", rune(pathGlyphV2)),
		fmt.Sprintf(" %c  wall", rune(wallGlyphV2)),
		fmt.Sprintf(" %c  slow zone", rune(slowZoneGlyphV2)),
		fmt.Sprintf(" %c  r: range", rune(rangeGlyphV2)),
	}
}

// towerCostV2 looks up a tower type's cost from the game's active balance
// config, falling back to 0 (rendered as a harmless "0") if the type is
// somehow absent -- this file reads g.Balance rather than hardcoding the
// costs the mockup happens to show (100/100/200/300), so the legend always
// reflects whatever balance config the match is actually running under.
func towerCostV2(g *eng.Game, towerType string) int {
	if g == nil || g.Balance.Towers == nil {
		return 0
	}
	return g.Balance.Towers[towerType].Cost
}

// legendColsV2 are the display columns the wide legend's three groups start
// at: DEFENDER, ATTACKER, TERRAIN. They are named constants rather than
// padding widths baked into eight separate format strings, which is how the
// last column came to start at 40 on two rows and 44 on the other six --
// the "lowercase = enemy" row ran straight into ">>> the engine acted" with
// no gap.
const (
	legendColA = 2
	legendColB = 25
	legendColC = 44
)

// placeColsV2 lays cells out at the given display columns, padding with
// spaces. A cell that would overrun the next column's start is truncated to
// fit, so a long entry can never shunt the columns to its right -- the
// failure mode a chain of %-Ns verbs has, where one over-long value silently
// widens the row.
func placeColsV2(cols []int, cells []string) string {
	var b strings.Builder
	for i, cell := range cells {
		if i >= len(cols) {
			break
		}
		if pad := cols[i] - runeColsV2(b.String()); pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		budget := -1
		if i+1 < len(cols) && i+1 < len(cells) {
			budget = cols[i+1] - cols[i] - 1
		}
		if budget >= 0 {
			cell = truncateCells(cell, budget)
		}
		b.WriteString(cell)
	}
	return b.String()
}

// legendWideLinesV2 is the wide-mode, full-width legend
// row followed by a 3-column DEF/ATT/TERRAIN block with tower costs, per
// testdata/mockups/160x50.txt lines 42-49 (8 lines total, matching that
// fixture's fixed legendH=8).
func legendWideLinesV2(g *eng.Game, w int) []string {
	cols := []int{legendColA, legendColB, legendColC}
	entry := func(glyph rune, name string) string {
		return fmt.Sprintf("%c  %s", glyph, name)
	}
	towerEntry := func(typ, name string) string {
		return fmt.Sprintf("%c  %-14s%d", towerGlyph(typ), name, towerCostV2(g, typ))
	}
	enemyEntry := func(typ string) string {
		return entry(enemyGlyph(typ), enemyDisplayName(typ))
	}

	return []string{
		titledRuleV2(w, "LEGEND", "? toggles"),
		placeColsV2(cols, []string{"DEFENDER  blue", "ATTACKER  orange", "TERRAIN  grey"}),
		placeColsV2(cols, []string{towerEntry("basic", "basic tower"), enemyEntry("basic"), entry(pathGlyphV2, "path")}),
		placeColsV2(cols, []string{towerEntry("sniper", "sniper"), enemyEntry("fast"), entry(wallGlyphV2, "wall")}),
		placeColsV2(cols, []string{towerEntry("splash", "splash"), enemyEntry("tank"), entry(slowZoneGlyphV2, "slow zone")}),
		placeColsV2(cols, []string{towerEntry("buffer", "buffer"), enemyEntry("shielded"), entry(breachGlyphV2, "breach (rev)")}),
		placeColsV2(cols, []string{entry(rangeGlyphV2, "r: tower range"), enemyEntry("healer"), entry(flowGlyphV2, "flow direction")}),
		placeColsV2(cols, []string{"punctuation = tower", "lowercase = enemy", ">>> the engine acted"}),
	}
}
