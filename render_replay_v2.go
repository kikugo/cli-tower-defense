package main

// The replay inspector, ported onto computeLayoutV2's panes.
//
// --- There was no replay mockup, so this is derived, not invented ----------
//
// The design pack (testdata/mockups/) covers the live match screen only. That
// was the stated reason the replay screen kept the old four-mode
// computeLayout through the cutover: porting it meant choosing a six-mode
// arrangement with no fixture to check it against.
//
// The way out is not to invent one. The live view already assigns a MEANING
// to each pane, and every one of those meanings has an exact counterpart in
// the replay, so the port is a mapping rather than a design:
//
//	pane      live meaning                    replay counterpart
//	------    ---------------------------     ---------------------------
//	header    where the match is now          where the PLAYHEAD is now
//	          + the trust band                + whether the reconstruction
//	                                            can be trusted
//	board     the board right now             the board reconstructed to here
//	feed      what has happened               the same, ending at the playhead
//	cards     who the two players are         who acted on THIS event, and why
//	timeline  the match's own tallies         the snapshot's tallies + the
//	                                            event's raw Details
//	legend    what the glyphs mean            unchanged
//	keys      controls                        replay controls
//
// Two of those are worth calling out because they are improvements the port
// gets for free rather than translations:
//
//  1. The feed. The replay already HAS the event stream, so RenderFeedV2 runs
//     on `events[:idx+1]` directly: the same collapsed duplicate runs, the
//     same priority ordering when the pane is short, the same engine-as-third-
//     actor rows the live view gets. The old replay screen showed a
//     json.MarshalIndent dump instead and no feed at all.
//
//  2. The truncation warning. It used to be a banner row stolen out of the
//     board's own budget, with careful per-mode arithmetic to keep the total
//     row count intact (shrink both panes in wide mode, one in stacked). That
//     row is gone: truncation is exactly what the trust band exists to say --
//     "this display cannot be fully trusted, here is why" -- so it goes in the
//     band, in the row the layout already allocates. No budget arithmetic, no
//     mode-specific special case, and it reuses the same three-state vocabulary
//     the live view's assist/provenance disclosure uses.
//
// --- What is NOT ported ----------------------------------------------------
//
// The live view's player CARDS pane shows per-side provenance: authored share,
// saves, engine assists. A ReplaySnapshot carries none of that -- it is
// reconstructed from the event stream, which records what happened, not who
// authored it. So the cards slot shows the current event instead, and says
// nothing about authorship rather than showing an unknown-filled scoreboard.

import (
	"encoding/json"
	"fmt"
	"strings"

	eng "tower-defense/engine"

	"github.com/charmbracelet/lipgloss"
)

// --- input data -----------------------------------------------------------

// ReplayHeaderData is the pure input to the replay header, pre-extracted the
// same way MatchHeaderData is for the live view.
type ReplayHeaderData struct {
	// Index/Total are 1-based position and length, as shown ("5 of 407").
	Index, Total int

	// EventType/Action/PlayerID/Role describe the event at the playhead.
	EventType, Action, PlayerID, Role string
	Tick                              int64

	Wave  int
	Lives int // -1 when no breach has revealed it yet

	Towers, Kills, Breaches int

	Paused bool

	// GameOver/Winner/WinReason are the reconstructed match result, shown
	// once the playhead has passed the game-end event.
	GameOver  bool
	Winner    string
	WinReason string
}

// replayTrustBand renders the replay's disclosure row: whether the board on
// screen is the whole story. It occupies the same header row, at the same
// split column, as the live view's trust band.
//
// It does NOT go through RenderTrustBand/TrustState, and that is the
// interesting decision. TrustState's four states are about engine assists
// and decision provenance -- neither of which a RECONSTRUCTION can speak to,
// because the event stream records what happened, not who authored it.
// Handing it a zero-valued TrustState would print "ENGINE ASSIST UNKNOWN",
// which is true but says nothing about the thing a replay viewer actually
// needs to distrust; handing it anything non-zero would be a claim the
// snapshot cannot support.
//
// So the SHAPE is shared (a titled dash rule split at the same column as the
// vertical rule beneath it) and the vocabulary is the replay's own. What it
// discloses is snap.Truncated: the event stream was capped, so towers and
// enemies from before the discarded window are simply absent and every count
// on screen is a floor rather than a fact.
func replayTrustBand(snap eng.ReplaySnapshot, width, splitCol int) string {
	clause := "─ " + replayTruncationText(snap, true) + " "
	if splitCol <= 0 || splitCol >= width {
		return dashFill(clause, width)
	}
	return dashFill(clause, splitCol) + "┬" + dashFill("─ event stream ", width-splitCol-1)
}

// --- the header -----------------------------------------------------------

// fmtReplayLives renders the reconstructed defender lives, which are -1 until
// a breach event reveals them: the event stream records lives only when they
// change, so before the first breach the true value is genuinely unknown to a
// reconstruction rather than equal to the starting value.
func fmtReplayLives(lives int) string {
	if lives < 0 {
		return "lives not yet revealed"
	}
	return fmt.Sprintf("lives %d", lives)
}

func replayActorLabel(d ReplayHeaderData) string {
	side := sideForRole(d.Role)
	if d.PlayerID == "" && d.Role == "" {
		return "ENGINE/SYSTEM"
	}
	if d.PlayerID == "" {
		return side
	}
	return fmt.Sprintf("%s %s", side, d.PlayerID)
}

func replayResultLine(d ReplayHeaderData) string {
	if !d.GameOver {
		return "match in progress at this point"
	}
	if d.Winner == "" {
		return "match over: no winner recorded"
	}
	return fmt.Sprintf("match over: %s wins (%s)", d.Winner, d.WinReason)
}

// RenderReplayHeaderV2 renders layout.header for the replay screen: exactly
// l.header.h rows of exactly l.header.w columns, in the same
// mirrored-scoreboard shape RenderHeaderV2 uses, with the playhead where the
// live header puts the wave/tick.
func RenderReplayHeaderV2(l layoutV2, d ReplayHeaderData, snap eng.ReplaySnapshot) []string {
	w, h := l.header.w, l.header.h
	if w <= 0 || h <= 0 {
		return blankRows(h, w)
	}

	run := "PLAYING"
	if d.Paused {
		run = "PAUSED"
	}

	var rows []string
	switch l.mode {
	case modeWide:
		rows = []string{
			threeCol(w,
				fmt.Sprintf("  REPLAY   event %d of %d", d.Index, d.Total),
				fmt.Sprintf("%s  %s   tick %d", strings.ToUpper(d.EventType),
					fillBar(d.Index, d.Total, 10), d.Tick),
				fmt.Sprintf("%s   %s", replayActorLabel(d), run),
			),
			threeCol(w,
				fmt.Sprintf("  wave %d   %s", d.Wave, fmtReplayLives(d.Lives)),
				progressBar(d.Index-1, d.Total, 24),
				fmt.Sprintf("towers %d   kills %d   breaches %d", d.Towers, d.Kills, d.Breaches),
			),
			threeCol(w,
				fmt.Sprintf("  action %s", orElse(d.Action, "none")),
				replayResultLine(d),
				"n/b step   [/] +/-10   g/G ends",
			),
			replayTrustBand(snap, w, boardMaxW),
		}

	case modeMid:
		rows = []string{
			threeCol(w,
				fmt.Sprintf("  REPLAY %d/%d", d.Index, d.Total),
				fmt.Sprintf("%s  tick %d", strings.ToUpper(d.EventType), d.Tick),
				fmt.Sprintf("%s  %s", replayActorLabel(d), run),
			),
			threeCol(w,
				fmt.Sprintf("  wave %d  %s", d.Wave, fmtReplayLives(d.Lives)),
				progressBar(d.Index-1, d.Total, 16),
				fmt.Sprintf("towers %d  kills %d  breaches %d", d.Towers, d.Kills, d.Breaches),
			),
			threeCol(w,
				fmt.Sprintf("  action %s", orElse(d.Action, "none")),
				"",
				replayResultLine(d),
			),
		}

	case modeNarrow, modeMinimum, modeCompact:
		rows = []string{
			joinFields(w,
				fmt.Sprintf("  REPLAY %d/%d", d.Index, d.Total),
				strings.ToUpper(d.EventType),
				fmt.Sprintf("t%d", d.Tick),
				replayActorLabel(d),
				run,
			),
			joinFields(w,
				fmt.Sprintf("  wave %d", d.Wave),
				fmtReplayLives(d.Lives),
				fmt.Sprintf("towers %d", d.Towers),
				fmt.Sprintf("kills %d", d.Kills),
				fmt.Sprintf("breaches %d", d.Breaches),
			),
		}

	default:
		return blankRows(h, w)
	}

	return fitLines(rows, w, h)
}

// --- the reconstructed grid ------------------------------------------------
//
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
	// Same texture the live board draws (render_board_v2.go rule 3): a dim
	// '.' with a '>' flow tick every flowTickIntervalV2 tiles. Without the
	// ticks the replay legend advertised a "flow direction" glyph the replay
	// board never drew.
	for _, path := range snap.MapPaths {
		for i, pos := range path {
			if !inBounds(pos) {
				continue
			}
			ch := rune(pathGlyphV2)
			if i > 0 && i%flowTickIntervalV2 == 0 {
				ch = flowGlyphV2
			}
			grid[pos.Y][pos.X] = ch
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

// --- the board ------------------------------------------------------------

// renderReplayBoardV2 draws the reconstructed snapshot into layout.board:
// the same framed, exactly-sized, palette-styled board renderFramedBoardV2
// produces for a live match, from a ReplaySnapshot instead of an *eng.Game.
//
// It pans to centre the current event's highlighted tile, so the pane always
// shows what the event is about -- the one behaviour the replay board has
// that the live board does not (the live one auto-follows the leading enemy).
func renderReplayBoardV2(snap eng.ReplaySnapshot, highlight *eng.Position, rc rect, bottomLeft, bottomRight string) []string {
	if rc.h < 2 || rc.w < 2 {
		return blankRows(rc.h, rc.w)
	}

	grid := buildSnapshotGrid(snap, highlight)
	if grid == nil {
		return replayNoMapPane(rc, bottomLeft, bottomRight)
	}

	mapH := len(grid)
	mapW := 0
	if mapH > 0 {
		mapW = len(grid[0])
	}

	// contentW is the interior between the two border columns and their one
	// column of padding each side -- NOT the map's own width.
	//
	// Those differ here in a way they never do for the live board, and it
	// matters. A live map is always 80 columns inside an 84-column pane, so
	// "pad the row to the viewport width" and "pad the row to the pane's
	// interior" happen to be the same number. A REPLAY map is whatever the
	// recording says: the truncation fixtures use 12x6. Padding to the
	// viewport there produced a box whose borders were 84 columns wide and
	// whose content rows were 16 -- a ragged frame that every exact-width
	// test still passed, because fitPaneRows had already padded each row out
	// to the pane. Only looking at it showed the frame was broken.
	contentW := rc.w - 4
	if contentW < 0 {
		contentW = 0
	}
	contentH := rc.h - 2

	vw := boardViewportWidth(mapW, rc.w)
	vh := contentH
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

	innerW := rc.w - 2
	topLeft := "SPAWN >>"
	topRight := fmt.Sprintf("CORE %s", fmtReplayLives(snap.DefenderLives))

	out := make([]string, 0, rc.h)
	out = append(out, "┌"+titledRuleV2(innerW, topLeft, topRight)+"┐")
	// Every interior row is drawn, map or not, so the frame is the pane. A
	// map shorter than the pane leaves blank rows INSIDE the box rather than
	// closing the box early and leaving blank rows beneath it.
	for y := 0; y < contentH; y++ {
		row := ""
		if y < vh {
			row = styleSnapshotGridRow(grid, y, panX, vw)
		}
		out = append(out, "│ "+padCells(row, contentW)+" │")
	}
	out = append(out, "└"+titledRuleV2(innerW, bottomLeft, bottomRight)+"┘")

	return fitPaneRows(out, rc)
}

// replayNoMapPane is what the board shows before the map_init event, or for
// a replay recorded before map_init existed: an explicit statement that
// there is no board yet, rather than an empty frame that reads like an empty
// board. Those are different facts and the older replays on disk are exactly
// the case where confusing them misleads.
func replayNoMapPane(rc rect, bottomLeft, bottomRight string) []string {
	innerW := rc.w - 2
	contentW := rc.w - 4
	if contentW < 0 {
		contentW = 0
	}
	out := []string{"┌" + titledRuleV2(innerW, "NO BOARD YET", "") + "┐"}
	for i := 0; i < rc.h-2; i++ {
		line := ""
		switch i {
		case 1:
			line = "this replay has not reached its map_init event"
		case 2:
			line = "(replays recorded before map_init have none at all)"
		}
		out = append(out, "│ "+padCells(truncateCells(line, contentW), contentW)+" │")
	}
	out = append(out, "└"+titledRuleV2(innerW, bottomLeft, bottomRight)+"┘")
	return fitPaneRows(out, rc)
}

// fitPaneRows pads/truncates rows to exactly rc.h rows of exactly rc.w
// columns -- the tail every pane renderer in this codebase ends with.
func fitPaneRows(rows []string, rc rect) []string {
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

// --- the event pane (the cards slot) --------------------------------------

// renderReplayEventPaneV2 fills the slot the live view gives the player
// cards: who acted on the event at the playhead, what they did, and the
// reason they gave for it.
//
// The reason is the point of this pane. It is the model's own words about a
// single decision, and reading it beside the board state that decision
// produced is the whole reason to step through a replay rather than read the
// JSON.
func renderReplayEventPaneV2(ev eng.ReplayEvent, idx, total int, rc rect) []string {
	if rc.h <= 0 || rc.w <= 0 {
		return blankRows(rc.h, rc.w)
	}

	rows := []string{
		titledRuleV2(rc.w, "THIS EVENT", fmt.Sprintf("%d of %d", idx, total)),
		cardRow("type", string(ev.Type)),
		cardRow("tick", fmt.Sprintf("%d", ev.Tick)),
		cardRow("actor", orElse(strings.TrimSpace(sideForRole(ev.Role)+" "+ev.PlayerID), "engine")),
		cardRow("action", orElse(ev.Action, "none")),
		cardRow("target", orElse(posStr(ev.Position), "no position")),
	}

	// Whatever rows are left go to the reason, wrapped as a quote the same
	// way the player cards wrap a model's reasoning.
	quoteBudget := rc.h - len(rows)
	if quoteBudget > 0 {
		rows = append(rows, "")
		quoteBudget--
	}
	if quoteBudget > 0 {
		rows = append(rows, quoteRowsV2(ev.Reason, rc.w, quoteBudget)...)
	}

	for i, r := range rows {
		rows[i] = padCells(truncateCells(r, rc.w), rc.w)
	}
	return fitPaneRows(rows, rc)
}

// --- the detail pane (the timeline slot) ----------------------------------

// renderReplayDetailPaneV2 fills the slot the live view gives the match
// timeline: the snapshot's own tallies, then the event's raw Details as
// indented JSON.
//
// The JSON is capped by fitLinesWithMoreIndicator rather than by silent
// truncation, and that is not cosmetic: a map_init event's Details is the
// full paths array, which renders at ~396 rows. The pre-rewrite viewer
// handed that straight to the terminal. Here the pane says how many rows it
// is hiding, so "the content ends here" and "the pane ends here" are
// distinguishable.
func renderReplayDetailPaneV2(snap eng.ReplaySnapshot, ev eng.ReplayEvent, rc rect) []string {
	if rc.h <= 0 || rc.w <= 0 {
		return blankRows(rc.h, rc.w)
	}

	head := []string{titledRuleV2(rc.w, "RECONSTRUCTED STATE", fmt.Sprintf("tick %d", snap.Tick))}
	for _, line := range snap.SummaryLines() {
		head = append(head, "  "+line)
	}
	head = append(head, "")

	if len(head) >= rc.h {
		return fitPaneRows(head, rc)
	}

	details := "{}"
	if len(ev.Details) > 0 {
		if data, err := json.MarshalIndent(ev.Details, "", "  "); err == nil {
			details = string(data)
		}
	}
	body := append([]string{titledRuleV2(rc.w, "EVENT DETAILS", "")},
		strings.Split(details, "\n")...)

	rows := append(head, fitLinesWithMoreIndicator(body, rc.w, rc.h-len(head))...)
	return fitPaneRows(rows, rc)
}

// --- composition ----------------------------------------------------------

// replayKeyTextV2 is the replay screen's key bar. "+/-10", not "±10": the
// ASCII fold is one display column in for one out, and '±' has no
// one-character ASCII equivalent (see render_theme_v2.go).
func replayKeyTextV2(paused bool) string {
	keys := "space pause/resume   n/b step   [/] +/-10   g/G start/end   e game end   ? legend   q quit"
	if paused {
		return "PAUSED   " + keys
	}
	return keys
}

// ReplayViewV2 renders one frame of the replay inspector: exactly m.height
// rows of exactly m.width columns, through the same six-mode layout and the
// same blit composition the live view uses.
func (m model) ReplayViewV2() string {
	w, h := m.width, m.height
	if w == 0 || h == 0 {
		w, h = 80, 24
	}
	l := computeLayoutV2(w, h)
	if l.mode == modeNotice {
		return tooSmallNotice(w, h)
	}

	total := len(m.replay)
	if total == 0 {
		return strings.Join(fitLines(
			[]string{"Replay mode: no events loaded", "Press q to quit."}, w, h), "\n")
	}

	idx := clampReplayIdx(m.replayIdx, total)
	ev := m.replay[idx]
	snap := eng.ReconstructSnapshot(m.replay, idx+1)

	data := ReplayHeaderData{
		Index: idx + 1, Total: total,
		EventType: string(ev.Type), Action: ev.Action,
		PlayerID: ev.PlayerID, Role: ev.Role,
		Tick:   snap.Tick,
		Wave:   snap.Wave,
		Lives:  snap.DefenderLives,
		Towers: len(snap.Towers), Kills: snap.Kills, Breaches: snap.Breaches,
		Paused:   m.paused,
		GameOver: snap.GameOver, Winner: snap.Winner,
		WinReason: humanizeFeedText(snap.WinReason),
	}

	frame := newFrameV2(l.w, l.h)
	blitV2(frame, l.header, RenderReplayHeaderV2(l, data, snap), l.w)

	// The board's bottom border carries the trust line in every mode whose
	// header has no row for it -- the same rule render_board_v2.go follows
	// for the live board.
	bl, br := "? legend", "n/b step"
	if l.mode != modeWide {
		bl, br = replayTruncationText(snap, false), ""
	}

	switch l.mode {
	case modeMinimum, modeCompact:
		blitV2(frame, l.label, []string{padCells(truncateCells(
			replayLabelRowV2(data, snap), l.label.w), l.label.w)}, l.w)
		blitV2(frame, l.mapPane, renderReplayMapPaneV2(snap, ev.Position, rect{w: l.mapPane.w, h: l.mapPane.h}), l.w)
	default:
		blitV2(frame, l.board,
			renderReplayBoardV2(snap, ev.Position, rect{w: l.board.w, h: l.board.h}, bl, br), l.w)
	}

	if l.legend.area() > 0 {
		blitV2(frame, l.legend, renderReplayLegendV2(rect{w: l.legend.w, h: l.legend.h}), l.w)
	}
	if l.rule.area() > 0 {
		blitV2(frame, l.rule, verticalRuleV2(l.rule.h), l.w)
	}
	if l.cards.area() > 0 {
		blitV2(frame, l.cards, renderReplayEventPaneV2(ev, idx+1, total, rect{w: l.cards.w, h: l.cards.h}), l.w)
	}
	if l.timeline.area() > 0 {
		blitV2(frame, l.timeline,
			renderReplayDetailPaneV2(snap, ev, rect{w: l.timeline.w, h: l.timeline.h}), l.w)
	}

	// The feed is the replay's own event stream up to the playhead, through
	// the live view's feed renderer -- so a replay gets collapsed duplicate
	// runs, priority ordering and engine rows exactly as a live match does.
	blitV2(frame, l.feed, m.replayFeedPaneV2(l, idx), l.w)
	blitV2(frame, l.keys, []string{replayKeyTextV2(m.paused)}, l.w)

	if m.asciiMode {
		frame = asciiFoldRows(frame)
	}
	return strings.Join(frame, "\n")
}

// replayFeedPaneV2 renders the events up to and including the playhead. In
// the non-wide modes there is no separate detail pane, so the feed shares its
// column with nothing and the event details go unshown -- the event pane and
// the detail pane are wide-mode-only, exactly like the cards and timeline
// they replace.
func (m model) replayFeedPaneV2(l layoutV2, idx int) []string {
	return RenderFeedV2(m.replay[:idx+1], l.feed.w, l.feed.h)
}

// replayTruncationText is the disclosure's wording, in one place, in two
// lengths. Both say the CONSEQUENCE -- the board is missing earlier towers
// and enemies -- not just the bare fact that a truncation happened.
//
// That is the part worth keeping across the layout port. A row reading
// "TRUNCATED: 858 events discarded" tells a reader something occurred; it
// does not tell them the board in front of them is wrong, which is the only
// thing they need to know. The short form exists because the board border
// has less room than a header row, and it keeps the consequence and drops
// the count, in that order of priority.
func replayTruncationText(snap eng.ReplaySnapshot, long bool) string {
	if !snap.Truncated {
		if long {
			return "RECONSTRUCTED FROM EVENTS ─ counts are exact"
		}
		return "RECONSTRUCTED FROM EVENTS"
	}

	if long {
		if snap.TruncatedEvents > 0 {
			return fmt.Sprintf("TRUNCATED ─ %d events discarded, board is missing earlier towers and enemies, every count is a floor",
				snap.TruncatedEvents)
		}
		return "TRUNCATED ─ events discarded, board is missing earlier towers and enemies, every count is a floor"
	}

	if snap.TruncatedEvents > 0 {
		return fmt.Sprintf("TRUNCATED ─ board is missing earlier towers and enemies (%d events discarded)",
			snap.TruncatedEvents)
	}
	return "TRUNCATED ─ board is missing earlier towers and enemies"
}

// replayLabelRowV2 is the minimum/compact modes' one-row map title, matching
// the live view's renderLabelRowV2 shape.
func replayLabelRowV2(d ReplayHeaderData, snap eng.ReplaySnapshot) string {
	segments := []string{
		fmt.Sprintf("E%d/%d t%d", d.Index, d.Total, d.Tick),
		replayTruncationText(snap, false),
		fmt.Sprintf("SPAWN >> CORE %s", fmtReplayLives(d.Lives)),
	}
	return "─ " + strings.Join(segments, " ─ ") + " ─"
}

// renderReplayMapPaneV2 is the borderless map for minimum/compact modes.
func renderReplayMapPaneV2(snap eng.ReplaySnapshot, highlight *eng.Position, rc rect) []string {
	if rc.h <= 0 || rc.w <= 0 {
		return blankRows(rc.h, rc.w)
	}
	grid := buildSnapshotGrid(snap, highlight)
	if grid == nil {
		return fitPaneRows([]string{"", "  this replay has not reached its map_init event"}, rc)
	}

	mapH := len(grid)
	mapW := 0
	if mapH > 0 {
		mapW = len(grid[0])
	}

	vw := mapViewportWidth(mapW, rc.w)
	vh := rc.h
	if vh > mapH {
		vh = mapH
	}
	panX := 0
	if highlight != nil {
		panX = clampPan(highlight.X-vw/2, mapW, vw)
	}

	rows := make([]string, vh)
	for y := 0; y < vh; y++ {
		rows[y] = styleSnapshotGridRow(grid, y, panX, vw)
	}
	return fitPaneRows(rows, rc)
}

// renderReplayLegendV2 is the live legend minus the tower costs, which a
// snapshot does not carry (the balance config is not in the event stream),
// plus the replay's own highlight marker.
func renderReplayLegendV2(rc rect) []string {
	if rc.h <= 0 || rc.w <= 0 {
		return blankRows(rc.h, rc.w)
	}

	var lines []string
	if rc.w < legendWideMinWidthV2 {
		lines = append(legendNarrowLinesV2(),
			fmt.Sprintf(" %c  this event", highlightGlyphV2))
	} else {
		cols := []int{legendColA, legendColB, legendColC}
		entry := func(g rune, name string) string { return fmt.Sprintf("%c  %s", g, name) }
		lines = []string{
			titledRuleV2(rc.w, "LEGEND", "reconstructed board"),
			placeColsV2(cols, []string{"DEFENDER  blue", "ATTACKER  orange", "TERRAIN  grey"}),
			placeColsV2(cols, []string{entry(towerGlyph("basic"), "basic tower"), entry(enemyGlyph("basic"), "grunt"), entry(pathGlyphV2, "path")}),
			placeColsV2(cols, []string{entry(towerGlyph("sniper"), "sniper"), entry(enemyGlyph("fast"), "fast"), entry(wallGlyphV2, "wall")}),
			placeColsV2(cols, []string{entry(towerGlyph("splash"), "splash"), entry(enemyGlyph("tank"), "tank"), entry(slowZoneGlyphV2, "slow zone")}),
			placeColsV2(cols, []string{entry(towerGlyph("buffer"), "buffer"), entry(enemyGlyph("shielded"), "shielded"), entry(breachGlyphV2, "breach (rev)")}),
			placeColsV2(cols, []string{entry(highlightGlyphV2, "this event"), entry(enemyGlyph("healer"), "healer"), entry(flowGlyphV2, "flow direction")}),
			placeColsV2(cols, []string{"punctuation = tower", "lowercase = enemy", "costs not in the stream"}),
		}
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
