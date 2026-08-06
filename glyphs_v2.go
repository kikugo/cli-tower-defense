package main

// The redesign's single glyph vocabulary.
//
// This file exists to close a specific gap. Phase 2 was written by parallel
// agents given disjoint files, and three of those files ended up owning
// three different answers to "what character is a sniper":
//
//	render_board_v2.go   ^ ! * +   /  o f t s h   (the new design)
//	engine/balance.go    ^ ⌖ ⊕ B   /  o > □ S H   (the retired set, still
//	                                               stamped onto Entity.Char)
//	render_feed_v2.go    -- rendered type NAMES, because it had no basis to
//	                        pick between the other two
//
// Nobody broke the file boundary; the boundary was drawn around the wrong
// thing. A glyph set is one concept, so it gets one file, and every v2
// renderer reads it from here.
//
// --- Rule 1: ownership is the primary encoding, not type -------------------
//
// Every glyph belongs to a side and the glyph CLASS alone says which:
// punctuation (^ ! * +) is always a tower, lowercase letters (o f t s h) are
// always an enemy. A monochrome screenshot still reads correctly, which is
// the property the old set lost by encoding side in colour.
//
// --- Rule 2: every glyph is exactly one display column ----------------------
//
// The old set (⬡ ⌖ ⊕ ≋ □ ✗ ♥ ⛁) is retired: several of those are two display
// cells wide in emoji-capable fonts and break the grid. Every glyph here is a
// single ASCII byte, hence trivially one column -- verified rather than
// asserted, by TestBoardV2GlyphsAreOneColumn.
//
// engine/balance.go's towerChars/enemyChars are NOT reconciled with this
// table, deliberately: they are only read through Entity.Char by the old
// board_viewport.go render path, which the Phase 4 cutover deletes. Making
// the engine agree with a presentation table it is about to stop having any
// opinion about would be churn with a determinism gate attached.
// TestNoRetiredGlyphsInV2Output is what keeps the retired set out of the new
// UI in the meantime.

// towerGlyphV2 maps engine tower-type strings to the redesign's punctuation
// glyphs.
var towerGlyphV2 = map[string]rune{
	"basic":  '^',
	"sniper": '!',
	"splash": '*',
	"buffer": '+',
}

// enemyGlyphV2 maps engine enemy-type strings to the redesign's lowercase
// glyphs.
var enemyGlyphV2 = map[string]rune{
	"basic":    'o',
	"fast":     'f',
	"tank":     't',
	"shielded": 's',
	"healer":   'h',
}

// enemyDisplayNameV2 gives the prose name for each engine enemy type. The
// engine's type string for the base enemy is "basic"; the design labels it
// "grunt" -- same entity, glyph 'o', just a name that doesn't collide with
// the basic TOWER in prose.
var enemyDisplayNameV2 = map[string]string{
	"basic":    "grunt",
	"fast":     "fast",
	"tank":     "tank",
	"shielded": "shielded",
	"healer":   "healer",
}

const (
	pathGlyphV2     = '.'
	flowGlyphV2     = '>'
	wallGlyphV2     = '#'
	slowZoneGlyphV2 = '~'
	breachGlyphV2   = 'X'
)

// unknownGlyphV2 is what an unrecognised type renders as. It is '?' rather
// than a plausible-looking default: a type this build has never heard of is
// exactly the case where guessing "probably a basic tower" would put a
// confident wrong glyph on the board, and a board is read faster than it is
// questioned.
const unknownGlyphV2 = '?'

// towerGlyph returns the glyph for an engine tower-type string, or
// unknownGlyphV2.
func towerGlyph(towerType string) rune {
	if g, ok := towerGlyphV2[towerType]; ok {
		return g
	}
	return unknownGlyphV2
}

// enemyGlyph returns the glyph for an engine enemy-type string, or
// unknownGlyphV2.
func enemyGlyph(enemyType string) rune {
	if g, ok := enemyGlyphV2[enemyType]; ok {
		return g
	}
	return unknownGlyphV2
}

// enemyDisplayName returns the prose name for an engine enemy-type string,
// falling back to the raw type so an unrecognised one is still legible
// rather than blank.
func enemyDisplayName(enemyType string) string {
	if n, ok := enemyDisplayNameV2[enemyType]; ok {
		return n
	}
	return enemyType
}

// retiredGlyphsV2 is the exact set this redesign removed. Kept in production
// code rather than only in a test because "these must never appear" is as
// much a part of the glyph vocabulary as the glyphs that replaced them.
var retiredGlyphsV2 = []rune{'⬡', '⌖', '⊕', '≋', '□', '✗', '♥', '⛁'}
