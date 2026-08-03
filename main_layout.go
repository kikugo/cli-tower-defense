package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
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
