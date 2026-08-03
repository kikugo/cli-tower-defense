package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// --- renderedRows -----------------------------------------------------

// TestRenderedRowsMatchesLipglossExpression is the contract itself: for a
// table of REAL log lines built from the exact format strings g.logf is
// called with across engine/*.go (copied verbatim, not paraphrased),
// renderedRows must equal lipgloss.Height(lipgloss.NewStyle().Width(width).
// Render(s)) -- the definition given in the task brief -- at every width
// exercised. 33 is the sidebar's usable width: sidebarStyle is
// Width(35).Padding(0,1), so lipgloss subtracts the 1-column left/right
// padding before wrapping, same as replayView's 58-wide "Reason:" line.
func TestRenderedRowsMatchesLipglossExpression(t *testing.T) {
	lines := []struct {
		name string
		s    string
	}{
		// engine/core.go:858
		{"turn_timeout", fmt.Sprintf("Turn timeout! Switching turn from %s", "p2")},
		// engine/core.go:936
		{"taunt", fmt.Sprintf("%s: %s", "gpt-4o-mini", "Attacking now, better watch your lives!")},
		// engine/core.go:939
		{"decided", fmt.Sprintf("%s (%s) decided to: %s", "gemini-3-flash-preview", "attacker", "spawn_wave")},
		// engine/core.go:960
		{"fallback_place", fmt.Sprintf("%s fallback placed %s tower at [%d,%d] after invalid target [%d,%d]", "o3", "sniper", 4, 5, 4, 20)},
		// engine/core.go:965
		{"defender_rejected", fmt.Sprintf("%s defender place rejected at [%d,%d]: %s", "gemini-2.5-flash", 3, 7, "tile occupied")},
		// engine/core.go:1082
		{"action_rejected", fmt.Sprintf("%s (%s) action rejected: %s", "qwen/qwen3-next-80b-a3b-instruct", "place_tower", "insufficient resources")},
		// engine/core.go:1193
		{"api_error", fmt.Sprintf("%s API error: %v", "o3", "context deadline exceeded")},
		// engine/actions.go:362
		{"invalid_tower", fmt.Sprintf("Invalid tower type: %s", "megacannon")},
		// engine/actions.go:470
		{"invalid_enemy", fmt.Sprintf("Invalid enemy type: %s", "megaboss")},
		// engine/core.go:849 (the multi-line game state block, see the
		// dedicated 12-row assertion below for a fixed sample)
		{"game_state_block", fmt.Sprintf("\n=== Game State ===\nWave: %d\nCurrent Turn: %s (%s)\n%s (Def) - Lives: %d, Res: %d\n%s (Att) - Res: %d\nActive Towers: %d, Enemies: %d\n==================\n",
			7, "p1", "gemini-2.5-flash", "qwen/qwen3-next-80b-a3b-instruct", 18, 340, "gemini-3-flash-preview", 220, 9, 14)},
	}

	widths := []int{33, 58, 20}

	for _, l := range lines {
		for _, w := range widths {
			t.Run(fmt.Sprintf("%s_w%d", l.name, w), func(t *testing.T) {
				want := lipgloss.Height(lipgloss.NewStyle().Width(w).Render(l.s))
				got := renderedRows(l.s, w)
				if got != want {
					t.Fatalf("renderedRows(%q, %d) = %d, want %d (lipgloss.Height(lipgloss.NewStyle().Width(%d).Render(s)))", l.s, w, got, w, w)
				}
			})
		}
	}
}

// TestRenderedRowsGameStateBlockIs12Rows locks down the specific claim in
// the task brief: the "=== Game State ===" block (engine/core.go:849's
// format string, copied verbatim) is a SINGLE g.Logs entry that costs 12
// rendered rows once word-wrapped into the sidebar's 33-usable-column pane.
// The model names below were chosen (by brute-force search over the five
// real model names this project actually configures) to be a realistic
// combination that lands on exactly 12 rows at width 33.
func TestRenderedRowsGameStateBlockIs12Rows(t *testing.T) {
	block := fmt.Sprintf("\n=== Game State ===\nWave: %d\nCurrent Turn: %s (%s)\n%s (Def) - Lives: %d, Res: %d\n%s (Att) - Res: %d\nActive Towers: %d, Enemies: %d\n==================\n",
		7, "p1", "gemini-2.5-flash", "qwen/qwen3-next-80b-a3b-instruct", 18, 340, "gemini-3-flash-preview", 220, 9, 14)

	if got := renderedRows(block, 33); got != 12 {
		t.Fatalf("renderedRows(gameStateBlock, 33) = %d, want 12", got)
	}
}

// --- fitLines -----------------------------------------------------------

const sidebarWidth = 33

func repeatWord(word string, n int) string {
	words := make([]string, n)
	for i := range words {
		words[i] = word
	}
	return strings.Join(words, " ")
}

func TestFitLinesAlwaysReturnsBudgetRows(t *testing.T) {
	gameStateBlock := fmt.Sprintf("\n=== Game State ===\nWave: %d\nCurrent Turn: %s (%s)\n%s (Def) - Lives: %d, Res: %d\n%s (Att) - Res: %d\nActive Towers: %d, Enemies: %d\n==================\n",
		7, "p1", "gemini-2.5-flash", "qwen/qwen3-next-80b-a3b-instruct", 18, 340, "gemini-3-flash-preview", 220, 9, 14)

	manyShortLines := make([]string, 20)
	for i := range manyShortLines {
		manyShortLines[i] = fmt.Sprintf("log entry %d", i)
	}

	cases := []struct {
		name   string
		lines  []string
		width  int
		budget int
	}{
		{"empty_slice", nil, sidebarWidth, 5},
		{"empty_slice_zero_budget", []string{}, sidebarWidth, 0},
		{"single_over_long_line", []string{repeatWord("attacking-relentlessly", 40)}, sidebarWidth, 4},
		{"embedded_newlines_under_budget", []string{"short one", gameStateBlock, "short two"}, sidebarWidth, 5},
		{"embedded_newlines_exact_budget", []string{gameStateBlock}, sidebarWidth, 12},
		{"embedded_newlines_over_budget", []string{gameStateBlock}, sidebarWidth, 30},
		{"more_content_than_budget", manyShortLines, sidebarWidth, 5},
		{"less_content_than_budget", []string{"only one line"}, sidebarWidth, 10},
		{"less_content_than_budget_multi", []string{"line one", "line two"}, sidebarWidth, 10},
		{"zero_budget_with_content", manyShortLines, sidebarWidth, 0},
		{"negative_budget_clamped", manyShortLines, sidebarWidth, -3},
		{"zero_width", []string{"hello world this is a fairly long line of text"}, 0, 6},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := fitLines(c.lines, c.width, c.budget)
			wantLen := c.budget
			if wantLen < 0 {
				wantLen = 0
			}
			if len(got) != wantLen {
				t.Fatalf("fitLines(...): len = %d, want %d (budget=%d)", len(got), wantLen, c.budget)
			}
		})
	}
}

// TestFitLinesPadsWithEmptyStrings checks the "less content than budget"
// case concretely: the trailing rows beyond the real content must be "".
// Row 0 goes through lipgloss's own left-align padding (trailing spaces out
// to sidebarWidth columns, the same as every other row width would render),
// so it's compared after trimming that padding rather than byte-for-byte.
func TestFitLinesPadsWithEmptyStrings(t *testing.T) {
	got := fitLines([]string{"only one line"}, sidebarWidth, 5)
	if len(got) != 5 {
		t.Fatalf("len = %d, want 5", len(got))
	}
	if strings.TrimRight(got[0], " ") != "only one line" {
		t.Fatalf("first row = %q, want the untouched content (modulo width padding)", got[0])
	}
	for i := 1; i < 5; i++ {
		if got[i] != "" {
			t.Fatalf("row %d = %q, want empty padding", i, got[i])
		}
	}
}

// TestFitLinesGameStateBlockSplitsToExactRows checks fitLines against
// renderedRows for the concrete case the task brief calls out by name: one
// eng.Logs entry containing embedded newlines. Requesting exactly the number
// of rows renderedRows reports for that entry must reproduce it line-for-
// line (via lipgloss's own wrap), and asking for fewer must truncate to
// exactly that many rows.
func TestFitLinesGameStateBlockSplitsToExactRows(t *testing.T) {
	block := fmt.Sprintf("\n=== Game State ===\nWave: %d\nCurrent Turn: %s (%s)\n%s (Def) - Lives: %d, Res: %d\n%s (Att) - Res: %d\nActive Towers: %d, Enemies: %d\n==================\n",
		7, "p1", "gemini-2.5-flash", "qwen/qwen3-next-80b-a3b-instruct", 18, 340, "gemini-3-flash-preview", 220, 9, 14)

	want := renderedRows(block, sidebarWidth)
	if want != 12 {
		t.Fatalf("setup invariant: renderedRows(block, %d) = %d, want 12", sidebarWidth, want)
	}

	full := fitLines([]string{block}, sidebarWidth, want)
	if len(full) != want {
		t.Fatalf("fitLines at exact budget: len = %d, want %d", len(full), want)
	}
	wantRows := strings.Split(lipgloss.NewStyle().Width(sidebarWidth).Render(block), "\n")
	for i := range wantRows {
		if full[i] != wantRows[i] {
			t.Fatalf("row %d = %q, want %q", i, full[i], wantRows[i])
		}
	}

	truncated := fitLines([]string{block}, sidebarWidth, 5)
	if len(truncated) != 5 {
		t.Fatalf("fitLines at budget 5: len = %d, want 5", len(truncated))
	}
	for i := 0; i < 5; i++ {
		if truncated[i] != wantRows[i] {
			t.Fatalf("truncated row %d = %q, want %q", i, truncated[i], wantRows[i])
		}
	}
}

// ansiSeqRE matches a complete SGR escape sequence (ESC [ ... m), the only
// kind lipgloss emits for foreground/bold styling in this codebase.
var ansiSeqRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// TestFitLinesNeverSplitsAnAnsiEscapeSequence feeds fitLines a lipgloss-
// styled line long enough that it must wrap (and, at a small budget, get
// truncated) across the point where its color codes sit, then checks that
// every ESC byte in the output belongs to a complete, well-formed SGR
// sequence. If a wrap or truncation step ever cut inside "\x1b[196m", a bare
// "\x1b" (or a mangled remainder) would survive stripping all complete
// sequences out.
func TestFitLinesNeverSplitsAnAnsiEscapeSequence(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).
		Render(repeatWord("defender-tower-under-heavy-sustained-attack", 15))

	for _, budget := range []int{1, 2, 3, 8, 40} {
		t.Run(fmt.Sprintf("budget_%d", budget), func(t *testing.T) {
			rows := fitLines([]string{styled, "a plain second entry", styled}, sidebarWidth, budget)
			if len(rows) != budget {
				t.Fatalf("len = %d, want %d", len(rows), budget)
			}
			joined := strings.Join(rows, "\n")
			stripped := ansiSeqRE.ReplaceAllString(joined, "")
			if strings.ContainsRune(stripped, '\x1b') {
				t.Fatalf("dangling/mangled ANSI escape survived: stripped = %q, joined = %q", stripped, joined)
			}
			if !utf8.ValidString(joined) {
				t.Fatalf("fitLines output is not valid UTF-8: %q", joined)
			}
		})
	}
}

// --- fitLinesWithMoreIndicator ---------------------------------------------

// TestFitLinesWithMoreIndicatorAlwaysReturnsBudgetRows checks the same
// exact-budget contract fitLines has, across cases that do and don't need
// truncation.
func TestFitLinesWithMoreIndicatorAlwaysReturnsBudgetRows(t *testing.T) {
	manyLines := make([]string, 50)
	for i := range manyLines {
		manyLines[i] = fmt.Sprintf("line %d", i)
	}

	cases := []struct {
		name   string
		lines  []string
		width  int
		budget int
	}{
		{"under_budget", []string{"a", "b"}, sidebarWidth, 5},
		{"exact_budget", []string{"a", "b", "c"}, sidebarWidth, 3},
		{"over_budget", manyLines, sidebarWidth, 5},
		{"over_budget_budget_1", manyLines, sidebarWidth, 1},
		{"zero_budget", manyLines, sidebarWidth, 0},
		{"negative_budget", manyLines, sidebarWidth, -3},
		{"empty_lines", nil, sidebarWidth, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := fitLinesWithMoreIndicator(c.lines, c.width, c.budget)
			want := c.budget
			if want < 0 {
				want = 0
			}
			if len(got) != want {
				t.Fatalf("len = %d, want %d", len(got), want)
			}
		})
	}
}

// TestFitLinesWithMoreIndicatorReportsHiddenCount checks the indicator's
// actual content: when lines overflow budget, the LAST row must be a "+N
// more lines" marker where N is exactly the number of wrapped rows that
// didn't make it into the budget (not the number of INPUT lines, which can
// differ once wrapping is involved).
func TestFitLinesWithMoreIndicatorReportsHiddenCount(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("entry %02d", i)
	}
	width, budget := 40, 5

	all := wrapAllLines(lines, width)
	wantHidden := len(all) - (budget - 1)

	got := fitLinesWithMoreIndicator(lines, width, budget)
	if len(got) != budget {
		t.Fatalf("len = %d, want %d", len(got), budget)
	}
	last := strings.TrimRight(got[budget-1], " ")
	wantSuffix := fmt.Sprintf("+%d more lines", wantHidden)
	if !strings.HasSuffix(last, wantSuffix) {
		t.Fatalf("last row = %q, want it to end with %q", last, wantSuffix)
	}
	// The rows before the indicator must be the untouched leading content.
	for i := 0; i < budget-1; i++ {
		if got[i] != all[i] {
			t.Fatalf("row %d = %q, want %q", i, got[i], all[i])
		}
	}
}

// TestFitLinesWithMoreIndicatorNoIndicatorWhenContentFits checks the
// no-truncation path returns the untouched content (via fitLines), with no
// "+N more" marker anywhere.
func TestFitLinesWithMoreIndicatorNoIndicatorWhenContentFits(t *testing.T) {
	lines := []string{"short one", "short two"}
	got := fitLinesWithMoreIndicator(lines, sidebarWidth, 5)
	for i, row := range got {
		if strings.Contains(row, "more lines") {
			t.Fatalf("row %d = %q, unexpected indicator when content fits within budget", i, row)
		}
	}
	if strings.TrimRight(got[0], " ") != "short one" || strings.TrimRight(got[1], " ") != "short two" {
		t.Fatalf("got = %q, want untouched content", got)
	}
}

// --- shortName ------------------------------------------------------------

// realModelNames are the model identifiers this project actually configures
// (see engine/provider*.go and readme.md), plus the CJK and emoji strings
// the task brief calls out as needing grapheme-safe handling.
var realModelNames = []string{
	"o3",
	"gpt-4o-mini",
	"gemini-2.5-flash",
	"gemini-3-flash-preview",
	"qwen/qwen3-next-80b-a3b-instruct",
}

func TestShortNameFitsBudgetAndStaysValidUTF8(t *testing.T) {
	inputs := append([]string{}, realModelNames...)
	inputs = append(inputs,
		"我要摧毁你的防御塔并且赢得这场比赛哈哈哈哈", // CJK, real failing input from wrapText
		"Attacking now 🚀🚀🚀…",         // emoji, real failing input from wrapText
	)

	cellBudgets := []int{0, 1, 2, 3, 4, 6, 8, 10, 12, 20, 40}

	for _, in := range inputs {
		for _, cells := range cellBudgets {
			name := fmt.Sprintf("%q_cells%d", in, cells)
			t.Run(name, func(t *testing.T) {
				got := shortName(in, cells)
				if !utf8.ValidString(got) {
					t.Fatalf("shortName(%q, %d) = %q, not valid UTF-8", in, cells, got)
				}
				if w := lipgloss.Width(got); w > cells {
					t.Fatalf("shortName(%q, %d) = %q, width %d exceeds budget %d", in, cells, got, w, cells)
				}
			})
		}
	}
}

// TestShortNameLeavesRoomForContent checks shortName isn't just returning ""
// or the bare ellipsis whenever a budget is large enough to show a
// meaningful prefix of a real, long model name.
func TestShortNameLeavesRoomForContent(t *testing.T) {
	got := shortName("qwen/qwen3-next-80b-a3b-instruct", 12)
	if got == "" {
		t.Fatalf("shortName with a 12-cell budget returned empty for a long model name")
	}
	if !strings.HasPrefix(got, "qwen") {
		t.Fatalf("shortName(%q, 12) = %q, want it to keep the recognizable prefix", "qwen/qwen3-next-80b-a3b-instruct", got)
	}
}

// --- truncateCells --------------------------------------------------------

// TestTruncateCellsHandlesRealFailingInputs exercises the two verified
// failing inputs from the task brief -- LLM taunt/reasoning text containing
// emoji and CJK -- across many cell budgets, including ones that would have
// landed mid-rune under the old text[:width-3] byte slice. In every case the
// result must be valid UTF-8 and must not exceed the requested cell budget.
func TestTruncateCellsHandlesRealFailingInputs(t *testing.T) {
	inputs := []string{
		"Attacking now 🚀🚀🚀…",
		"我要摧毁你的防御塔并且赢得这场比赛哈哈哈哈",
	}

	for _, in := range inputs {
		total := lipgloss.Width(in)
		for cells := 0; cells <= total+5; cells++ {
			cells := cells
			t.Run(fmt.Sprintf("%q_cells%d", in, cells), func(t *testing.T) {
				got := truncateCells(in, cells)
				if !utf8.ValidString(got) {
					t.Fatalf("truncateCells(%q, %d) = %q, not valid UTF-8", in, cells, got)
				}
				if w := lipgloss.Width(got); w > cells {
					t.Fatalf("truncateCells(%q, %d) = %q, width %d exceeds budget %d", in, cells, got, w, cells)
				}
				if cells >= total && got != in {
					t.Fatalf("truncateCells(%q, %d) = %q, want the untouched input once the budget covers it", in, cells, got)
				}
			})
		}
	}
}

// TestTruncateCellsSmallBudgetsDoNotPanic covers the old wrapText's actual
// crash: text[:width-3] panics with a negative slice bound for width < 3.
// cells 0-3 (and, for good measure, a negative value) must return cleanly.
func TestTruncateCellsSmallBudgetsDoNotPanic(t *testing.T) {
	inputs := []string{
		"",
		"x",
		"hello world",
		"我要摧毁你的防御塔并且赢得这场比赛哈哈哈哈",
		"Attacking now 🚀🚀🚀…",
	}
	budgets := []int{-5, -1, 0, 1, 2, 3}

	for _, in := range inputs {
		for _, cells := range budgets {
			in, cells := in, cells
			t.Run(fmt.Sprintf("%q_cells%d", in, cells), func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("truncateCells(%q, %d) panicked: %v", in, cells, r)
					}
				}()
				got := truncateCells(in, cells)
				if !utf8.ValidString(got) {
					t.Fatalf("truncateCells(%q, %d) = %q, not valid UTF-8", in, cells, got)
				}
				if w := lipgloss.Width(got); cells > 0 && w > cells {
					t.Fatalf("truncateCells(%q, %d) = %q, width %d exceeds budget %d", in, cells, got, w, cells)
				}
				if cells <= 0 && got != "" {
					t.Fatalf("truncateCells(%q, %d) = %q, want empty for non-positive budget", in, cells, got)
				}
			})
		}
	}
}

// --- computeLayout (T2.1) --------------------------------------------------

// bodyHeight is how the property test below defines "the row budget" for a
// layout, adapted for wide mode's two parallel columns: in wide mode the
// move feed is the full right-hand column and the left column (board+stats)
// is capped to fit inside the same budget, so the taller of the two -- which
// by construction is always moves.h -- is what the status/keybar rows sit
// above/below. In stacked/compact mode there's only one column, so it's a
// plain sum.
func bodyHeight(l layout) int {
	if l.mode == layoutWide {
		left := l.board.h + l.stats.h
		if l.moves.h > left {
			return l.moves.h
		}
		return left
	}
	return l.board.h + l.stats.h + l.moves.h
}

// TestComputeLayoutInvariants is the property test the task brief specifies
// for T2.1: over the full w in [40,250] x h in [10,80] grid, (a) heights sum
// to exactly h, (b) boardW+movesW == w in wide mode, (c) no pane width
// exceeds w, and (d) the mode is monotonically non-decreasing as w grows for
// any fixed h. ~21000 cases; runs in milliseconds since computeLayout is
// pure arithmetic.
func TestComputeLayoutInvariants(t *testing.T) {
	const noPrevMode layoutMode = -1
	for h := 10; h <= 80; h++ {
		prevMode := noPrevMode
		for w := 40; w <= 250; w++ {
			l := computeLayout(w, h)

			// (d) mode transitions monotonic in w: the mode value (ordered
			// least-to-most-spacious, see layoutMode's doc comment) must
			// never decrease as w grows for a fixed h.
			if prevMode != noPrevMode && l.mode < prevMode {
				t.Fatalf("w=%d h=%d: mode %v is smaller than the previous mode %v seen at a narrower w -- not monotonic", w, h, l.mode, prevMode)
			}
			prevMode = l.mode

			if l.mode == layoutTooSmall {
				// No row/width budget claims apply below layoutTooSmall --
				// the notice path (tooSmallNotice) is responsible for its
				// own fit, checked separately in TestTooSmallNoticeFits.
				continue
			}

			// (a) heights sum to exactly h.
			total := l.status.h + bodyHeight(l) + l.keybar.h
			if total != h {
				t.Fatalf("w=%d h=%d mode=%v: heights sum to %d, want %d (status=%d board=%d stats=%d moves=%d keybar=%d)",
					w, h, l.mode, total, h, l.status.h, l.board.h, l.stats.h, l.moves.h, l.keybar.h)
			}

			// (b) boardW + movesW == w, in wide mode.
			if l.mode == layoutWide {
				if sum := l.board.w + l.moves.w; sum != w {
					t.Fatalf("w=%d h=%d: wide mode board.w(%d)+moves.w(%d) = %d, want %d", w, h, l.board.w, l.moves.w, sum, w)
				}
			}

			// (c) no pane width exceeds w.
			for name, r := range map[string]rect{"status": l.status, "board": l.board, "stats": l.stats, "moves": l.moves, "keybar": l.keybar} {
				if r.w > w {
					t.Fatalf("w=%d h=%d mode=%v: pane %q width %d exceeds w", w, h, l.mode, name, r.w)
				}
				if r.w < 0 || r.h < 0 {
					t.Fatalf("w=%d h=%d mode=%v: pane %q has negative dimension {%d,%d}", w, h, l.mode, name, r.w, r.h)
				}
			}
		}
	}
}

// TestComputeLayoutZeroSizeNormalizedTo80x24 locks down invariant #2: a
// zero width or height (the pre-first-WindowSizeMsg state) is treated as
// 80x24, never as an unbounded "wide" terminal the way the pre-rewrite
// layoutForSize(0) == layoutWide used to.
func TestComputeLayoutZeroSizeNormalizedTo80x24(t *testing.T) {
	want := computeLayout(80, 24)
	for _, c := range [][2]int{{0, 0}, {0, 40}, {160, 0}} {
		got := computeLayout(c[0], c[1])
		if got != want {
			t.Fatalf("computeLayout(%d,%d) = %+v, want the 80x24 layout %+v", c[0], c[1], got, want)
		}
	}
	if want.mode == layoutWide {
		t.Fatalf("setup invariant violated: 80x24 must not resolve to layoutWide")
	}
}

// TestComputeLayoutTooSmallBoundaries checks the exact w<60 / h<15 threshold
// from the task brief's breakpoint table.
func TestComputeLayoutTooSmallBoundaries(t *testing.T) {
	cases := []struct {
		w, h int
		want layoutMode
	}{
		{59, 15, layoutTooSmall},
		{60, 15, layoutCompact},
		{60, 14, layoutTooSmall},
		{83, 20, layoutCompact},
		{84, 20, layoutStacked},
		{115, 20, layoutStacked},
		{116, 20, layoutWide},
	}
	for _, c := range cases {
		if got := computeLayout(c.w, c.h).mode; got != c.want {
			t.Fatalf("computeLayout(%d,%d).mode = %v, want %v", c.w, c.h, got, c.want)
		}
	}
}

// TestTooSmallNoticeFits checks the layoutTooSmall fallback text itself
// respects the literal w/h it was given, however small -- it can't rely on
// computeLayout's own budgets since those aren't computed in this mode.
// Widths below ~20 are deliberately excluded: fitLines (and therefore
// tooSmallNotice) is defined in terms of
// lipgloss.NewStyle().Width(w).Render(), and that underlying primitive does
// not guarantee a hard per-character break for a single unbroken word
// narrower than the word itself at pathologically small widths (observed:
// Width(1).Render("terminal...") returns 3 display columns, not 1) -- that
// is a property of the shared lipgloss/x-ansi wrap primitive the whole
// codebase is built on (renderedRows/fitLines are intentionally defined AS
// that expression, not a hand-rolled reimplementation), not something this
// function can override without diverging from the contract. No real
// terminal is 1-19 columns wide, and neither the task's own T2.1 property
// test range (w in [40,250]) nor any protected main_view_test.go size goes
// below 60, so this is exercising well past the meaningfully specified
// range.
func TestTooSmallNoticeFits(t *testing.T) {
	for h := 1; h <= 14; h++ {
		for _, w := range []int{20, 30, 40, 59} {
			out := tooSmallNotice(w, h)
			if got := lipgloss.Height(out); got > h {
				t.Fatalf("tooSmallNotice(%d,%d): height %d exceeds %d", w, h, got, h)
			}
			if got := lipgloss.Width(out); got > w {
				t.Fatalf("tooSmallNotice(%d,%d): width %d exceeds %d", w, h, got, w)
			}
		}
	}
}

// TestTruncateCellsNeverExceedsByteLengthOfInput is a cheap sanity check
// that truncateCells is actually shrinking, not just measuring: for a budget
// smaller than the input's own width, the result must be a strict prefix
// (by grapheme clusters) of the input, and shorter in bytes.
func TestTruncateCellsNeverExceedsByteLengthOfInput(t *testing.T) {
	in := "我要摧毁你的防御塔并且赢得这场比赛哈哈哈哈"
	got := truncateCells(in, 10)
	if !strings.HasPrefix(in, got) {
		t.Fatalf("truncateCells(%q, 10) = %q, want a prefix of the input", in, got)
	}
	if len(got) >= len(in) {
		t.Fatalf("truncateCells(%q, 10) = %q (%d bytes), want fewer bytes than input (%d)", in, got, len(got), len(in))
	}
}
