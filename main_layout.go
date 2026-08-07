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

// boardMaxW is the board pane's own content ceiling: the simulation map is
// fixed at 80 columns (engine/core.go:636-638) and uiBorder adds a 2-column
// border plus 1-column padding on each side, so a fully-visible board is
// never wider than 80+4 = 84 columns, no matter how wide the terminal is.
const boardMaxW = 84

// boardMaxH is the board pane's row ceiling: 14 fixed map rows plus the
// 2-row NormalBorder top/bottom.
const boardMaxH = 16
