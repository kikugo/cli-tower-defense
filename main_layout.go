package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
	"github.com/rivo/uniseg"
)

// renderedRows reports how many terminal rows s occupies when rendered inside
// a pane of the given width. It accounts for soft wrapping (long lines wrap
// at word boundaries, with a hard break inside a single word that alone
// exceeds width) and for embedded newlines, by deferring to the exact
// primitive the rest of the render path uses: lipgloss.NewStyle().Width().
// This is intentionally identical to the definition in the task contract
// (lipgloss.Height(lipgloss.NewStyle().Width(width).Render(s))) rather than a
// hand-rolled re-implementation, so it can never drift from what the sidebar
// actually renders.
func renderedRows(s string, width int) int {
	return lipgloss.Height(lipgloss.NewStyle().Width(width).Render(s))
}

// fitLines returns EXACTLY budget rows: every input line is wrapped to width
// (long lines soft-wrap, embedded newlines split onto their own rows) using
// the same lipgloss primitive renderedRows is defined in terms of, so a
// returned row always costs exactly one rendered row at that width -- rows
// beyond budget are dropped, and if there isn't enough content the result is
// padded out with empty rows. Because wrapping goes through
// lipgloss.NewStyle().Width().Render() (backed by charmbracelet/x/ansi.Wrap,
// which treats escape sequences as atomic), an ANSI escape sequence already
// present in an input line is never split across a wrap point, and cutting
// the result down to budget only ever drops whole rows, never mid-sequence
// bytes.
func fitLines(lines []string, width, budget int) []string {
	if budget < 0 {
		budget = 0
	}

	rows := make([]string, 0, budget)
	style := lipgloss.NewStyle().Width(width)
	for _, line := range lines {
		wrapped := style.Render(line)
		rows = append(rows, strings.Split(wrapped, "\n")...)
		if len(rows) >= budget {
			break
		}
	}

	if len(rows) > budget {
		rows = rows[:budget]
	}
	for len(rows) < budget {
		rows = append(rows, "")
	}
	return rows
}

// wrapAllLines wraps every input line to width via the exact same lipgloss
// primitive fitLines is defined in terms of, but WITHOUT capping or padding
// to a budget -- the complete wrapped row list, however long. fitLines is
// equivalent to wrapAllLines followed by a cap/pad to budget; it is kept as
// its own loop (rather than calling this) so it can still break out early
// once budget is reached, which matters for large inputs (e.g. a 250-entry
// raw log tail) where only the first few rows are ever wanted. This function
// exists for fitLinesWithMoreIndicator below, which needs the TRUE row count
// (to report how many rows a "+N more lines" indicator hides) and so cannot
// stop early.
func wrapAllLines(lines []string, width int) []string {
	style := lipgloss.NewStyle().Width(width)
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped := style.Render(line)
		rows = append(rows, strings.Split(wrapped, "\n")...)
	}
	return rows
}

// fitLinesWithMoreIndicator behaves exactly like fitLines (always returns
// EXACTLY budget rows, via the same wrapping primitive) except when lines
// would otherwise be silently cut to fit budget: the last returned row is
// replaced with a "+N more lines" indicator reporting how many wrapped rows
// were hidden, so a viewer can tell content was cut rather than assuming the
// pane's content ends where the pane does. This is the fix for the replay
// Event Details JSON pane (main.go's replayView), which can be arbitrarily
// long -- the map_init event's full paths array alone renders ~396 rows
// before any budget is applied (main_view_test.go's replay_event_zero case).
func fitLinesWithMoreIndicator(lines []string, width, budget int) []string {
	if budget < 0 {
		budget = 0
	}
	all := wrapAllLines(lines, width)
	if len(all) <= budget {
		return fitLines(lines, width, budget)
	}
	if budget == 0 {
		return nil
	}
	kept := append([]string{}, all[:budget-1]...)
	hidden := len(all) - len(kept)
	indicator := fmt.Sprintf("… +%d more lines", hidden)
	kept = append(kept, lipgloss.NewStyle().Width(width).Render(truncateCells(indicator, width)))
	return kept
}

// ellipsis is appended by shortName when truncation is needed. It is a
// single display cell wide, so budgeting for it is a flat -1.
const ellipsis = "…"

// shortName abbreviates a model name to at most cells display columns,
// appending a single-cell ellipsis when it doesn't fit as-is. Like
// truncateCells, it walks grapheme clusters (via uniseg) rather than bytes or
// runes, so it is safe for CJK and emoji input and never emits invalid
// UTF-8.
func shortName(s string, cells int) string {
	if cells <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= cells {
		return s
	}

	ellW := lipgloss.Width(ellipsis)
	if cells <= ellW {
		return truncateCells(ellipsis, cells)
	}
	return truncateCells(s, cells-ellW) + ellipsis
}

// truncateCells truncates s to at most cells display columns. It replaces
// the old wrapText helper, which sliced text[:width-3] -- a byte-length
// slice that both confused bytes with display columns and could cut a
// multi-byte rune in half, emitting invalid UTF-8 for any free-form LLM
// output (taunts, reasoning) containing CJK text or emoji.
//
// truncateCells instead walks grapheme clusters one at a time via
// uniseg.FirstGraphemeClusterInString, accumulating their display width and
// stopping before the budget would be exceeded. Every byte written to the
// result comes from a complete grapheme cluster, so the output is always
// valid UTF-8, and non-positive cells (including the negative values that
// would have panicked the old text[:width-3] slice) simply yield "".
func truncateCells(s string, cells int) string {
	if cells <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= cells {
		return s
	}

	var b strings.Builder
	width := 0
	state := -1
	rest := s
	for len(rest) > 0 {
		cluster, remainder, w, newState := uniseg.FirstGraphemeClusterInString(rest, state)
		if width+w > cells {
			break
		}
		b.WriteString(cluster)
		width += w
		rest = remainder
		state = newState
	}
	return b.String()
}

// padCells forces s to occupy EXACTLY w display columns: padded with plain
// trailing spaces if short, hard-truncated if long. Unlike truncateCells
// (grapheme-safe but ANSI-oblivious), padCells is safe to call on strings
// that already carry embedded lipgloss/ANSI styling (the board rows, and
// anything composed via hjoin) -- shrinking goes through
// github.com/muesli/reflow/truncate.String, which treats a complete escape
// sequence as atomic and never cuts inside one; padding only ever appends
// plain spaces after existing content, which can't corrupt a sequence
// either. This is a different function from muesli/reflow's wordwrap (which
// the task brief warns against): truncate.String performs a hard,
// single-line cut at a column, never a word-wrap re-flow, so it can't
// introduce the row-accounting drift a wordwrap/lipgloss mismatch would.
func padCells(s string, w int) string {
	if w <= 0 {
		return ""
	}
	cur := lipgloss.Width(s)
	if cur > w {
		return truncate.String(s, uint(w))
	}
	if cur < w {
		return s + strings.Repeat(" ", w-cur)
	}
	return s
}

// hjoin merges two already-rendered row slices side by side, padding the
// shorter slice with blank rows (at its own width) so the result has
// len == max(len(left), len(right)) -- mirroring lipgloss.JoinHorizontal's
// auto-pad behavior for mismatched block heights, but operating on already
// exact-width []string rows instead of a single multi-line string, and using
// the ANSI-safe padCells rather than lipgloss's internal line merge, so it
// is safe when either side carries embedded styling (the board does).
func hjoin(left []string, leftW int, right []string, rightW int) []string {
	n := len(left)
	if len(right) > n {
		n = len(right)
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		l := ""
		if i < len(left) {
			l = left[i]
		}
		r := ""
		if i < len(right) {
			r = right[i]
		}
		out[i] = padCells(l, leftW) + padCells(r, rightW)
	}
	return out
}

// vstack concatenates row slices vertically. Each input slice is assumed to
// already be exactly its own pane's row count (built via fitLines,
// renderBoard, renderStats, etc.), so this is a plain append -- rows drawn
// on separate lines never need matching widths the way hjoin's side-by-side
// merge does.
func vstack(groups ...[]string) []string {
	total := 0
	for _, g := range groups {
		total += len(g)
	}
	out := make([]string, 0, total)
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// blankRows returns n rows, each padded to exactly w columns of spaces. Used
// as the degenerate-size fallback when a pane's rect has zero content area
// (for example a board rect too narrow/short to show any map at all).
func blankRows(n, w int) []string {
	if n < 0 {
		n = 0
	}
	out := make([]string, n)
	fill := ""
	if w > 0 {
		fill = strings.Repeat(" ", w)
	}
	for i := range out {
		out[i] = fill
	}
	return out
}

// rect is a pane's allotted size in terminal cells: exactly h rows, each
// exactly w columns wide once rendered (invariant #5 in the layout
// contract -- short content padded, long content truncated BY THE PANE).
type rect struct {
	w, h int
}

// layoutMode names the arrangement chosen for a given terminal size. The
// numeric ordering is deliberately least-to-most-spacious (layoutTooSmall <
// layoutCompact < layoutStacked < layoutWide) so "mode transitions are
// monotonic in w" (layout contract invariant #3 / the T2.1 property test)
// reads as a plain non-decreasing comparison on the mode value as w grows.
type layoutMode int

const (
	layoutTooSmall layoutMode = iota
	layoutCompact
	layoutStacked
	layoutWide
)

// layout is the pure output of computeLayout: the arrangement plus every
// pane's exact rect. Nothing downstream re-derives a size from a constant --
// every width and height here was computed from w/h.
type layout struct {
	mode   layoutMode
	w, h   int // the terminal size actually used (post zero-size normalization)
	status rect
	board  rect
	stats  rect
	moves  rect
	keybar rect
}

// boardMaxW is the board pane's own content ceiling: the simulation map is
// fixed at 80 columns (engine/core.go:636-638) and uiBorder adds a 2-column
// border plus 1-column padding on each side, so a fully-visible board is
// never wider than 80+4 = 84 columns, no matter how wide the terminal is.
const boardMaxW = 84

// boardMaxH is the board pane's row ceiling: 14 fixed map rows plus the
// 2-row NormalBorder top/bottom.
const boardMaxH = 16

// computeLayout is the pure function that turns a terminal size into an
// arrangement and a set of pane rects. It never reads global/package state
// and every returned width/height is DERIVED from w/h -- there are no
// independent width constants that could drift out of agreement with each
// other (the bug the task brief calls out: three unrelated constants -- 35
// sidebar, 80 map, 120 threshold -- used to have to agree by coincidence).
//
// Per the task contract:
//  1. w == 0 || h == 0 (the pre-WindowSizeMsg state) is normalized to 80x24
//     rather than being treated as an unbounded/"wide" terminal.
//  2. w < 60 or h < 15 (after normalization) reports layoutTooSmall; callers
//     must render only the two-line notice in that case, not the board.
//  3. Otherwise the mode is chosen purely from w: compact (60-83), stacked
//     (84-115), wide (116+) -- monotonically non-decreasing as w grows for
//     any fixed h.
//  4. Row budget is allocated top-down from fixed minima -- status(1),
//     board(<=16), stats(5, or 3 in compact), keybar(1) -- and the move feed
//     absorbs whatever remains. In wide mode the move feed is the full
//     right-hand column (h-2 rows): the left column (board+stats) is capped
//     at the same remaining budget but is typically shorter, and the
//     shortfall is blank-padded by hjoin when the two columns are joined.
//  5. Pane widths: board/stats are min(84, w) (never wider than the fixed
//     80-column map can use); moves is w-board.w in wide mode (so
//     board.w+moves.w == w exactly) or the full w in stacked/compact mode
//     (it sits below the board there, not beside it).
func computeLayout(w, h int) layout {
	if w == 0 || h == 0 {
		w, h = 80, 24
	}
	if w < 60 || h < 15 {
		return layout{mode: layoutTooSmall, w: w, h: h}
	}

	var mode layoutMode
	switch {
	case w >= 116:
		mode = layoutWide
	case w >= 84:
		mode = layoutStacked
	default:
		mode = layoutCompact
	}

	boardW := w
	if boardW > boardMaxW {
		boardW = boardMaxW
	}

	remaining := h - 2 // status + keybar
	boardH := boardMaxH
	if boardH > remaining {
		boardH = remaining
	}
	if boardH < 0 {
		boardH = 0
	}
	afterBoard := remaining - boardH

	statsTarget := 5
	if mode == layoutCompact {
		statsTarget = 3
	}
	statsH := statsTarget
	if statsH > afterBoard {
		statsH = afterBoard
	}
	if statsH < 0 {
		statsH = 0
	}
	afterStats := afterBoard - statsH

	var movesW, movesH int
	if mode == layoutWide {
		movesW = w - boardW
		movesH = remaining // full column height; the only pane whose height tracks h/slack directly
	} else {
		movesW = w
		movesH = afterStats
		if movesH < 0 {
			movesH = 0
		}
	}

	return layout{
		mode:   mode,
		w:      w,
		h:      h,
		status: rect{w: w, h: 1},
		board:  rect{w: boardW, h: boardH},
		stats:  rect{w: boardW, h: statsH},
		moves:  rect{w: movesW, h: movesH},
		keybar: rect{w: w, h: 1},
	}
}
