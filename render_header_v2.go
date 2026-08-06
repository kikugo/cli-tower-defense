package main

// This file renders the Phase 2 header pane (layout.header from
// main_layout_v2.go's computeLayoutV2) and the trust band -- the horizontal
// rule that carries the engine-assist state and the decision-provenance
// state, styled to ride the board's top border rule in wide mode and
// available for the board renderer (a different file, a different agent) to
// place wherever it needs to in the other modes.
//
// Nothing here reaches into *eng.Game or engine.MatchResult: every input is
// a small, pre-extracted, already-decided struct (MatchHeaderData,
// TrustState). That is deliberate, not an oversight -- the three engine
// accessors this design is built around (MatchResult.ModelAuthored,
// EngineAssistTotal, AuthoredSaves) all return (value, ok), and ok==false
// means "not measured", which must render as an explicit unknown/none-yet
// word, never as a bare zero. Keeping the boundary at a flat struct means
// that "not measured" decision is made once, by whoever populates the
// struct from the engine, and this file only ever has to render the
// three-way result (known-value / no-data-yet / unknown) it's handed --
// it can't accidentally collapse "not measured" into "measured as zero" by
// reaching past the struct for a raw field.
//
// Layout-wise this file follows main_layout_v2.go's own convention: no
// package-level state, every width/height comes from the layoutV2 passed
// in, and padCells/truncateCells/fitLines/shortName/blankRows from
// main_layout.go are reused rather than re-implemented (per the task
// brief). It does NOT reuse frameDisplayWidth/checkFits/checkNoOrphanDividers
// from mockup_fit_test.go: those live in a _test.go file and are only
// compiled into test binaries, so production rendering code measures
// display width the same way padCells/truncateCells/shortName already do
// -- via lipgloss.Width -- rather than depending on test-only code.

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// --- input data -----------------------------------------------------------

// AuthoredShare is the three-way result of engine.MatchResult.ModelAuthored:
// a real percentage, "no decisions resolved yet" (Known but !HasData), or
// "provenance not tracked on this MatchResult at all" (!Known,
// ProvenanceVersion == 0). Known is ModelAuthored's ok return value;
// HasData additionally distinguishes "ok but total==0" (which ModelAuthored
// itself reports as (0, false) -- see its doc comment) from genuine
// unknown, since the two render as different words ("none yet" vs
// "unknown") even though the engine method collapses them to the same
// (0, false). The caller populating this struct is the one who has to make
// that distinction (e.g. by checking ProvenanceVersion directly); this file
// only renders whichever of the three states it's handed.
type AuthoredShare struct {
	Known   bool
	HasData bool
	Share   float64 // 0..1, meaningful only when Known && HasData
}

// SavesStat is the three-way result of engine.MatchResult.AuthoredSaves:
// Known mirrors its ok return (false => ProvenanceVersion < 3, unknown);
// when Known, Total == 0 means no decisions have resolved yet ("none yet"),
// otherwise Authored/Total render as a ratio.
type SavesStat struct {
	Known    bool
	Authored int
	Total    int
}

// LeakStat is engine.MatchResult.RecentLeaks flattened: Window == 0 means
// nothing has resolved yet ("leaked none yet"), matching RecentLeaks'
// `full` signal at the empty end -- a caller does not need to pass `full`
// separately, since the design only ever distinguishes "window has zero
// entries" from "window has some", not "window is exactly at capacity".
type LeakStat struct {
	Leaked int
	Window int
}

// PlayerHeaderData is one side's (defender's or attacker's) header content,
// pre-extracted from *eng.Game / engine.MatchResult by the caller. Fields
// that only apply to one side (Lives/MaxLives for the defender, Live/Sent
// for the attacker) are simply left zero on the other side -- nothing here
// reads a Role tag to decide which fields matter, since MatchHeaderData
// already keeps Defender and Attacker as two distinct fields and each
// render function knows which is which.
type PlayerHeaderData struct {
	ModelName string
	Resources int
	Income    int

	// Lives/MaxLives: defender only.
	Lives, MaxLives int

	// Built: defender only, pre-formatted built-tower tally (e.g.
	// "^3 !2 *1 +1"). Empty renders as "nothing" (trust-160.txt STATE 2).
	Built string

	// Live/Sent: attacker only. Live is a pre-formatted live-enemy tally
	// (e.g. "o4 f2 t1 s1 h1"); empty renders as "none yet". Sent is the
	// cumulative enemies-sent count.
	Live string
	Sent int

	Saves    SavesStat
	Authored AuthoredShare
}

// MatchHeaderData is the complete, pure input to RenderHeaderV2: everything
// the header pane's content needs, across every mode, pre-extracted from
// engine state by the caller.
type MatchHeaderData struct {
	Defender, Attacker PlayerHeaderData

	Wave, MaxWave int
	Tick, MaxTick int64

	// TurnSide is whose turn/last decision it is, rendered as "[ATT]" /
	// "[DEF]" -- pass the short player tag ("ATT" or "DEF").
	TurnSide string
	Speed    float64
	// RunState is a short caption for the run/pause state, e.g. "RUN" or
	// "PAUSED".
	RunState string

	// Breached is the whole-match breach count (engine.Game.BreachCount /
	// MatchResult.BreachCount) -- one counter for the whole match, not
	// per-player, since a breach is one event the attacker causes and the
	// defender suffers.
	Breached int

	Leak LeakStat
}

// TrustState is the pure input to RenderTrustBand: the engine-assist state
// plus the decision-provenance state for one match, already reduced to the
// three-way (known-and-on / known-and-off / unknown) and four-way
// (off / on-not-fired / on-firing / unknown) shapes the trust band renders.
//
//   - AssistKnown mirrors EngineAssistTotal's ok return: false means this
//     MatchResult predates assist tracking (ProvenanceVersion < 2) --
//     unknown, never zero.
//   - AssistsEnabled is the ruleset's own switch (the negation of
//     ArenaRuleset.DisableAssists). Only meaningful when AssistKnown.
//   - AssistCount is EngineAssistTotal's value. Only meaningful when
//     AssistKnown && AssistsEnabled; a count of exactly 0 in that state
//     renders as "armed, nothing fired" (ENGINE ASSIST ON), never as
//     "ENGINE HELPED 0x" -- see RenderTrustBand.
//   - AssistDetail is optional free text for the clause after the headline
//     label (e.g. "armed at tick 1, has not acted", "6 unrecorded, started
//     4 of 5 waves") -- match-specific wording the caller supplies, since
//     this file has no access to *eng.Game to compute it itself. Empty is
//     fine; the row is simply shorter.
//   - ProvenanceKnown mirrors ModelAuthored's ok return (false =>
//     ProvenanceVersion == 0, unknown).
type TrustState struct {
	AssistKnown    bool
	AssistsEnabled bool
	AssistCount    int
	AssistDetail   string

	ProvenanceKnown bool
}

// --- small formatting helpers ---------------------------------------------

// fillBar renders a width-cell bar of '█' (filled) followed by '░' (empty),
// filled to round(value/max*width) cells. max<=0 renders an entirely empty
// bar rather than dividing by zero.
//
// Named fillBar rather than progressBar to avoid colliding with main.go's
// existing progressBar (a "[---|---]" replay-timeline position indicator --
// a different shape for a different purpose); this file does not touch
// main.go.
func fillBar(value, max, width int) string {
	if width <= 0 {
		return ""
	}
	if max <= 0 {
		return strings.Repeat("░", width)
	}
	n := int(math.Round(float64(value) / float64(max) * float64(width)))
	if n < 0 {
		n = 0
	}
	if n > width {
		n = width
	}
	return strings.Repeat("█", n) + strings.Repeat("░", width-n)
}

func moneyLong(resources, income int) string {
	return fmt.Sprintf("$%d +%d/t", resources, income)
}

func moneyShort(resources, income int) string {
	return fmt.Sprintf("$%d+%d", resources, income)
}

// fmtAuthored renders AuthoredShare's three states: "unknown" (!Known),
// "none yet" (Known && !HasData), or "N%" (Known && HasData). A zero
// percentage is only ever printed when HasData is true -- i.e. when the
// caller has affirmatively measured a 0% share, not merely defaulted one.
func fmtAuthored(a AuthoredShare) string {
	switch {
	case !a.Known:
		return "unknown"
	case !a.HasData:
		return "none yet"
	default:
		return fmt.Sprintf("%d%%", int(math.Round(a.Share*100)))
	}
}

// fmtSaves renders SavesStat's three states: "unknown", "none yet"
// (Total == 0), or "authored/total".
func fmtSaves(s SavesStat) string {
	switch {
	case !s.Known:
		return "unknown"
	case s.Total == 0:
		return "none yet"
	default:
		return fmt.Sprintf("%d/%d", s.Authored, s.Total)
	}
}

// fmtLeakLong and fmtLeakShort render LeakStat for the wide/mid header rows
// respectively -- both render "leaked none yet[, 0 enemies resolved]"
// rather than "leaked 0 of last 0" when the window is empty.
func fmtLeakLong(l LeakStat) string {
	if l.Window == 0 {
		return "leaked none yet, 0 enemies resolved"
	}
	return fmt.Sprintf("leaked %d of last %d enemies resolved", l.Leaked, l.Window)
}

func fmtLeakShort(l LeakStat) string {
	if l.Window == 0 {
		return "leaked none yet"
	}
	return fmt.Sprintf("leaked %d of last %d", l.Leaked, l.Window)
}

// fmtTick renders the tick counter. A MaxTick of 0 means no cap was
// configured for this run, and "tick 117/0" is worse than useless -- it
// reads as a finished match, and fillBar would draw a full bar for it. The
// uncapped case says so instead.
func fmtTick(tick, maxTick int64) string {
	if maxTick <= 0 {
		return fmt.Sprintf("tick %d, no cap", tick)
	}
	return fmt.Sprintf("tick %d/%d", tick, maxTick)
}

func builtOrNothing(s string) string {
	if s == "" {
		return "nothing"
	}
	return s
}

func liveOrNoneYet(s string) string {
	if s == "" {
		return "none yet"
	}
	return s
}

// dashFill pads s with trailing '─' runes out to exactly width display
// columns, or hard-truncates it (via truncateCells, so it never splits a
// grapheme cluster) if it's already too long. It is the trust band's
// analogue of padCells, which pads with spaces instead -- the trust band is
// a dash rule, not text on a blank background.
func dashFill(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := lipgloss.Width(s)
	if w >= width {
		return truncateCells(s, width)
	}
	return s + strings.Repeat("─", width-w)
}

// threeCol lays three pre-built strings out across exactly width display
// columns: left flush to column 0, right flush to the far edge, and center
// placed in the middle of whatever space remains between them -- the
// "mirrored scoreboard: defender identity left, attacker identity right,
// match state centre" shape the task brief specifies for wide/mid header
// rows. If the three pieces don't fit even with zero gap, center is
// dropped first (it's the least essential of the three -- the player
// identities are what the row exists to show), then right is truncated,
// then left; padCells is the final safety net that guarantees the exact
// width no matter what.
func threeCol(width int, left, center, right string) string {
	if width <= 0 {
		return ""
	}
	lw := lipgloss.Width(left)
	cw := lipgloss.Width(center)
	rw := lipgloss.Width(right)

	if cw > 0 && lw+cw+rw+2 > width {
		center = ""
		cw = 0
	}
	if rw > 0 && lw+rw+1 > width {
		budget := width - lw - 1
		if budget < 0 {
			budget = 0
		}
		right = truncateCells(right, budget)
		rw = lipgloss.Width(right)
	}
	if lw > width {
		left = truncateCells(left, width)
		lw = lipgloss.Width(left)
	}

	gap := width - lw - rw
	if gap < 0 {
		gap = 0
	}
	leftPad := (gap - cw) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	rightPad := gap - cw - leftPad
	if rightPad < 0 {
		rightPad = 0
	}

	row := left + strings.Repeat(" ", leftPad) + center + strings.Repeat(" ", rightPad) + right
	return padCells(row, width)
}

// joinFields joins fields with a fixed 3-space separator and pads/truncates
// the result to exactly width via padCells -- used by the narrow/minimum/
// compact modes' collapsed two-line form, which (unlike wide/mid) has no
// mirrored left/center/right shape to preserve, just a left-to-right list
// of stats.
func joinFields(width int, fields ...string) string {
	kept := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			kept = append(kept, f)
		}
	}
	return padCells(strings.Join(kept, "   "), width)
}

// --- the trust band ---------------------------------------------------

// assistLabel is the trust band's headline clause: "ENGINE ASSIST UNKNOWN"
// when assists aren't measured at all, "ENGINE ASSIST OFF" when the
// ruleset disabled them, "ENGINE ASSIST ON" when they're enabled but have
// not yet fired (AssistCount == 0 -- the count is deliberately never
// printed at zero), or "ENGINE HELPED Nx" once N > 0.
func (t TrustState) assistLabel() string {
	switch {
	case !t.AssistKnown:
		return "ENGINE ASSIST UNKNOWN"
	case !t.AssistsEnabled:
		return "ENGINE ASSIST OFF"
	case t.AssistCount == 0:
		return "ENGINE ASSIST ON"
	default:
		return fmt.Sprintf("ENGINE HELPED %dx", t.AssistCount)
	}
}

// provenanceLabel is the trust band's provenance clause: "provenance
// measured" when ModelAuthored is trackable on this MatchResult,
// "provenance UNKNOWN" (upper case, matching trust-160.txt's STATE 4)
// otherwise.
func (t TrustState) provenanceLabel() string {
	if !t.ProvenanceKnown {
		return "provenance UNKNOWN"
	}
	return "provenance measured"
}

// TrustBandLabel is the trust band's text WITHOUT any rule chrome: the
// assist headline plus the caller's detail clause, e.g.
// "ENGINE HELPED 2x ─ queued 4 enemies" or plain "ENGINE ASSIST OFF".
//
// It exists because the same sentence has to appear in two different frames.
// Wide mode gives the band a whole header row, which RenderTrustBand draws
// as a dash rule with a "┬" split. Every narrower mode has no spare row, so
// the board renderer embeds the same text in a border or label row it is
// already drawing (see render_board_v2.go's top-of-file note and
// testdata/mockups/100x30.txt line 19). Both paths call this, so a board
// border and a header band describing the same match cannot word the assist
// state differently -- which they did while the board owned its own
// placeholder formatter.
func TrustBandLabel(t TrustState) string {
	label := t.assistLabel()
	if t.AssistDetail != "" {
		label += " ─ " + t.AssistDetail
	}
	return label
}

// RenderTrustBand renders the trust band as ONE horizontal-rule row of
// exactly width display columns: an engine-assist clause on the left and,
// when splitCol is strictly between 0 and width, a "┬"-divided
// decision-provenance clause on the right -- the shape trust-160.txt uses,
// where the split lands at the same column (boardMaxW, 84) as the vertical
// rule directly beneath it in wide mode, so the two visually align. Pass
// splitCol <= 0 (or >= width) to render the assist clause alone across the
// full width -- e.g. for a mode whose board pane has no separate
// provenance column to divide into.
//
// This is the function the board renderer (a different file) is expected
// to call for narrow/mid/minimum/compact placement of the trust band;
// RenderHeaderV2 also calls it directly for wide mode's header row 4 (see
// trust-160.txt / 160x50.txt row 4).
func RenderTrustBand(width, splitCol int, t TrustState) string {
	if width <= 0 {
		return ""
	}

	leftClause := "─ " + TrustBandLabel(t) + " "

	if splitCol <= 0 || splitCol >= width {
		return dashFill(leftClause, width)
	}

	leftPart := dashFill(leftClause, splitCol)
	rightClause := "─ " + t.provenanceLabel() + " "
	rightPart := dashFill(rightClause, width-splitCol-1)
	return leftPart + "┬" + rightPart
}

// --- the header pane --------------------------------------------------

// nameBudget bounds how many display columns a model name is allowed
// before shortName truncates it with an ellipsis -- generous enough for
// realistic model names, but small enough that even a pathological
// 60-character name can't push a fixed-width row past its budget (threeCol
// and joinFields both also hard-truncate as a final safety net, but
// starting from a sane budget keeps normal-width names from being clipped
// unnecessarily at the tighter mode widths).
func nameBudget(mode layoutModeV2) int {
	switch mode {
	case modeWide:
		return 22
	case modeMid:
		return 14
	default:
		// 16 comfortably fits "gpt-4o-mini" (11 cells) and similar
		// real-world model names without ellipsis at the collapsed form's
		// narrowest width (80x24.txt shows the full name fitting) -- a
		// pathologically long name still degrades safely via shortName's
		// own truncation plus joinFields/padCells's row-level safety net.
		return 16
	}
}

func renderWideHeader(w int, data MatchHeaderData, trust TrustState) []string {
	nb := nameBudget(modeWide)
	defName := shortName(data.Defender.ModelName, nb)
	attName := shortName(data.Attacker.ModelName, nb)

	row1 := threeCol(w,
		fmt.Sprintf("  DEFENDER   %s", defName),
		fmt.Sprintf("WAVE %d/%d  %s   tick %d/%d",
			data.Wave, data.MaxWave, fillBar(data.Wave, data.MaxWave, 10), data.Tick, data.MaxTick),
		fmt.Sprintf("%s   ATTACKER", attName),
	)

	row2 := threeCol(w,
		fmt.Sprintf("  lives %d/%d %s   %s",
			data.Defender.Lives, data.Defender.MaxLives,
			fillBar(data.Defender.Lives, data.Defender.MaxLives, 10),
			moneyLong(data.Defender.Resources, data.Defender.Income)),
		fmt.Sprintf("TURN [%s]     speed %.1fx     %s", data.TurnSide, data.Speed, data.RunState),
		fmt.Sprintf("%s   sent %d  breached %d",
			moneyLong(data.Attacker.Resources, data.Attacker.Income), data.Attacker.Sent, data.Breached),
	)

	row3 := threeCol(w,
		fmt.Sprintf("  built %s   authored %s", builtOrNothing(data.Defender.Built), fmtAuthored(data.Defender.Authored)),
		fmtLeakLong(data.Leak),
		fmt.Sprintf("live %s   authored %s", liveOrNoneYet(data.Attacker.Live), fmtAuthored(data.Attacker.Authored)),
	)

	row4 := RenderTrustBand(w, boardMaxW, trust)

	return []string{row1, row2, row3, row4}
}

func renderMidHeader(w int, data MatchHeaderData) []string {
	nb := nameBudget(modeMid)
	defName := shortName(data.Defender.ModelName, nb)
	attName := shortName(data.Attacker.ModelName, nb)

	row1 := threeCol(w,
		fmt.Sprintf("  DEFENDER %s", defName),
		fmt.Sprintf("WAVE %d/%d    tick %d/%d", data.Wave, data.MaxWave, data.Tick, data.MaxTick),
		fmt.Sprintf("%s  ATTACKER", attName),
	)

	row2 := threeCol(w,
		fmt.Sprintf("  %d/%d %s  %s",
			data.Defender.Lives, data.Defender.MaxLives,
			fillBar(data.Defender.Lives, data.Defender.MaxLives, 10),
			moneyShort(data.Defender.Resources, data.Defender.Income)),
		fmt.Sprintf("[%s] %.1fx %s   %s", data.TurnSide, data.Speed, data.RunState, fmtLeakShort(data.Leak)),
		fmt.Sprintf("breached %d   %s", data.Breached, moneyShort(data.Attacker.Resources, data.Attacker.Income)),
	)

	row3 := threeCol(w,
		fmt.Sprintf("  %s  saves %s  %s", builtOrNothing(data.Defender.Built), fmtSaves(data.Defender.Saves), fmtAuthored(data.Defender.Authored)),
		"model-authored",
		fmt.Sprintf("saves %s  %s  %s", fmtSaves(data.Attacker.Saves), fmtAuthored(data.Attacker.Authored), liveOrNoneYet(data.Attacker.Live)),
	)

	return []string{row1, row2, row3}
}

func renderCollapsedHeader(mode layoutModeV2, w int, data MatchHeaderData) []string {
	nb := nameBudget(mode)
	defName := shortName(data.Defender.ModelName, nb)
	attName := shortName(data.Attacker.ModelName, nb)

	// Narrow mode is the one collapsed mode with no label row under it (it
	// draws a framed board, not the borderless map that minimum/compact
	// title with "W12/30 t117/400 ..."), so it is the one that has to carry
	// wave and tick in the header itself. Adding them for all three would
	// duplicate the label row at 80 columns, where space is scarcest.
	waveField := ""
	if mode == modeNarrow {
		waveField = fmt.Sprintf("W%d/%d %s", data.Wave, data.MaxWave, fmtTick(data.Tick, data.MaxTick))
	}

	row1 := joinFields(w,
		"  DEF "+defName,
		waveField,
		fmt.Sprintf("%d/%d %s", data.Defender.Lives, data.Defender.MaxLives, fillBar(data.Defender.Lives, data.Defender.MaxLives, 10)),
		moneyShort(data.Defender.Resources, data.Defender.Income),
		"saves "+fmtSaves(data.Defender.Saves),
		fmtAuthored(data.Defender.Authored)+" authored",
	)

	row2 := joinFields(w,
		"  ATT "+attName,
		fmt.Sprintf("breached %d", data.Breached),
		moneyShort(data.Attacker.Resources, data.Attacker.Income),
		"saves "+fmtSaves(data.Attacker.Saves),
		fmtAuthored(data.Attacker.Authored)+" authored",
	)

	return []string{row1, row2}
}

// RenderHeaderV2 renders the header pane -- layout.header from
// computeLayoutV2 -- for every mode that has one: exactly l.header.h rows,
// each exactly l.header.w display columns (checkFits' invariant; the final
// fitLines call is the safety net that guarantees it even if a row builder
// above ever drifted).
//
// Content per mode, per the task brief:
//   - wide (4 rows) and mid (3 rows): the mirrored scoreboard -- defender
//     identity left, attacker identity right, match state centre. Wide's
//     4th row is the trust band (RenderTrustBand), split at boardMaxW to
//     align with the vertical rule directly beneath it; mid has no room
//     for it (3 rows are all scoreboard) -- the board renderer places the
//     trust band itself for that mode, via RenderTrustBand.
//   - narrow/minimum/compact (2 rows): the collapsed two-line form.
//
// modeNotice (or a degenerate zero-size header) renders as blank rows --
// notice mode has no header pane at all in computeLayoutV2 (l.header is
// its zero value), and this function does not special-case that: it just
// pads out l.header.h/l.header.w, whatever they are.
func RenderHeaderV2(l layoutV2, data MatchHeaderData, trust TrustState) []string {
	w, h := l.header.w, l.header.h
	if w <= 0 || h <= 0 {
		return blankRows(h, w)
	}

	var rows []string
	switch l.mode {
	case modeWide:
		rows = renderWideHeader(w, data, trust)
	case modeMid:
		rows = renderMidHeader(w, data)
	case modeNarrow, modeMinimum, modeCompact:
		rows = renderCollapsedHeader(l.mode, w, data)
	default:
		return blankRows(h, w)
	}

	return fitLines(rows, w, h)
}
