package main

// Pane renderers for the live match view: status bar, stats table, move
// feed, key bar, and the too-small notice. Each function returns either a
// single fixed-width string (status/key bar, always exactly 1 row) or a
// []string of exactly the requested budget rows (stats/move feed), per
// layout contract invariant #5 -- every pane renders EXACTLY its allotted
// rows, short content padded, long content truncated by the pane itself,
// never left for Bubble Tea to clip.

import (
	"fmt"
	"strings"
)

// renderKeyText builds the one-line key bar. AI on/off and log-pane state
// are the two bits of dynamic state that change which keys are meaningful
// to mention; everything else is a fixed reference.
func renderKeyText(paused, aiEnabled, showLogs bool) string {
	aiStatus := "on"
	if !aiEnabled {
		aiStatus = "off"
	}
	logStatus := "off"
	if showLogs {
		logStatus = "on"
	}
	s := fmt.Sprintf("space pause · +/- speed · a ai:%s · r range · L logs:%s · q quit", aiStatus, logStatus)
	if paused {
		s = "PAUSED · " + s
	}
	return s
}

// fmtTokens abbreviates a token count to a compact, fixed-ish width (e.g.
// "128k") so it doesn't dominate the stats table's column width the way a
// raw 6-7 digit integer would.
func fmtTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fm", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%dk", n/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// tooSmallNotice is the layoutTooSmall fallback: a two-line message stating
// both the CURRENT terminal size and the size actually required, fit to
// whatever size was given (however small) rather than assuming there's room
// for the full message.
func tooSmallNotice(w, h int) string {
	lines := []string{
		fmt.Sprintf("terminal too small: %dx%d", w, h),
		"need at least 60x15",
	}
	budget := 2
	if h < budget {
		budget = h
	}
	if budget < 0 {
		budget = 0
	}
	rows := fitLines(lines, w, budget)
	return strings.Join(rows, "\n")
}

// --- Game-over result card -------------------------------------------------
//
// main.go's old GameOver branch returned a bare two-line string, discarding
// the board, score, wave, and cost at the exact moment a viewer most wants
// them. gameOverView (main.go) replaces it with the frozen final board plus
// this card spliced into the middle of it (board_viewport.go's
// renderBoardWithCard) -- the board stays visible, the card just sits on top
// of it, the same way a modal would.

// renderGameOverKeyText builds the game-over key bar. Only the keys that
// still do something once the match has ended are listed: r (range overlay)
// and L (raw log pane) still act on the frozen board/side panel exactly as
// they did live; space/+/-/a no longer have any effect (the sim has
// stopped), so they're dropped rather than left in as dead hints.
func renderGameOverKeyText(showLogs bool) string {
	logStatus := "off"
	if showLogs {
		logStatus = "on"
	}
	return fmt.Sprintf("r range · L logs:%s · q quit", logStatus)
}
