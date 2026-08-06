package main

// The Phase 2 match-timeline pane: layout.timeline's renderer (wide mode
// only -- computeLayoutV2 leaves the rect zero-valued everywhere else). See
// testdata/mockups/160x50.txt lines 37-49: a per-wave table over a block of
// whole-match summary rows.
//
// Nothing here is wired into main.go's View() yet; the cutover is Phase 4.
//
// --- Why the wave table rolls up rather than scrolls -----------------------
//
// timeline is main_layout_v2.go's slack-absorbing pane in wide mode's left
// column: its height is whatever board (16) and cards (16) leave over, so
// it can be 13 rows at 160x50 and single digits on a shorter terminal,
// while a 30-wave ruleset wants 30 table rows. Dropping the oldest waves to
// fit would be the obvious fix and it is the wrong one -- the first waves
// are where a defender's opening is visible, and a table that silently
// starts at wave 18 reads as though the match started there.
//
// So the oldest waves are BANDED into a single "1-10" row whose counters
// are the band's sums and whose lives column spans the band's whole life
// swing. Nothing is dropped; the resolution just drops. That is also what
// the fixture shows (line 39: a "1-10" row above individual rows for waves
// 11 and 12), so the design already anticipated this.
//
// --- Why this file takes []eng.WaveSummary directly ------------------------
//
// render_header_v2.go and render_cards_v2.go take flat pre-extracted
// structs specifically so they cannot collapse an unmeasured provenance
// figure into a confident zero. WaveSummary has no such hazard: it is
// already a flat value type of plain counters with no (value, ok) accessor
// behind it, and engine/telemetry.go's buildWaveSummaries anchors its
// LivesStart/LivesEnd walk forward from a fixed StartingLives so the rows
// arrive internally consistent. Re-copying it field-for-field into a
// look-alike struct here would add a place for the two to drift without
// adding any safety. The provenance figures on this pane -- which do have
// the unknown state -- still come through AuthoredShare/TrustState, reused
// from render_header_v2.go.

import (
	"fmt"

	eng "tower-defense/engine"
)

// --- input data -----------------------------------------------------------

// TimelineData is the complete, pure input to RenderTimelineV2.
type TimelineData struct {
	// Waves is engine.Game.buildWaveSummaries' output (exposed to the
	// caller via whatever accessor Phase 4 wires up), ordered by wave
	// number. May be empty before the first wave.
	Waves []eng.WaveSummary

	Wave, MaxWave int
	Tick, MaxTick int64

	// StartingLives/Lives drive the whole-match lives bar. Kept separate
	// from the Waves slice deliberately: the bar must be right even before
	// any wave has a summary row.
	StartingLives, Lives int

	DefAuthored, AttAuthored AuthoredShare

	// Substituted is how many decisions the engine substituted for a model
	// across the whole match; SubstitutedKnown mirrors its accessor's ok
	// return, so "not tracked" never renders as a reassuring zero.
	Substituted      int
	SubstitutedKnown bool

	// Assist carries the engine-assist and provenance state, reusing
	// render_header_v2.go's TrustState so the timeline's "engine" row and
	// the header's trust band can never disagree about the same match.
	Assist TrustState

	// RejectedDef/RejectedAtt are per-side rejected-action counts
	// (engine.Game.RejectedActions). RejectedDefReason is optional free
	// text naming the dominant rejection cause ("unaffordable placements").
	RejectedDef, RejectedAtt int
	RejectedDefReason        string

	// Ruleset is a pre-formatted one-line provenance stamp for the run
	// ("balance v3 a3f1c8   30 waves   assists on   pricing unset").
	Ruleset string
}

// --- the wave table --------------------------------------------------------

// timelineFixedRows is how many rows the pane spends on things other than
// the wave table: the title rule, the table's column header, the blank
// spacer, and the timelineSummaryRows whole-match rows below it. Whatever
// is left goes to the table.
const timelineFixedRows = 1 + 1 + 1 + timelineSummaryRows

// timelineSummaryRows is the count of whole-match rows under the table
// (lives, authored, engine, rejected, waves, horizon, ruleset).
const timelineSummaryRows = 7

// waveResult names a wave's outcome in the table's last column. A wave that
// is still taking arrivals is "in progress"; a finished wave that ended
// with lives left "held"; one that ran the defender to zero "CORE LOST" --
// shouted, because it is the only row in the table that can end the match.
func waveResult(ws eng.WaveSummary) string {
	switch {
	case !ws.Complete:
		return "in progress"
	case ws.LivesEnd <= 0:
		return "CORE LOST"
	default:
		return "held"
	}
}

// bandRow folds a run of consecutive WaveSummary rows into one table row:
// counters summed, towers taken from the last wave in the band (a snapshot,
// not a total -- summing tower counts would be meaningless), and the lives
// column spanning from the band's first LivesStart to its last LivesEnd.
// The result is "CORE LOST" if any wave in the band ran lives to zero,
// otherwise "held"; a band never contains the current wave (it is always a
// prefix of the wave list, see buildWaveTableRows), so it is never "in
// progress".
func bandRow(band []eng.WaveSummary) (label string, sent, leaked, killed, towers, livesStart, livesEnd int, result string) {
	first, last := band[0], band[len(band)-1]
	label = fmt.Sprintf("%d-%d", first.Wave, last.Wave)
	if len(band) == 1 {
		label = fmt.Sprintf("%d", first.Wave)
	}
	result = "held"
	for _, ws := range band {
		sent += ws.Sent
		leaked += ws.Leaked
		killed += ws.Killed
		if ws.LivesEnd <= 0 {
			result = "CORE LOST"
		}
	}
	return label, sent, leaked, killed, last.Towers, first.LivesStart, last.LivesEnd, result
}

// timelineRowText formats one table row (banded or single) into the column
// layout testdata/mockups/160x50.txt line 38 heads.
func timelineRowText(label string, sent, leaked, killed, towers, livesStart, livesEnd int, result string) string {
	return fmt.Sprintf("  %6s %8d %8d %8d %8d   %-8s %s",
		label, sent, leaked, killed, towers,
		fmt.Sprintf("%d->%d", livesStart, livesEnd), result)
}

// buildWaveTableRows renders at most budget rows covering EVERY wave in
// waves: the newest budget-1 waves individually, and everything older
// folded into a single leading band row. When the waves already fit, they
// are all rendered individually and no band appears.
//
// budget <= 0 yields no rows at all, and budget == 1 with more than one
// wave folds the entire match into one band -- degenerate, but still
// truthful, which is the property that matters when the pane is squeezed.
func buildWaveTableRows(waves []eng.WaveSummary, budget int) []string {
	if budget <= 0 || len(waves) == 0 {
		return nil
	}

	if len(waves) <= budget {
		rows := make([]string, 0, len(waves))
		for _, ws := range waves {
			rows = append(rows, timelineRowText(fmt.Sprintf("%d", ws.Wave),
				ws.Sent, ws.Leaked, ws.Killed, ws.Towers, ws.LivesStart, ws.LivesEnd, waveResult(ws)))
		}
		return rows
	}

	bandLen := len(waves) - (budget - 1)
	band := waves[:bandLen]
	rest := waves[bandLen:]

	rows := make([]string, 0, budget)
	label, sent, leaked, killed, towers, ls, le, result := bandRow(band)
	rows = append(rows, timelineRowText(label, sent, leaked, killed, towers, ls, le, result))
	for _, ws := range rest {
		rows = append(rows, timelineRowText(fmt.Sprintf("%d", ws.Wave),
			ws.Sent, ws.Leaked, ws.Killed, ws.Towers, ws.LivesStart, ws.LivesEnd, waveResult(ws)))
	}
	return rows
}

// --- the summary block -----------------------------------------------------

// fmtSubstituted renders the substituted-decision count with its own
// unknown state: an engine that substituted zero decisions is a very
// different claim from an engine whose substitutions were never counted,
// and this row is one of the two places in the UI that claim gets made.
func fmtSubstituted(count int, known bool) string {
	if !known {
		return "substitutions not tracked"
	}
	return fmt.Sprintf("substituted %d decisions", count)
}

// timelineSummaryRow formats one whole-match summary row. It is the wave
// table's own two-column indent plus a label field the same width as the
// player cards' (cardLabelW), so the summary block, the table above it and
// the cards pane above that all line their values up on the same column --
// which is what makes the left column read as one panel rather than three.
func timelineSummaryRow(label, value string) string {
	return fmt.Sprintf("  %-*s%s", cardLabelW, label, value)
}

// timelineSummaryLines builds the seven whole-match rows below the table.
// width is passed through for the two bar rows, which size their bars to
// whatever is left after their label and trailing caption.
func timelineSummaryLines(d TimelineData, width int) []string {
	row := timelineSummaryRow
	livesBarW := barBudget(width, 22)
	horizonBarW := barBudget(width, 26)

	rejected := fmt.Sprintf("DEF %d", d.RejectedDef)
	if d.RejectedDef > 0 && d.RejectedDefReason != "" {
		rejected = fmt.Sprintf("DEF %d turns lost to %s", d.RejectedDef, d.RejectedDefReason)
	}
	rejected += fmt.Sprintf("   ATT %d", d.RejectedAtt)

	engineRow := fmt.Sprintf("%-26s %s",
		fmtSubstituted(d.Substituted, d.SubstitutedKnown), d.Assist.assistLabel())
	if d.Assist.AssistDetail != "" {
		engineRow += ", " + d.Assist.AssistDetail
	}

	return []string{
		row("lives", fmt.Sprintf("%s  %d -> %d",
			fillBar(d.Lives, d.StartingLives, livesBarW), d.StartingLives, d.Lives)),
		row("authored", fmt.Sprintf("DEF %-6s ATT %-6s    %s",
			fmtAuthored(d.DefAuthored), fmtAuthored(d.AttAuthored), d.Assist.provenanceLabel())),
		row("engine", engineRow),
		row("rejected", rejected),
		row("waves", horizonSentenceV2(d)),
		row("horizon", fmt.Sprintf("%s  %s",
			fillBar(int(d.Tick), int(d.MaxTick), horizonBarW), fmtTick(d.Tick, d.MaxTick))),
		row("ruleset", orElse(d.Ruleset, "ruleset not stamped")),
	}
}

// horizonSentenceV2 states what will end the match. With no tick cap
// configured there is only one horizon to name, and claiming a tick limit
// that does not exist would be a fabricated fact on the row whose entire job
// is stating the match's terms.
func horizonSentenceV2(d TimelineData) string {
	if d.MaxTick <= 0 {
		return fmt.Sprintf("%d of %d played   ends at wave %d, no tick cap set",
			d.Wave, d.MaxWave, d.MaxWave)
	}
	return fmt.Sprintf("%d of %d played   ends at wave %d or tick %d, whichever comes first",
		d.Wave, d.MaxWave, d.MaxWave, d.MaxTick)
}

// barBudget sizes a summary row's bar: everything left of the row's width
// after its label column and a caption allowance, clamped to something
// drawable. Both bar rows go through this so a narrower pane shrinks them
// evenly instead of one of them overflowing.
func barBudget(width, caption int) int {
	n := width - cardLabelW - caption - 2
	if n < 4 {
		n = 4
	}
	if n > 52 {
		n = 52
	}
	return n
}

// --- the pane --------------------------------------------------------------

// timelineHeaderRow is the wave table's column header (see
// testdata/mockups/160x50.txt line 38). Its spacing is derived from the
// same format string timelineRowText uses, so the two cannot drift.
func timelineHeaderRow() string {
	return fmt.Sprintf("  %6s %8s %8s %8s %8s   %-8s %s",
		"wave", "sent", "leaked", "killed", "towers", "lives", "result")
}

// RenderTimelineV2 renders layout.timeline: exactly rc.h rows of exactly
// rc.w display columns. The row budget is spent title-rule first, then the
// seven summary rows (which are whole-match facts and must not be starved
// by a long match), and the wave table gets what is left -- banding to fit,
// never dropping waves. If even the summary rows don't fit, the pane
// degrades by keeping the first rc.h rows built, which puts the title and
// the table's most recent rows first.
func RenderTimelineV2(rc rect, d TimelineData) []string {
	if rc.h <= 0 || rc.w <= 0 {
		return blankRows(rc.h, rc.w)
	}

	tableBudget := rc.h - timelineFixedRows
	if tableBudget < 0 {
		tableBudget = 0
	}

	rows := make([]string, 0, rc.h)
	rows = append(rows, titledRuleV2(rc.w, "MATCH TIMELINE", fmtTick(d.Tick, d.MaxTick)))

	table := buildWaveTableRows(d.Waves, tableBudget)
	if len(table) > 0 {
		rows = append(rows, timelineHeaderRow())
		rows = append(rows, table...)
	} else if tableBudget > 0 {
		rows = append(rows, timelineHeaderRow())
		rows = append(rows, "  no waves have started yet")
	}
	rows = append(rows, "")
	rows = append(rows, timelineSummaryLines(d, rc.w)...)

	final := make([]string, rc.h)
	for i := 0; i < rc.h; i++ {
		if i < len(rows) {
			final[i] = padCells(truncateCells(rows[i], rc.w), rc.w)
		} else {
			final[i] = padCells("", rc.w)
		}
	}
	return final
}
