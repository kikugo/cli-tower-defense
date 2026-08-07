package main

import (
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// withColorProfile runs fn with lipgloss's global colour profile forced to
// p, restoring it afterwards.
//
// Forcing it is not optional decoration on these tests -- it is the only
// thing that makes them mean anything. lipgloss picks its profile from the
// output device, and a test binary has no TTY, so the default profile is
// Ascii and every Render call returns its input unchanged. A colour test
// that skipped this would pass against a renderer that emitted no colour at
// all. (Same trap as reverseX in render_gameover_v2_test.go, from the other
// direction.)
//
// These tests therefore cannot run in parallel with each other, and none of
// them calls t.Parallel().
func withColorProfile(t *testing.T, p termenv.Profile, fn func()) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(p)
	defer lipgloss.SetColorProfile(prev)
	fn()
}

// stripANSI removes every escape sequence from s, leaving the printable
// characters. Written here rather than reached for from a library because
// the only thing it is used for is comparing a styled render against an
// unstyled one, and a hand-rolled 12-line scanner is easier to trust for
// that than a regexp whose behaviour on a partial sequence is unobvious.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			j := i + 1
			for j < len(s) && !((s[j] >= 0x40 && s[j] <= 0x5a) || (s[j] >= 0x61 && s[j] <= 0x7a)) {
				j++
			}
			i = j + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// --- the ASCII fold ------------------------------------------------------

// TestASCIIFoldLeavesNoBoxOrBlockCharacters is the fold's headline contract:
// after folding, nothing the v2 renderers draw as chrome is left non-ASCII.
// It runs against real rendered frames rather than hand-written strings, so
// a renderer that starts emitting a new box character is caught here rather
// than on someone's terminal.
func TestASCIIFoldLeavesNoBoxOrBlockCharacters(t *testing.T) {
	g := buildSeededGameV2(t, true)

	frames := map[string][]string{
		"board":    renderFramedBoardV2(g, rect{w: boardMaxW, h: boardMaxH}, 0, "ENGINE HELPED 2x", "r range", false),
		"legend":   renderLegendV2(g, rect{w: 74, h: 8}),
		"feed":     RenderFeedV2(g.ReplayEvents, 74, 20),
		"cards":    RenderCardsV2(rect{w: boardMaxW, h: 16}, sampleCardsData()),
		"timeline": RenderTimelineV2(rect{w: boardMaxW, h: 13}, sampleTimelineData()),
		"gameover": RenderGameOverCardV2(sampleGameOverData(), gameOverCardMinW),
		"header": RenderHeaderV2(computeLayoutV2(160, 50), MatchHeaderData{
			Defender: PlayerHeaderData{ModelName: "o3", Lives: 7, MaxLives: 10},
			Attacker: PlayerHeaderData{ModelName: "gpt-4o-mini"},
			Wave:     12, MaxWave: 30, Tick: 117, MaxTick: 400,
		}, TrustState{AssistKnown: true, AssistsEnabled: true, AssistCount: 2, ProvenanceKnown: true}),
	}

	for name, rows := range frames {
		for i, row := range asciiFoldRows(rows) {
			for _, r := range row {
				if r > unicode.MaxASCII {
					t.Fatalf("%s row %d still contains %q (U+%04X) after the fold: %q",
						name, i, string(r), r, row)
				}
			}
		}
	}
}

// TestASCIIFoldPreservesWidth is what makes it safe to fold AFTER layout has
// committed to column positions: every substitution is one display column
// for one display column, so no row changes width.
func TestASCIIFoldPreservesWidth(t *testing.T) {
	for from, to := range asciiFoldTable {
		if w := frameDisplayWidth(string(from)); w != 1 {
			t.Fatalf("fold source %q is %d display columns, want 1", string(from), w)
		}
		if to > unicode.MaxASCII {
			t.Fatalf("fold target for %q is not ASCII: %q", string(from), string(to))
		}
	}

	g := buildSeededGameV2(t, true)
	rows := renderLegendV2(g, rect{w: 74, h: 8})
	for i, row := range rows {
		before := lipgloss.Width(row)
		after := lipgloss.Width(asciiFold(row))
		if before != after {
			t.Fatalf("row %d changed width across the fold: %d -> %d", i, before, after)
		}
	}
}

// TestASCIIFoldKeepsANSIIntact checks the fold does not disturb escape
// sequences. It contains no characters in the table, so this should be
// true by construction -- but "by construction" is exactly the kind of
// claim that stops being true when someone adds a row to the table.
func TestASCIIFoldKeepsANSIIntact(t *testing.T) {
	withColorProfile(t, termenv.ANSI256, func() {
		styled := styleAttackerV2.Render("┌──┐")
		if !strings.Contains(styled, "\x1b[") {
			t.Fatal("fixture carries no ANSI even at ANSI256 -- the test would prove nothing")
		}
		folded := asciiFold(styled)
		if !strings.Contains(folded, "+--+") {
			t.Fatalf("box characters were not folded: %q", folded)
		}
		if lipgloss.Width(folded) != lipgloss.Width(styled) {
			t.Fatalf("fold changed the printable width of a styled string: %q", folded)
		}
		// The escape sequences must be byte-identical, since none of their
		// characters is in the table.
		if strings.Count(folded, "\x1b[") != strings.Count(styled, "\x1b[") {
			t.Fatalf("fold altered the escape sequences: %q", folded)
		}
	})
}

// TestASCIIFoldIsIdempotent: folding already-folded output must be a no-op,
// so it is safe to apply defensively.
func TestASCIIFoldIsIdempotent(t *testing.T) {
	in := "┌─ SPAWN >> ─┐ ████░░░░ … ╰╯"
	once := asciiFold(in)
	if twice := asciiFold(once); twice != once {
		t.Fatalf("not idempotent: %q -> %q", once, twice)
	}
}

// --- the palette ---------------------------------------------------------

// TestPaletteIsDecorationOnly is the accessibility contract this redesign is
// built on: strip every colour and the UI must still say the same things.
// It renders the board at full colour and at no colour and checks the
// PRINTABLE characters are identical -- i.e. colour adds no glyph, removes
// no glyph, and changes no glyph.
func TestPaletteIsDecorationOnly(t *testing.T) {
	g := buildSeededGameV2(t, true)

	var coloured, plain []string
	withColorProfile(t, termenv.ANSI256, func() {
		coloured = renderFramedBoardV2(g, rect{w: boardMaxW, h: boardMaxH}, 0, "", "", false)
	})
	withColorProfile(t, termenv.Ascii, func() {
		plain = renderFramedBoardV2(g, rect{w: boardMaxW, h: boardMaxH}, 0, "", "", false)
	})

	if len(coloured) != len(plain) {
		t.Fatalf("row counts differ: %d vs %d", len(coloured), len(plain))
	}
	sawColour := false
	for i := range coloured {
		if strings.Contains(coloured[i], "\x1b[") {
			sawColour = true
		}
		if got, want := stripANSI(coloured[i]), plain[i]; got != want {
			t.Fatalf("row %d differs once colour is stripped:\n coloured: %q\n plain:    %q", i, got, want)
		}
	}
	if !sawColour {
		t.Fatal("no row carried colour at ANSI256 -- the comparison proved nothing")
	}
}

// TestBoardRowsKeepExactWidthWithColour is the invariant most at risk from
// Phase 3: adding escape sequences must not change any row's DISPLAY width.
// Measured with lipgloss.Width, because frameDisplayWidth counts escape
// bytes as columns and cannot be used on a styled row.
func TestBoardRowsKeepExactWidthWithColour(t *testing.T) {
	g := buildSeededGameV2(t, true)
	withColorProfile(t, termenv.ANSI256, func() {
		for _, size := range []rect{{w: boardMaxW, h: boardMaxH}, {w: 60, h: 12}, {w: 100, h: 20}} {
			for i, row := range renderFramedBoardV2(g, size, 0, "ENGINE HELPED 2x", "r range", false) {
				if w := lipgloss.Width(row); w != size.w {
					t.Fatalf("%dx%d row %d is %d columns, want %d: %q", size.w, size.h, i, w, size.w, row)
				}
			}
			for i, row := range RenderFeedV2(g.ReplayEvents, size.w, size.h) {
				if w := lipgloss.Width(row); w != size.w {
					t.Fatalf("feed %dx%d row %d is %d columns, want %d: %q", size.w, size.h, i, w, size.w, row)
				}
			}
		}
	})
}

// TestGlyphStyleMatchesOwnership checks the palette agrees with the glyph
// vocabulary: every tower glyph gets the defender's colour, every enemy
// glyph the attacker's, and terrain the dim one. This is the row the wide
// legend promises a reader ("DEFENDER blue  ATTACKER orange  TERRAIN grey"),
// so the two must not drift.
func TestGlyphStyleMatchesOwnership(t *testing.T) {
	for typ, g := range towerGlyphV2 {
		if glyphStyleV2(g).String() != styleDefenderV2.String() {
			t.Fatalf("tower %q (%q) is not styled as the defender", typ, string(g))
		}
	}
	for typ, g := range enemyGlyphV2 {
		if glyphStyleV2(g).String() != styleAttackerV2.String() {
			t.Fatalf("enemy %q (%q) is not styled as the attacker", typ, string(g))
		}
	}
	for _, g := range []rune{pathGlyphV2, flowGlyphV2, wallGlyphV2, slowZoneGlyphV2} {
		if glyphStyleV2(g).String() != styleTerrainV2.String() {
			t.Fatalf("terrain glyph %q is not styled as terrain", string(g))
		}
	}
}

// TestBreachSurvivesMonochrome pins what actually happens to the breach
// marker as the terminal's capabilities fall away, which is NOT what this
// codebase claimed before the test was written.
//
// The claim was "reverse video is an SGR attribute, not a colour, so the
// breach alert survives monochrome". Half right. At profile ANSI -- a
// terminal that emits escapes but has only 16 colours, the meaningful
// "monochrome" case -- the reverse attribute does survive. At profile Ascii
// termenv drops EVERY escape sequence, attributes included, so the marker
// renders as a bare "X".
//
// That is still correct behaviour, for a reason worth stating: profile
// Ascii means the caller asked for plain text (NO_COLOR, TERM=dumb, a pipe),
// and 'X' is a glyph used for nothing else in the vocabulary, so the alert
// is carried by the character itself with no attribute needed. The design
// holds; the justification in the comment was wrong.
func TestBreachSurvivesMonochrome(t *testing.T) {
	withColorProfile(t, termenv.ANSI, func() {
		// lipgloss merges the attribute and the colour into ONE SGR
		// sequence at this profile ("\x1b[7;91m"), so the check is on the
		// sequence's leading parameter, not on a standalone "\x1b[7m".
		got := styleBreachV2.Render("X")
		if !strings.HasPrefix(got, "\x1b[7") {
			t.Fatalf("breach marker lost its reverse video at profile ANSI: %q", got)
		}
	})

	withColorProfile(t, termenv.Ascii, func() {
		if got := styleBreachV2.Render("X"); got != "X" {
			t.Fatalf("profile Ascii emitted %q, want a bare X", got)
		}
	})

	// The glyph must be unique for the plain-text case to carry the alert.
	for typ, g := range towerGlyphV2 {
		if g == breachGlyphV2 {
			t.Fatalf("tower %q shares the breach glyph", typ)
		}
	}
	for typ, g := range enemyGlyphV2 {
		if g == breachGlyphV2 {
			t.Fatalf("enemy %q shares the breach glyph", typ)
		}
	}
	for _, g := range []rune{pathGlyphV2, flowGlyphV2, wallGlyphV2, slowZoneGlyphV2} {
		if g == breachGlyphV2 {
			t.Fatalf("terrain glyph %q shares the breach glyph", string(g))
		}
	}
}

// TestPaletteDegradesTo16Colours checks the 16-colour fallback the design
// asks for actually happens. It is termenv that performs the mapping, not
// this codebase, so what is being verified here is that the palette is
// declared in a form termenv CAN degrade -- an out-of-range index maps down
// rather than being emitted verbatim to a terminal that cannot show it.
func TestPaletteDegradesTo16Colours(t *testing.T) {
	var at256, at16 string
	withColorProfile(t, termenv.ANSI256, func() { at256 = styleAttackerV2.Render("o") })
	withColorProfile(t, termenv.ANSI, func() { at16 = styleAttackerV2.Render("o") })

	if at256 == at16 {
		t.Fatalf("ANSI256 and ANSI rendered identically (%q) -- no degradation happened", at256)
	}
	if !strings.Contains(at16, "\x1b[") {
		t.Fatalf("16-colour rendering carries no colour at all: %q", at16)
	}
	// The 256-colour form uses the extended "38;5;N" selector; the 16-colour
	// form must not, or it is not really a 16-colour sequence.
	if strings.Contains(at16, "38;5;") {
		t.Fatalf("16-colour rendering still uses a 256-colour selector: %q", at16)
	}
	if !strings.Contains(at256, "38;5;") {
		t.Fatalf("256-colour rendering does not use the extended selector: %q", at256)
	}
}

// --- the tick horizon -----------------------------------------------------

// TestFmtTickNeverInventsACap covers both directions of the fix for a live
// recording that rendered "tick 445/400" with a full progress bar -- a
// match that looked finished while it was still running. The interactive
// loop has no tick cap (see model.tickHorizon), so a zero max must say so
// rather than divide by it.
func TestFmtTickNeverInventsACap(t *testing.T) {
	if got := fmtTick(117, 400); got != "tick 117/400" {
		t.Fatalf("capped run: got %q", got)
	}
	for _, max := range []int64{0, -1} {
		got := fmtTick(445, max)
		if strings.Contains(got, "/") {
			t.Fatalf("uncapped run (max=%d) rendered a ratio: %q", max, got)
		}
		if !strings.Contains(got, "445") {
			t.Fatalf("uncapped run (max=%d) lost the tick count: %q", max, got)
		}
	}
}

// TestTickHorizonIsZeroInteractively pins the decision itself: the live view
// reports no tick cap, because the interactive Update loop enforces none.
// If a cap is ever enforced there, this test is the reminder that the view
// has to start reporting it.
func TestTickHorizonIsZeroInteractively(t *testing.T) {
	m := model{maxTicks: 400}
	if got := m.tickHorizon(); got != 0 {
		t.Fatalf("tickHorizon = %d, want 0 -- see the doc comment before changing this", got)
	}
}

// TestFillBarNeverOverfillsAnUncappedRun: fillBar's max<=0 branch is what
// stops an uncapped run from drawing a full horizon bar.
func TestFillBarNeverOverfillsAnUncappedRun(t *testing.T) {
	bar := fillBar(445, 0, 10)
	if strings.Contains(bar, "█") {
		t.Fatalf("uncapped horizon drew a filled bar: %q", bar)
	}
	if lipgloss.Width(bar) != 10 {
		t.Fatalf("bar width %d, want 10", lipgloss.Width(bar))
	}
}
