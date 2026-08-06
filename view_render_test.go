package main

// Tests for what remains of view_render.go after the redesign cutover: the
// key bar. The stats pane, the status line and the move feed that this file
// also used to cover were deleted with the old view -- their replacements
// (RenderHeaderV2, RenderCardsV2, RenderFeedV2) carry their own tests, and
// keeping shells of the old ones pointed at deleted functions would have
// been the only reason to keep the functions.

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestKeyBarExactlyOneRowAtEveryWidth: the key bar is the bottom row of
// every non-notice mode and the layout allocates it exactly one row, so a
// wrap here silently pushes a row of the pane above it off the frame.
func TestKeyBarExactlyOneRowAtEveryWidth(t *testing.T) {
	for w := 40; w <= 250; w += 7 {
		for _, key := range []string{
			renderKeyText(false, true, false),
			renderKeyText(true, false, true),
			renderGameOverKeyText(false),
			renderGameOverKeyText(true),
		} {
			row := padCells(key, w)
			if h := lipgloss.Height(row); h != 1 {
				t.Fatalf("w=%d: key bar height %d, want 1", w, h)
			}
			if got := lipgloss.Width(row); got != w {
				t.Fatalf("w=%d: key bar width %d, want exactly %d", w, got, w)
			}
		}
	}
}
