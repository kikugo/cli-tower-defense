package main

// The Phase 2 player-cards pane: layout.cards' renderer (wide mode only --
// computeLayoutV2 only allocates a cards rect in modeWide; every other mode
// leaves it zero-valued). See testdata/mockups/160x50.txt lines 21-36 for
// the shape this produces: a titled "─ DEFENDER ── ATTACKER ─" rule over
// two side-by-side identity cards, each a stack of label/value rows ending
// in the model's own last reasoning quote.
//
// Nothing here is wired into main.go's View() yet, exactly like every other
// *_v2.go file in this phase; the cutover happens in Phase 4.
//
// --- Why this file takes flat structs and never an *eng.Game --------------
//
// Same reason render_header_v2.go does, and it matters more here than
// anywhere else in the redesign: this pane is the match's honesty report.
// Nine of its eighteen rows are provenance/assist/saves figures whose
// engine accessors return (value, ok), where ok==false means "not measured"
// and MUST render as a word, never as a zero. Half the point of the pane is
// that a reader can tell "the attacker authored 0% of its decisions" from
// "we never recorded who authored the attacker's decisions". If this file
// could reach into *eng.Game it could quietly read a raw counter and print
// a confident 0 for both. It can't, so it can't.
//
// AuthoredShare and SavesStat are deliberately REUSED from
// render_header_v2.go rather than redeclared here: the header and the cards
// render the same two quantities, and one shared three-state type means
// the "unknown vs none-yet vs measured" decision is made once by whoever
// populates it, not twice with a chance of disagreeing. fmtAuthored and
// fmtSaves are reused for the same reason.
//
// --- Degradation -----------------------------------------------------------
//
// computeLayoutV2 asks for 16 rows of cards but pays via payRows, so at the
// low end of wide mode's valid height the pane can be handed fewer (or
// zero). Rows are built in a fixed order -- rule, name, role blurb, blank,
// stats, quote -- and the pane keeps the FIRST rc.h of them. That order is
// the priority order on purpose: squeezing the pane drops the reasoning
// quote first, then the least-structural stat rows, and never the two rows
// that say which model is which.

import (
	"fmt"
	"strings"
)

// --- input data -----------------------------------------------------------

// CardsPlayerData is one side's card content, pre-extracted from
// *eng.Game / engine.MatchResult by the caller. Fields that only apply to
// one side (Lives/MaxLives and Built/Research for the defender, Breaches/
// Sent/Live/Abilities for the attacker) are simply left zero on the other
// side; nothing here reads a role tag, because MatchCardsData keeps
// Defender and Attacker as two distinct fields and each builder below knows
// which is which.
type CardsPlayerData struct {
	ModelName string

	// Lives/MaxLives: defender only.
	Lives, MaxLives int
	// Breaches: attacker only -- how many times this side reached the core.
	Breaches int

	Resources, Income int

	// Built: defender only, a pre-formatted tower tally ("^3  !2  *1  +1").
	// Empty renders as "nothing yet".
	Built string
	// Research: defender only, pre-formatted ("economy 2, range 1"). Empty
	// renders as "none bought".
	Research string

	// Sent/Live: attacker only -- cumulative enemies sent, and how many are
	// on the board right now.
	Sent, Live int
	// Abilities: attacker only, pre-formatted ("surge 1, shield_burst 1").
	// Empty renders as "none used".
	Abilities string

	Authored AuthoredShare
	Saves    SavesStat

	// Calls/Tokens are provider-call telemetry (engine.Game.ProviderCalls /
	// ProviderTokenUsage). A scripted side legitimately has zero of both, so
	// unlike the provenance figures these need no unknown state.
	Calls, Tokens int

	// AssistKnown mirrors engine.MatchResult.EngineAssistTotal's ok return
	// (false => ProvenanceVersion < 2, unknown). AssistCount is how many
	// engine assists fired FOR THIS SIDE -- zero with AssistKnown true means
	// a measured zero ("none on this side"), which is a different and much
	// more useful statement than the unknown.
	AssistKnown bool
	AssistCount int

	// Streak/StreakMax is this side's progress toward triggering an engine
	// assist (engine.Game.NoopStreak against AutoDefendMinStreak). The
	// design shows it because it is the only forward-looking number on the
	// card: it says an assist is ABOUT to fire, before it does.
	Streak, StreakMax int

	// Reasoning is the model's own last stated reason, rendered as a quote.
	// Empty renders as no quote at all (the rows are simply blank).
	Reasoning string
}

// MatchCardsData is the complete, pure input to RenderCardsV2.
type MatchCardsData struct {
	Defender, Attacker CardsPlayerData
}

// --- small formatters ------------------------------------------------------

// orElse returns s, or fallback when s is empty -- the pattern
// builtOrNothing/liveOrNoneYet follow in render_header_v2.go, generalised
// here because this pane has five such fields rather than two and a
// named one-liner apiece would be noise.
func orElse(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// fmtCardAssist renders a side's assist figure in the card's vocabulary:
// "unknown" when assists were never tracked, "none on this side" for a
// measured zero, and the shouted form once the engine has actually acted
// for this player. The shout is deliberate -- an engine-assisted decision
// is the thing this whole telemetry effort exists to stop hiding.
func fmtCardAssist(known bool, count int) string {
	switch {
	case !known:
		return "unknown, not tracked"
	case count == 0:
		return "none on this side"
	default:
		return fmt.Sprintf("ENGINE ACTED %dx FOR THIS SIDE", count)
	}
}

// fmtCardSaves spells out what fmtSaves abbreviates: the header row has to
// fit "17/59" into a shared line, but the card has a full column and the
// word "model-authored" is the part a reader needs -- a save the MODEL
// chose is a strategic decision, a save the engine substituted is not.
func fmtCardSaves(s SavesStat) string {
	switch {
	case !s.Known:
		return "unknown, not tracked"
	case s.Total == 0:
		return "none yet"
	default:
		return fmt.Sprintf("%d of %d  model-authored", s.Authored, s.Total)
	}
}

// fmtStreak renders the assist-progress counter. A StreakMax of 0 means the
// caller has no threshold to report (e.g. assists are disabled), so the row
// says so rather than printing a meaningless "0 of 0".
func fmtStreak(streak, max int) string {
	if max <= 0 {
		return "no assist threshold set"
	}
	return fmt.Sprintf("%d of %d toward an assist", streak, max)
}

// fmtCardTokens renders a token count for the card's calls row. The
// magnitude abbreviation itself is view_render.go's existing fmtTokens,
// reused rather than reimplemented (it already handles the millions case
// this would have got wrong); all this adds is the unit, which the old
// stats table omits because its column header carries it and the card has
// no column headers.
func fmtCardTokens(tokens int) string {
	return fmtTokens(tokens) + " tok"
}

// --- one card's rows -------------------------------------------------------

// cardLabelW is the width of a card row's label column ("lives", "economy",
// "authored", ...), matching testdata/mockups/160x50.txt lines 25-33 where
// every value starts at the same column regardless of label length.
const cardLabelW = 11

// cardRow formats one label/value row of a card: a leading space, the label
// padded to cardLabelW, then the value. Truncation to the column width is
// left to the caller (renderCardColumn), which owns the width.
func cardRow(label, value string) string {
	return fmt.Sprintf(" %-*s%s", cardLabelW, label, value)
}

// authoredCardRow is the one stat row that gets a bar as well as a number,
// because the share is the headline figure of the whole pane. The bar is
// only drawn when there IS a measured share -- an unknown renders as the
// word alone, never as an empty bar, which would read as a measured 0%.
func authoredCardRow(a AuthoredShare, barW int) string {
	if !a.Known || !a.HasData {
		return cardRow("authored", fmtAuthored(a))
	}
	return cardRow("authored", fmt.Sprintf("%-6s%s",
		fmtAuthored(a), fillBar(int(a.Share*1000), 1000, barW)))
}

// defenderCardStats is the defender's nine stat rows, in the order
// testdata/mockups/160x50.txt uses: what it is defending, what it is
// spending, what it has bought, then the four provenance rows.
func defenderCardStats(p CardsPlayerData, barW int) []string {
	return []string{
		cardRow("lives", fmt.Sprintf("%-6s%s",
			fmt.Sprintf("%d/%d", p.Lives, p.MaxLives), fillBar(p.Lives, p.MaxLives, 10))),
		cardRow("economy", fmt.Sprintf("%-6s+%d/tick", fmt.Sprintf("$%d", p.Resources), p.Income)),
		cardRow("built", orElse(p.Built, "nothing yet")),
		cardRow("research", orElse(p.Research, "none bought")),
		authoredCardRow(p.Authored, barW),
		cardRow("calls", fmt.Sprintf("%-6d%s", p.Calls, fmtCardTokens(p.Tokens))),
		cardRow("assist", fmtCardAssist(p.AssistKnown, p.AssistCount)),
		cardRow("saves", fmtCardSaves(p.Saves)),
		cardRow("streak", fmtStreak(p.Streak, p.StreakMax)),
	}
}

// attackerCardStats mirrors defenderCardStats row for row -- same count,
// same order, same labels wherever the quantity is the same one -- so the
// two columns read as a scoreboard rather than as two unrelated lists. The
// three rows that differ (breaches/sent/abilities against lives/built/
// research) sit at the same heights as their opposite numbers.
func attackerCardStats(p CardsPlayerData, barW int) []string {
	return []string{
		cardRow("breaches", fmt.Sprintf("%-6d%s", p.Breaches, "lives taken")),
		cardRow("economy", fmt.Sprintf("%-6s+%d/tick", fmt.Sprintf("$%d", p.Resources), p.Income)),
		cardRow("sent", fmt.Sprintf("%d total    live %d", p.Sent, p.Live)),
		cardRow("abilities", orElse(p.Abilities, "none used")),
		authoredCardRow(p.Authored, barW),
		cardRow("calls", fmt.Sprintf("%-6d%s", p.Calls, fmtCardTokens(p.Tokens))),
		cardRow("assist", fmtCardAssist(p.AssistKnown, p.AssistCount)),
		cardRow("saves", fmtCardSaves(p.Saves)),
		cardRow("streak", fmtStreak(p.Streak, p.StreakMax)),
	}
}

// quoteRowsV2 renders a model's reasoning as an opening-quoted block of
// exactly rows lines, word-wrapped to width columns, with a closing quote
// on the last non-empty line. Returns all-blank rows for empty reasoning
// rather than a lone pair of quote marks, and truncates (never wraps past
// the budget) when the reasoning is longer than the block -- a clipped
// quote is fine, a card that pushes the timeline off the frame is not.
//
// The wrapping is done here, by hand, rather than via fitLines/lipgloss's
// Width(): those pad every line out to the full width with spaces, which is
// exactly wrong for the continuation-indent shape this block needs (see
// testdata/mockups/160x50.txt lines 34-36, where continuation lines are
// indented one column past the opening quote).
func quoteRowsV2(reasoning string, width, rows int) []string {
	if rows <= 0 {
		return nil
	}
	out := make([]string, rows)
	if strings.TrimSpace(reasoning) == "" || width <= 2 {
		return out
	}

	// Budget: the first line spends one column on the opening quote, every
	// line spends one on the leading space, and the last spends one on the
	// closing quote.
	const indent = " "
	body := width - len(indent) - 1

	words := strings.Fields(reasoning)
	lines := make([]string, 0, rows)
	cur := ""
	for _, word := range words {
		candidate := word
		if cur != "" {
			candidate = cur + " " + word
		}
		if len([]rune(candidate)) > body && cur != "" {
			lines = append(lines, cur)
			if len(lines) == rows {
				break
			}
			cur = word
			continue
		}
		cur = candidate
	}
	if len(lines) < rows && cur != "" {
		lines = append(lines, cur)
	}
	if len(lines) == 0 {
		return out
	}

	last := len(lines) - 1
	for i, line := range lines {
		prefix := indent + " "
		if i == 0 {
			prefix = indent + `"`
		}
		suffix := ""
		if i == last {
			suffix = `"`
		}
		out[i] = prefix + line + suffix
	}
	return out
}

// renderCardColumn assembles one side's whole column: identity, role blurb,
// a blank spacer, the stat rows, then the reasoning quote -- padded and
// truncated to exactly width columns per row. The row ORDER here is the
// degradation priority documented at the top of this file; the caller
// truncates from the end.
func renderCardColumn(p CardsPlayerData, blurb string, stats []string, width, quoteRows int) []string {
	rows := make([]string, 0, 4+len(stats)+quoteRows)
	rows = append(rows, " "+shortName(p.ModelName, width-1))
	rows = append(rows, " "+blurb)
	rows = append(rows, "")
	rows = append(rows, stats...)
	rows = append(rows, quoteRowsV2(p.Reasoning, width, quoteRows)...)

	for i, r := range rows {
		rows[i] = padCells(truncateCells(r, width), width)
	}
	return rows
}

// --- the pane --------------------------------------------------------------

// cardsQuoteRows is how many rows the reasoning block gets at the pane's
// full 16-row height (testdata/mockups/160x50.txt lines 34-36).
const cardsQuoteRows = 3

// RenderCardsV2 renders layout.cards: exactly rc.h rows of exactly rc.w
// display columns, whatever rc.h is. Row 0 is the titled
// "─ DEFENDER ── ATTACKER ─" rule (titledRuleV2 from render_board_v2.go
// gives the left label; the ATTACKER label is placed at the column split so
// it sits directly above the attacker column rather than at the far right
// edge, matching the fixture). Rows 1.. are the two columns side by side.
//
// The pane is only ever allocated in wide mode, where rc.w is boardMaxW
// (84), but nothing here assumes that: the column split is computed from
// rc.w so a future mode with a different cards width renders correctly
// rather than silently overflowing.
func RenderCardsV2(rc rect, data MatchCardsData) []string {
	if rc.h <= 0 || rc.w <= 0 {
		return blankRows(rc.h, rc.w)
	}

	leftW := rc.w / 2
	rightW := rc.w - leftW
	if leftW <= 0 || rightW <= 0 {
		return blankRows(rc.h, rc.w)
	}

	// Bar width scales with the column so the authored bar never pushes its
	// row past the split; 18 is what the 42-column fixture shows.
	barW := leftW - 24
	if barW < 4 {
		barW = 4
	}
	if barW > 18 {
		barW = 18
	}

	left := renderCardColumn(data.Defender, "holds the core, places towers",
		defenderCardStats(data.Defender, barW), leftW, cardsQuoteRows)
	right := renderCardColumn(data.Attacker, "sends waves, must reach the core",
		attackerCardStats(data.Attacker, barW), rightW, cardsQuoteRows)

	out := make([]string, 0, rc.h)
	out = append(out, cardsTitleRow(rc.w, leftW))
	for i := 0; i < len(left) || i < len(right); i++ {
		l := padCells("", leftW)
		if i < len(left) {
			l = left[i]
		}
		r := padCells("", rightW)
		if i < len(right) {
			r = right[i]
		}
		out = append(out, l+r)
	}

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

// cardsTitleRow builds the pane's header rule with "DEFENDER" over the left
// column and "ATTACKER" over the right one -- two titledRuleV2 segments
// butted together at splitCol rather than one rule with a far-right label,
// so each label sits above the column it names even as the split moves with
// rc.w.
func cardsTitleRow(w, splitCol int) string {
	if splitCol <= 0 || splitCol >= w {
		return titledRuleV2(w, "DEFENDER", "ATTACKER")
	}
	return titledRuleV2(splitCol, "DEFENDER", "") + titledRuleV2(w-splitCol, "ATTACKER", "")
}
