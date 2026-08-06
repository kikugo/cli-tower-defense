package main

// The Phase 2 game-over card: the modal panel the redesign floats over the
// board when a match ends, plus the compositor that places it there. See
// testdata/mockups/gameover-100x30.txt lines 6-18.
//
// Nothing here is wired into main.go's View() yet; the cutover is Phase 4.
//
// --- Why this is an overlay and not a pane ---------------------------------
//
// computeLayoutV2 has no game-over rect, deliberately: the card appears at
// one instant, over content that is still meaningful behind it (the final
// board position is part of the story of how the match ended), and giving
// it a pane would mean every mode's layout carried a rect that is unused
// for the entire match. So it composites onto already-rendered rows, which
// makes it work identically in every mode for free -- the fixture shows it
// at 100x30 (mid), and the same call places it in wide or narrow.
//
// --- Compositing over styled rows ------------------------------------------
//
// The rows this overlays are the board's, and render_board_v2.go emits one
// ANSI sequence: the breach marker's reverse video. Cutting a styled string
// at a display column with a byte or rune slice would split an escape
// sequence and corrupt the rest of the terminal line, so neither cut here
// is done that way. The LEFT cut goes through padCells, which already
// documents itself as ANSI-safe (it shrinks via reflow/truncate.String,
// which treats a complete escape sequence as atomic). The RIGHT cut --
// "drop the first N display columns" -- has no equivalent helper in this
// codebase, so dropCellsV2 below implements it explicitly, and
// TestDropCellsV2PreservesStyling is what proves it rather than trusting
// the reasoning.
//
// --- Why the verdict line is not computed here -----------------------------
//
// "TICK CAP, ENGINE-ASSISTED" is a judgement about whether a result should
// be trusted, combining how the match ended with whether the engine acted.
// That judgement belongs with whoever knows the ruleset and the provenance
// version, not with a renderer, so GameOverData takes it as text. Same for
// the cost line: this project has shipped a wrong number before by having a
// renderer infer one, and "pricing unset -- unknown" is a legitimate value
// this field must be able to carry verbatim.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/ansi"
	"github.com/rivo/uniseg"
)

// --- input data -----------------------------------------------------------

// GameOverData is the complete, pure input to RenderGameOverCardV2.
type GameOverData struct {
	// WinnerName/WinnerRole identify the winner ("o3" / "DEFENDER"). Leave
	// WinnerRole empty for a draw, which renders as "no winner".
	WinnerName, WinnerRole string

	// EndedBy is the terse cause ("TICK CAP", "CORE LOST", "WAVES CLEARED")
	// and EndedDetail its expansion ("400 ticks reached").
	EndedBy, EndedDetail string

	Wave, MaxWave   int
	Lives, MaxLives int

	DefName, AttName   string
	DefScore, AttScore int

	DefAuthored, AttAuthored AuthoredShare
	DefSaves, AttSaves       SavesStat

	// Assist reuses render_header_v2.go's TrustState so the card's engine
	// line and the trust band that was on screen a tick earlier cannot
	// contradict each other.
	Assist TrustState

	RejectedDef       int
	RejectedDefReason string

	// Cost is a pre-formatted cost line. "pricing unset -- unknown" is the
	// expected value whenever token pricing was not configured; this file
	// never fabricates a number for it.
	Cost string

	// Verdict is the caller's one-line trust judgement on the result.
	Verdict string
}

// --- formatting ------------------------------------------------------------

// commaInt formats n with thousands separators ("1240" -> "1,240"), which
// is what the fixture's score line shows and what keeps a four- and a
// five-figure score visually distinguishable at a glance.
func commaInt(n int) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// gameOverCardRow formats one label/value row of the card, using the same
// label column width as the player cards (cardLabelW) so the two panels
// read as the same family.
func gameOverCardRow(label, value string) string {
	return fmt.Sprintf("  %-*s%s", cardLabelW, label, value)
}

// gameOverBodyLines builds the card's eleven content lines in the fixture's
// order: what happened, then who won on what, then the four trust rows,
// then the verdict that summarises them.
func gameOverBodyLines(d GameOverData) []string {
	winner := "no winner   DRAW"
	if d.WinnerRole != "" {
		winner = fmt.Sprintf("%s   %s", d.WinnerName, d.WinnerRole)
	}

	rejected := fmt.Sprintf("DEF %d", d.RejectedDef)
	if d.RejectedDef > 0 && d.RejectedDefReason != "" {
		rejected = fmt.Sprintf("DEF %d turns %s", d.RejectedDef, d.RejectedDefReason)
	}

	return []string{
		"MATCH OVER",
		gameOverCardRow("WINNER", winner),
		gameOverCardRow("ended by", strings.TrimSpace(d.EndedBy+"   "+d.EndedDetail)),
		gameOverCardRow("reached", fmt.Sprintf("wave %d of %d     lives %d/%d",
			d.Wave, d.MaxWave, d.Lives, d.MaxLives)),
		gameOverCardRow("score", fmt.Sprintf("DEF %-14s ATT %s",
			commaInt(d.DefScore), commaInt(d.AttScore))),
		gameOverCardRow("authored", fmt.Sprintf("DEF %-14s ATT %s",
			fmtAuthored(d.DefAuthored), fmtAuthored(d.AttAuthored))),
		gameOverCardRow("saves", fmt.Sprintf("DEF %-14s ATT %s",
			fmtSaves(d.DefSaves), fmtSaves(d.AttSaves))),
		gameOverCardRow("engine", gameOverAssistLine(d.Assist)),
		gameOverCardRow("rejected", rejected),
		gameOverCardRow("cost", orElse(d.Cost, "pricing unset -- unknown")),
		gameOverCardRow("verdict", orElse(d.Verdict, "not judged")),
	}
}

// gameOverAssistLine renders the assist state for the card's engine row.
// It reuses TrustState.assistLabel so a match that showed "ENGINE HELPED
// 9x" on the trust band a tick before the end cannot show something else
// here, and appends the detail clause when the caller supplied one.
func gameOverAssistLine(t TrustState) string {
	line := t.assistLabel()
	if t.AssistDetail != "" {
		line += ", " + t.AssistDetail
	}
	return line
}

// --- the card ---------------------------------------------------------------

// gameOverCardW is the card's total width including its two border
// columns, matching testdata/mockups/gameover-100x30.txt (46 columns). The
// card is a fixed size rather than a fraction of the terminal: its content
// is a fixed set of eleven labelled rows, so scaling it with the frame
// would only ever add whitespace.
const gameOverCardW = 46

// RenderGameOverCardV2 renders the modal card itself: a rounded box of
// exactly gameOverCardW columns and len(body)+2 rows, its first content row
// centred as a title and the rest flush left. It returns the card alone;
// OverlayCenteredV2 is what places it over a rendered pane.
func RenderGameOverCardV2(d GameOverData) []string {
	body := gameOverBodyLines(d)
	innerW := gameOverCardW - 2

	out := make([]string, 0, len(body)+2)
	out = append(out, "╭"+strings.Repeat("─", innerW)+"╮")
	for i, line := range body {
		content := line
		if i == 0 {
			content = centreCells(line, innerW)
		}
		out = append(out, "│"+padCells(truncateCells(content, innerW), innerW)+"│")
	}
	out = append(out, "╰"+strings.Repeat("─", innerW)+"╯")
	return out
}

// centreCells centres s within width display columns using plain leading
// spaces. Unlike threeCol (render_header_v2.go) it has no left/right
// content to balance against, so it is a simpler and separate helper rather
// than a threeCol call with two empty arguments -- which would centre s in
// the same place but read as though the empties meant something.
func centreCells(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return truncateCells(s, width)
	}
	return strings.Repeat(" ", (width-w)/2) + s
}

// --- compositing -------------------------------------------------------------

// dropCellsV2 returns s with its first n display columns removed, keeping
// every ANSI escape sequence intact: sequences encountered inside the
// dropped region are not printable columns, so they are collected and
// re-emitted ahead of the kept remainder rather than discarded (dropping
// them would leave the remainder unstyled, or worse, leave a style opened
// before the cut with no closer). Grapheme clusters are walked whole, via
// the same uniseg primitive truncateCells uses, so a cut never lands inside
// one and never emits invalid UTF-8.
//
// A cut that would land in the MIDDLE of a wide (two-column) cluster drops
// that whole cluster; the result is then one column narrower than
// len(s)-n, which callers must not assume away. OverlayCenteredV2 below
// does not: it pads the composed row to the exact frame width regardless.
func dropCellsV2(s string, n int) string {
	if n <= 0 {
		return s
	}

	var styles strings.Builder
	var out strings.Builder
	col := 0
	state := -1
	rest := s

	for len(rest) > 0 {
		if rest[0] == 0x1b {
			// Collect the whole escape sequence as one unit.
			end := 1
			for end < len(rest) && !ansi.IsTerminator(rune(rest[end])) {
				end++
			}
			if end < len(rest) {
				end++
			}
			seq := rest[:end]
			if col >= n {
				out.WriteString(seq)
			} else {
				styles.WriteString(seq)
			}
			rest = rest[end:]
			// Reset the grapheme-break state across the sequence. uniseg's
			// state is a continuation of the PRECEDING text, and an escape
			// sequence is not text -- carrying the state across one makes
			// uniseg treat the next cluster as a continuation of the last
			// one before the escape and report its width as 0. That is not
			// hypothetical: it is what this function did before this line
			// existed, and it silently dropped one column per escape, so a
			// row with two escapes in it came back two columns short. The
			// worst case for resetting is that a cluster spanning an escape
			// gets split -- but a cluster spanning an escape is already a
			// discontinuity nothing downstream could render correctly.
			state = -1
			continue
		}

		cluster, remainder, w, newState := uniseg.FirstGraphemeClusterInString(rest, state)
		state = newState
		rest = remainder
		if col >= n {
			out.WriteString(cluster)
		}
		col += w
	}

	return styles.String() + out.String()
}

// OverlayCenteredV2 composites card over base, centred both ways, and
// returns exactly len(base) rows of exactly width display columns each.
// Rows the card does not cover are returned padded but otherwise
// untouched; rows it does cover are rebuilt as
// left-of-card + card row + right-of-card, with both cuts made
// ANSI-safely (padCells for the left, dropCellsV2 for the right) so a
// styled glyph beside the card -- the board's reverse-video breach marker
// is the one this codebase actually emits -- survives the composite.
//
// A card taller or wider than the frame is clipped by the same padCells/
// row-count discipline every other pane in this phase uses, rather than
// being refused: a cramped card is still readable, a frame that grew a row
// is not.
func OverlayCenteredV2(base, card []string, width int) []string {
	out := make([]string, len(base))
	for i, row := range base {
		out[i] = padCells(row, width)
	}
	if len(card) == 0 || width <= 0 {
		return out
	}

	cardW := 0
	for _, row := range card {
		if w := lipgloss.Width(row); w > cardW {
			cardW = w
		}
	}
	if cardW > width {
		cardW = width
	}

	x := (width - cardW) / 2
	y := (len(base) - len(card)) / 2
	if y < 0 {
		y = 0
	}

	for i, cardRow := range card {
		row := y + i
		if row < 0 || row >= len(out) {
			continue
		}
		left := padCells(out[row], x)
		mid := padCells(truncateCells(cardRow, cardW), cardW)
		right := dropCellsV2(out[row], x+cardW)
		out[row] = padCells(left+mid+right, width)
	}
	return out
}
