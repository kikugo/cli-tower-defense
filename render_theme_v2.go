package main

// Phase 3 of the redesign: the two things that are properties of the OUTPUT
// DEVICE rather than of any pane -- whether the terminal can draw the
// Unicode box/block characters, and how much colour it has.
//
// --- Why ASCII is a fold, not a mode threaded through every renderer ------
//
// The obvious design is a charset parameter on every render function, or a
// package-level `ascii bool` the renderers consult. Both were rejected.
//
// A parameter means changing the signature of every pane renderer (header,
// board, feed, cards, timeline, game-over) and every call site, to carry a
// value that only ever changes at process start. A package-level flag means
// the pure render functions -- whose whole contract is that output depends
// only on their arguments -- would quietly depend on global state, and the
// tests would become order-dependent.
//
// The third option is the one this file takes: render in Unicode always, and
// fold to ASCII once, at the output stage. That matches what the fallback
// actually is. "This terminal cannot draw ┌" is not a fact about the board
// pane; it is a fact about the terminal, and it applies uniformly to every
// character that reaches it -- including box characters that arrive inside a
// model's own reasoning text, which a per-pane charset would miss.
//
// The fold is width-preserving BY CONSTRUCTION: every rune in the table maps
// to exactly one ASCII byte, and both are one display column. That is the
// property that makes it safe to apply after layout has already committed to
// column positions, and TestASCIIFoldPreservesWidth checks it against the
// same measurement the mockup fixtures use.
//
// --- Colour ----------------------------------------------------------------
//
// The palette is declared as 256-colour indices. Degrading to 16 colours, or
// to none at all, is NOT implemented here: lipgloss/termenv already detect
// the terminal's profile and map an out-of-range colour down to the nearest
// available one, which is a better job than a hand-written fallback table
// would do (it knows the actual terminal, this file does not). What this
// file owns is the part termenv cannot infer: which SEMANTIC role each
// colour plays.
//
// Every colour here is decoration. Ownership is already carried by glyph
// class (glyphs_v2.go, rule 1) and every alert that matters is carried by a
// reverse-video attribute rather than a hue, so the UI is fully readable at
// profile Ascii with the palette contributing nothing. That is deliberate,
// and TestPaletteIsDecorationOnly is what holds it true: a colour scheme is
// allowed to help, never to be the only thing carrying a fact.

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// --- ASCII fold --------------------------------------------------------

// asciiFoldTable maps every non-ASCII character the v2 renderers emit to a
// single-byte ASCII stand-in. Derived from testdata/mockups/ascii-fallback-100x30.txt,
// which shows the intended result: box corners and junctions become '+',
// horizontals '-', verticals '|', and the bar blocks become '#' (filled) and
// '.' (empty).
//
// Both rounded (╭╮╰╯, the game-over card) and square (┌┐└┘, the board) corner
// sets are covered, along with the '┬' the trust band uses for its split and
// the '─' the titled rules are built from.
var asciiFoldTable = map[rune]rune{
	// Box drawing: corners and junctions.
	'┌': '+', '┐': '+', '└': '+', '┘': '+',
	'╭': '+', '╮': '+', '╰': '+', '╯': '+',
	'├': '+', '┤': '+', '┬': '+', '┴': '+', '┼': '+',
	// Box drawing: lines.
	'─': '-', '│': '|',
	// Block elements: the fill bars.
	'█': '#', '░': '.', '▓': '#', '▒': '#',
	// Typography.
	'…': '.',
	'•': '*',
	'·': '.',
	'→': '>',
	'←': '<',
	'♥': '*',
}

// asciiFold replaces every character in s that has an entry in
// asciiFoldTable, leaving everything else -- including ANSI escape
// sequences, which contain no such characters -- byte-for-byte unchanged.
//
// Characters NOT in the table are passed through rather than replaced with a
// '?'. A stray non-ASCII rune from a model's reasoning text is far better
// rendered as a possibly-wrong glyph than as a run of question marks, and
// the fold's job is the chrome this codebase emits, which is fully covered.
func asciiFold(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { _, ok := asciiFoldTable[r]; return ok }) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if sub, ok := asciiFoldTable[r]; ok {
			b.WriteRune(sub)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// asciiFoldRows applies asciiFold to a whole rendered frame. This is the
// single call the top-level view makes when --ascii is set; nothing below it
// knows the mode exists.
func asciiFoldRows(rows []string) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = asciiFold(row)
	}
	return out
}

// --- palette -----------------------------------------------------------

// paletteV2 names the semantic roles the redesign colours, as 256-colour
// indices. The four content roles are the ones the wide legend already
// promises a reader (testdata/mockups/160x50.txt line 43: "DEFENDER blue
// ATTACKER orange TERRAIN grey"), so this table and that legend row are
// describing the same thing and must not drift.
var (
	// colDefender is every defender-owned glyph and figure.
	colDefender = lipgloss.Color("39") // blue
	// colAttacker is every attacker-owned glyph and figure.
	colAttacker = lipgloss.Color("208") // orange
	// colTerrain is path, wall and slow-zone texture: deliberately dim, so
	// it never competes with the units standing on it (glyphs_v2.go rule 3).
	colTerrain = lipgloss.Color("240") // grey
	// colEngine is the engine-as-third-actor rows and the trust band.
	colEngine = lipgloss.Color("178") // amber
	// colAlert is a breach or a match-ending event.
	colAlert = lipgloss.Color("196") // red
	// colDim is secondary chrome: rules, borders, column headers.
	colDim = lipgloss.Color("244")
)

// v2 styles. Each is the single point where its role's colour is applied, so
// a Phase 3 palette change is a change to this block and nothing else.
var (
	styleDefenderV2 = lipgloss.NewStyle().Foreground(colDefender)
	styleAttackerV2 = lipgloss.NewStyle().Foreground(colAttacker)
	styleTerrainV2  = lipgloss.NewStyle().Foreground(colTerrain)
	styleEngineV2   = lipgloss.NewStyle().Foreground(colEngine)
	styleAlertV2    = lipgloss.NewStyle().Foreground(colAlert)
	styleDimV2      = lipgloss.NewStyle().Foreground(colDim)

	// Bars: the filled and empty halves are distinguished by COLOUR as well
	// as by glyph. '█' and '░' are visually close in several popular
	// monospace fonts -- close enough that a 100%-full bar and an empty one
	// were hard to tell apart in a recorded demo -- and a progress bar that
	// needs to be read carefully is not doing its job.
	styleBarFullV2  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleBarEmptyV2 = lipgloss.NewStyle().Foreground(colTerrain)

	// styleBreachV2 is the one alert that must survive a monochrome
	// terminal, a screenshot and the ASCII fold: reverse video is an SGR
	// attribute, not a colour, so termenv keeps it at every profile
	// including Ascii. The red foreground is additive decoration on top.
	styleBreachV2 = lipgloss.NewStyle().Reverse(true).Foreground(colAlert)
)

// glyphStyleV2 returns the style for a board glyph, by role. Terrain is
// dimmed, towers are the defender's colour, enemies the attacker's, and the
// breach marker gets the reverse-video treatment -- so the board's colour
// carries exactly the same information its glyph classes already do, and
// nothing more.
func glyphStyleV2(r rune) lipgloss.Style {
	switch r {
	case breachGlyphV2:
		return styleBreachV2
	case pathGlyphV2, flowGlyphV2, wallGlyphV2, slowZoneGlyphV2, rangeGlyphV2:
		return styleTerrainV2
	}
	for _, g := range towerGlyphV2 {
		if g == r {
			return styleDefenderV2
		}
	}
	for _, g := range enemyGlyphV2 {
		if g == r {
			return styleAttackerV2
		}
	}
	return lipgloss.NewStyle()
}

// sideStyleV2 returns the style for a feed row's side column ("DEF" / "ATT"
// / ">>>" / "***").
func sideStyleV2(side string) lipgloss.Style {
	switch side {
	case "DEF":
		return styleDefenderV2
	case "ATT":
		return styleAttackerV2
	case ">>>":
		return styleEngineV2
	case "***":
		return styleAlertV2
	default:
		return styleDimV2
	}
}
