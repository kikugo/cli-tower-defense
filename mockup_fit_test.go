package main

// This file provides the executable fit-invariant harness for the Phase 2
// terminal UI redesign. The mockups in Claude_Design/mockups/*.txt (untracked
// scratch, not part of the build) were drawn to exact character grids and
// have been copied byte-for-byte into testdata/mockups/ as tracked fixtures.
// Those fixtures are the contract: TestMockupFixturesFit below asserts every
// one of them satisfies the fit invariant this whole layout rewrite exists to
// enforce -- content must never overflow the terminal, because Bubble Tea
// silently discards lines from the top when it does, which is exactly how
// the game board used to disappear.
//
// The three checks below (frameDisplayWidth, checkFits, checkNoOrphanDividers)
// are written as reusable, exported-within-package helpers rather than
// inline test logic specifically so that later phases -- once real rendering
// code exists -- can point them at freshly rendered frames (e.g. the output
// of model.View()) and not just at these static fixtures.

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// --- East-Asian-Width-aware display width -----------------------------

// eastAsianWideRanges is a small explicit range table of the Unicode
// codepoints whose East_Asian_Width property is Wide (W) or Fullwidth (F),
// per Unicode's EastAsianWidth.txt. Anything in these ranges occupies two
// terminal columns; everything else (including EAW Ambiguous, Narrow,
// Halfwidth and Neutral) occupies one. golang.org/x/text is not a available
// dependency for this project, so this table is hand-written rather than
// generated -- it covers the commonly-occurring Wide/Fullwidth blocks
// (Hangul Jamo, CJK symbols/punctuation, Hiragana/Katakana, CJK Unified
// Ideographs and extensions, Hangul Syllables, CJK Compatibility, Fullwidth
// Forms, and the Wide-classified pictograph/emoji blocks) and is not a
// byte-for-byte transcription of the full Unicode data file.
//
// Ranges are given as [lo, hi] inclusive, sorted ascending by lo, and
// searched with a binary search below.
//
// IMPORTANT design note: this project's box-drawing characters (U+2500-
// U+257F) and block-element characters (U+2580-U+259F) are EAW *Ambiguous*,
// not Wide -- Ambiguous is deliberately EXCLUDED from this table (and so
// counts as 1 column), because the mockups' layout assumes those glyphs
// render narrow in the terminals this game targets. That assumption is the
// documented basis of the whole design, not an oversight.
var eastAsianWideRanges = [][2]rune{
	{0x1100, 0x115F},   // Hangul Jamo
	{0x231A, 0x231B},   // Watch, Hourglass
	{0x2329, 0x232A},   // Angle brackets
	{0x23E9, 0x23EC},   // Black/white right/left-pointing double triangle
	{0x23F0, 0x23F0},   // Alarm clock
	{0x23F3, 0x23F3},   // Hourglass with flowing sand
	{0x25FD, 0x25FE},   // White/black medium small square
	{0x2614, 0x2615},   // Umbrella with rain drops, hot beverage
	{0x2648, 0x2653},   // Zodiac symbols
	{0x267F, 0x267F},   // Wheelchair symbol
	{0x2693, 0x2693},   // Anchor
	{0x26A1, 0x26A1},   // High voltage sign
	{0x26AA, 0x26AB},   // Medium white/black circle
	{0x26BD, 0x26BE},   // Soccer ball, baseball
	{0x26C4, 0x26C5},   // Snowman, sun behind cloud
	{0x26CE, 0x26CE},   // Ophiuchus
	{0x26D4, 0x26D4},   // No entry
	{0x26EA, 0x26EA},   // Church
	{0x26F2, 0x26F3},   // Fountain, flag in hole
	{0x26F5, 0x26F5},   // Sailboat
	{0x26FA, 0x26FA},   // Tent
	{0x26FD, 0x26FD},   // Fuel pump
	{0x2705, 0x2705},   // White heavy check mark
	{0x270A, 0x270B},   // Raised fist, raised hand
	{0x2728, 0x2728},   // Sparkles
	{0x274C, 0x274C},   // Cross mark
	{0x274E, 0x274E},   // Negative squared cross mark
	{0x2753, 0x2755},   // Question/exclamation marks
	{0x2757, 0x2757},   // Heavy exclamation mark
	{0x2795, 0x2797},   // Heavy plus/minus/division sign
	{0x27B0, 0x27B0},   // Curly loop
	{0x27BF, 0x27BF},   // Double curly loop
	{0x2B1B, 0x2B1C},   // Black/white large square
	{0x2B50, 0x2B50},   // White medium star
	{0x2B55, 0x2B55},   // Heavy large circle
	{0x2E80, 0x2FDF},   // CJK Radicals Supplement, Kangxi Radicals
	{0x2FF0, 0x303E},   // Ideographic Description Chars, CJK Symbols and Punctuation
	{0x3041, 0x33FF},   // Hiragana .. CJK Compatibility
	{0x3400, 0x4DBF},   // CJK Unified Ideographs Extension A
	{0x4E00, 0x9FFF},   // CJK Unified Ideographs
	{0xA000, 0xA4CF},   // Yi Syllables, Yi Radicals
	{0xAC00, 0xD7A3},   // Hangul Syllables
	{0xF900, 0xFAFF},   // CJK Compatibility Ideographs
	{0xFE30, 0xFE4F},   // CJK Compatibility Forms
	{0xFF00, 0xFF60},   // Fullwidth Forms
	{0xFFE0, 0xFFE6},   // Fullwidth Signs
	{0x1F300, 0x1F64F}, // Misc Symbols and Pictographs, Emoticons (Wide-classified emoji)
	{0x1F680, 0x1F6FF}, // Transport and Map Symbols
	{0x20000, 0x2FFFD}, // CJK Unified Ideographs Extension B and beyond (Plane 2)
	{0x30000, 0x3FFFD}, // Plane 3 CJK ideographs
}

// isEastAsianWide reports whether r's East_Asian_Width property is Wide or
// Fullwidth, per eastAsianWideRanges above.
func isEastAsianWide(r rune) bool {
	lo, hi := 0, len(eastAsianWideRanges)
	for lo < hi {
		mid := (lo + hi) / 2
		rg := eastAsianWideRanges[mid]
		switch {
		case r < rg[0]:
			hi = mid
		case r > rg[1]:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}

// frameDisplayWidth reports the display width of line in terminal columns:
// East-Asian-Width Wide/Fullwidth runes count as 2, combining marks
// (Unicode categories Mn "nonspacing mark" and Me "enclosing mark") count as
// 0 since they combine with the preceding column rather than occupying their
// own, and everything else -- including EAW Ambiguous, Narrow, Halfwidth and
// Neutral -- counts as 1.
//
// This project's mockups deliberately use box-drawing (U+2500-257F) and
// block-element (U+2580-259F) characters, which are EAW Ambiguous. The
// design assumes those render narrow (1 column) in the terminals this game
// targets, so Ambiguous is counted as 1 here to match -- that is the
// documented basis of the layout, not an accident.
func frameDisplayWidth(line string) int {
	width := 0
	for _, r := range line {
		if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
			continue
		}
		if isEastAsianWide(r) {
			width += 2
		} else {
			width++
		}
	}
	return width
}

// --- fit-invariant checks -----------------------------------------------

// checkFits verifies that frame is exactly wantRows lines, each exactly
// wantCols display columns wide (per frameDisplayWidth) -- not merely
// within budget, but exactly, since the mockups are uniformly padded and
// any drift from that shows up as raggedness. A single optional trailing
// newline is tolerated (and does not count as an extra row); embedded blank
// lines are not.
func checkFits(frame string, wantCols, wantRows int) error {
	frame = strings.TrimSuffix(frame, "\n")
	lines := strings.Split(frame, "\n")

	if len(lines) != wantRows {
		return fmt.Errorf("checkFits: frame has %d rows, want %d", len(lines), wantRows)
	}

	for i, line := range lines {
		w := frameDisplayWidth(line)
		if w != wantCols {
			return fmt.Errorf("checkFits: row %d is %d display columns wide, want %d (row content: %q)", i, w, wantCols, line)
		}
	}

	return nil
}

// dividerRunes are the vertical-divider glyphs checkNoOrphanDividers looks
// for: the box-drawing light vertical (used by the primary Unicode
// mockups) and plain ASCII pipe (used by the ascii-fallback mockup).
var dividerRunes = map[rune]bool{
	'│': true,
	'|': true,
}

// checkNoOrphanDividers histograms the display-column index of every
// vertical-divider rune (│ or |) across every row of frame, and fails if any
// column is used by at least one row but fewer than three. A pane boundary
// that is jogging sideways on some rows produces exactly this signature: a
// row can be exactly the right total width (so checkFits sees nothing wrong)
// while its divider sits one or two columns off from where every other row's
// divider sits. A divider column that's genuinely part of the design is used
// by many rows (a whole box edge), so a column used by only one or two rows
// is the tell.
func checkNoOrphanDividers(frame string) error {
	frame = strings.TrimSuffix(frame, "\n")
	lines := strings.Split(frame, "\n")

	histogram := make(map[int]int)
	for _, line := range lines {
		colsInRow := make(map[int]bool)
		col := 0
		for _, r := range line {
			if dividerRunes[r] {
				colsInRow[col] = true
			}
			if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
				continue
			}
			if isEastAsianWide(r) {
				col += 2
			} else {
				col++
			}
		}
		for c := range colsInRow {
			histogram[c]++
		}
	}

	orphanCols := make([]int, 0)
	for c, n := range histogram {
		if n >= 1 && n < 3 {
			orphanCols = append(orphanCols, c)
		}
	}
	if len(orphanCols) == 0 {
		return nil
	}
	sort.Ints(orphanCols)
	c := orphanCols[0]
	return fmt.Errorf("checkNoOrphanDividers: display column %d has a divider rune in only %d row(s) (need >= 3, or 0) -- looks like a pane boundary jogged sideways", c, histogram[c])
}
