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
	"time"

	eng "tower-defense/engine"
)

// modelNameCells is the fixed budget every model name is abbreviated to in
// the stats table (via shortName), so the table's row count -- and every
// column's width -- never depends on how long a model's name is. This is
// the direct fix for the 43->58 row swing the task brief measures between
// "gpt-4o-mini" and "qwen/qwen3-next-80b-a3b-instruct": both now cost
// exactly modelNameCells columns.
const modelNameCells = 13

// renderStatusText builds the one-line status summary: wave progress, tick,
// whose turn it is, speed, and pause state. Rendered without padding here;
// callers fit it to the pane width with padCells.
func renderStatusText(g *eng.Game, tickDur time.Duration, paused bool) string {
	turn := "DEF"
	if g.CurrentTurn == g.Attacker {
		turn = "ATT"
	}
	speed := 100.0 / float64(tickDur/time.Millisecond)
	play := "▶"
	if paused {
		play = "⏸"
	}
	return fmt.Sprintf("%s · tick %d · turn ▸%s · %.1fx · %s",
		waveProgressBar(g.Wave, g.MaxWaves, 12), g.TickCount, turn, speed, play)
}

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

// renderStats renders the defender/attacker stats table into exactly rc.h
// rows. In the normal 5-row form it's a header + name row + three data
// rows; the 3-row "cramped" form (compact mode) collapses header+names into
// one row. Either way, the row COUNT is fixed by which literal slice is
// built (never by wrapping), and every name is shortName-clamped to
// modelNameCells columns, so it is identical for every matchup --
// TestStatsRowCountIndependentOfModelName checks this directly for all five
// real model names this project configures.
func renderStats(g *eng.Game, rc rect) []string {
	defID, attID := g.Defender, g.Attacker
	defName := shortName(g.ModelNames[defID], modelNameCells)
	attName := shortName(g.ModelNames[attID], modelNameCells)

	var lines []string
	if rc.h <= 3 {
		lines = []string{
			fmt.Sprintf("DEF %-*s ATT %-*s", modelNameCells, defName, modelNameCells, attName),
			fmt.Sprintf("lives %d  res %d+%d  |  res %d+%d",
				g.Lives[defID], g.Resources[defID], g.Income[defID], g.Resources[attID], g.Income[attID]),
			fmt.Sprintf("towers %d  enemies %d  wave %d/%d", len(g.Towers), len(g.Enemies), g.Wave, g.MaxWaves),
		}
	} else {
		col := modelNameCells + 5
		lines = []string{
			fmt.Sprintf("%-13s%-*s%-*s", "", col, "DEFENDER", col, "ATTACKER"),
			fmt.Sprintf("%-13s%-*s%-*s", "", col, defName, col, attName),
			fmt.Sprintf("%-13s%-*s%-*s", "lives/res",
				col, fmt.Sprintf("%d ♥ %d ⛁+%d", g.Lives[defID], g.Resources[defID], g.Income[defID]),
				col, fmt.Sprintf("%d ⛁+%d", g.Resources[attID], g.Income[attID])),
			fmt.Sprintf("%-13s%-*s%-*s", "board",
				col, fmt.Sprintf("%d towers", len(g.Towers)),
				col, fmt.Sprintf("%d enemies · wave %d/%d", len(g.Enemies), g.Wave, g.MaxWaves)),
			fmt.Sprintf("%-13s%-*s%-*s", "calls/cost",
				col, fmt.Sprintf("%d · %s · %s", g.ProviderCalls[defID], fmtTokens(g.ProviderTokenUsage[defID]), fmtCostMicros(g.ProviderCostMicros[defID])),
				col, fmt.Sprintf("%d · %s · %s", g.ProviderCalls[attID], fmtTokens(g.ProviderTokenUsage[attID]), fmtCostMicros(g.ProviderCostMicros[attID]))),
		}
	}
	return fitLines(lines, rc.w, rc.h)
}

// moveFeedEventTypes are the ReplayEvent types that represent a player
// action (or the outcome of one) worth showing in the move feed. Excludes
// eng.ReplayTick (periodic state snapshots, not a move) and
// eng.ReplayMapInit (a one-time board-layout record, not a move -- and the
// event whose full-paths-array Details field is finding 1.4's 396-row
// blowup, which the feed must never render raw).
func isMoveFeedEvent(t eng.ReplayEventType) bool {
	switch t {
	case eng.ReplayDecision, eng.ReplayPlacement, eng.ReplaySpawn, eng.ReplayWave,
		eng.ReplayRejected, eng.ReplayProviderErr, eng.ReplayGameEnd, eng.ReplayBreach:
		return true
	}
	return false
}

// buildMoveFeed filters a game's full ReplayEvents history down to the
// subset the move feed displays, preserving order (oldest first).
func buildMoveFeed(events []eng.ReplayEvent) []eng.ReplayEvent {
	feed := make([]eng.ReplayEvent, 0, len(events))
	for _, ev := range events {
		if isMoveFeedEvent(ev.Type) {
			feed = append(feed, ev)
		}
	}
	return feed
}

// formatMoveRow renders one replay event as the fixed-column
// "turn | side | action | target" the task brief specifies. It is always a
// single logical row -- callers truncate (never wrap) it to the pane width.
func formatMoveRow(ev eng.ReplayEvent) string {
	side := "--"
	switch ev.Role {
	case "defender":
		side = "DEF"
	case "attacker":
		side = "ATT"
	}

	action := ev.Action
	if action == "" {
		action = string(ev.Type)
	}
	target := ""
	if ev.Position != nil {
		target = fmt.Sprintf("%d,%d", ev.Position.Y, ev.Position.X)
	}

	switch ev.Type {
	case eng.ReplayRejected:
		action = "REJECT " + action
		if target == "" {
			target = truncateCells(ev.Reason, 24)
		}
	case eng.ReplayProviderErr:
		action = "ERROR"
		target = truncateCells(ev.Reason, 24)
	case eng.ReplayGameEnd:
		side = "***"
		action = "GAME END"
		target = truncateCells(ev.Reason, 24)
	}

	return fmt.Sprintf("%5d │ %-3s │ %-11s │ %s", ev.Tick, side, truncateCells(action, 11), target)
}

// renderMoveFeed renders the newest `budget` entries of feed, newest at the
// bottom, each truncated (not wrapped) to width and padded so every
// returned row is exactly width columns -- so pane height is exactly
// min(budget, len(feed)) by construction and never depends on how the
// engine happened to format a log line (the 12-row "=== Game State ==="
// problem this replaces).
func renderMoveFeed(feed []eng.ReplayEvent, width, budget int) []string {
	if budget < 0 {
		budget = 0
	}
	tail := feed
	if len(feed) > budget {
		tail = feed[len(feed)-budget:]
	}

	rows := make([]string, 0, budget)
	for _, ev := range tail {
		rows = append(rows, padCells(formatMoveRow(ev), width))
	}
	for len(rows) < budget {
		rows = append([]string{padCells("", width)}, rows...)
	}
	if len(rows) > budget {
		rows = rows[len(rows)-budget:]
	}
	return rows
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
