package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestGlyphsAreOneDisplayColumn is rule 2, verified rather than asserted by
// inspection: every glyph in the vocabulary must occupy exactly one terminal
// cell, or the board grid shears. frameDisplayWidth is the same measurement
// the mockup fixtures are checked with, so this and the fixtures cannot
// disagree about what "one column" means.
func TestGlyphsAreOneDisplayColumn(t *testing.T) {
	var all []rune
	for _, g := range towerGlyphV2 {
		all = append(all, g)
	}
	for _, g := range enemyGlyphV2 {
		all = append(all, g)
	}
	all = append(all,
		pathGlyphV2, flowGlyphV2, wallGlyphV2, slowZoneGlyphV2,
		breachGlyphV2, unknownGlyphV2)

	for _, r := range all {
		if w := frameDisplayWidth(string(r)); w != 1 {
			t.Fatalf("glyph %q is %d display columns wide, want 1", string(r), w)
		}
	}
}

// TestGlyphClassEncodesOwnership is rule 1: a reader must be able to tell a
// tower from an enemy by glyph CLASS alone, with no colour. Towers are
// punctuation, enemies are lowercase letters, and the two sets must not
// intersect.
func TestGlyphClassEncodesOwnership(t *testing.T) {
	for typ, g := range towerGlyphV2 {
		if g >= 'a' && g <= 'z' {
			t.Fatalf("tower %q uses the lowercase glyph %q, which reads as an enemy", typ, string(g))
		}
	}
	for typ, g := range enemyGlyphV2 {
		if g < 'a' || g > 'z' {
			t.Fatalf("enemy %q uses %q, which is not a lowercase letter", typ, string(g))
		}
	}
	for towerType, tg := range towerGlyphV2 {
		for enemyType, eg := range enemyGlyphV2 {
			if tg == eg {
				t.Fatalf("tower %q and enemy %q share the glyph %q", towerType, enemyType, string(tg))
			}
		}
	}
}

// TestGlyphsAreDistinctWithinASide checks no two towers (or two enemies)
// collide -- a board where a sniper and a splash draw the same character is
// worse than one with no glyphs at all, because it looks readable.
func TestGlyphsAreDistinctWithinASide(t *testing.T) {
	for _, table := range []map[string]rune{towerGlyphV2, enemyGlyphV2} {
		seen := map[rune]string{}
		for typ, g := range table {
			if prev, dup := seen[g]; dup {
				t.Fatalf("%q and %q both render as %q", prev, typ, string(g))
			}
			seen[g] = typ
		}
	}
}

// TestUnknownTypeIsMarkedUnknown checks an unrecognised type is never given
// a plausible-looking default. A wrong-but-believable glyph on the board is
// read as fact; a '?' is read as a question.
func TestUnknownTypeIsMarkedUnknown(t *testing.T) {
	if g := towerGlyph("teleporter"); g != unknownGlyphV2 {
		t.Fatalf("unknown tower type rendered as %q", string(g))
	}
	if g := enemyGlyph("swarm"); g != unknownGlyphV2 {
		t.Fatalf("unknown enemy type rendered as %q", string(g))
	}
	// The prose name falls back to the raw type instead, since a legend
	// entry reading "?" twice would be less useful than the type string.
	if n := enemyDisplayName("swarm"); n != "swarm" {
		t.Fatalf("unknown enemy display name = %q, want the raw type", n)
	}
}

// TestNoRetiredGlyphsInV2Output is the cross-file guard that closes the
// three-disagreeing-tables gap: the retired set must not appear in ANY v2
// renderer's output, including the feed and legend, not just the board.
// engine/balance.go still holds those characters for the old render path
// (see glyphs_v2.go's note), so this is what stops them leaking back in.
func TestNoRetiredGlyphsInV2Output(t *testing.T) {
	g := buildSeededGameV2(t, true)

	frames := map[string]string{
		"board":  strings.Join(renderFramedBoardV2(g, rect{w: boardMaxW, h: boardMaxH}, 0, "", ""), "\n"),
		"map":    strings.Join(renderMapPaneV2(g, rect{w: 80, h: 14}, 0), "\n"),
		"legend": strings.Join(renderLegendV2(g, rect{w: 74, h: 8}), "\n"),
		"gutter": strings.Join(renderLegendV2(g, rect{w: 16, h: 16}), "\n"),
		"feed":   strings.Join(RenderFeedV2(g.ReplayEvents, 74, 20), "\n"),
	}

	for name, frame := range frames {
		for _, r := range retiredGlyphsV2 {
			if strings.ContainsRune(frame, r) {
				t.Fatalf("%s output contains the retired glyph %q:\n%s", name, string(r), frame)
			}
		}
	}
}

// TestFeedAndBoardUseTheSameGlyphs is the positive half of the same gap: it
// is not enough that the feed avoids the retired set -- it has to use the
// SAME character the board draws, or a reader cannot match a feed row to the
// thing that moved on the map. This drives a real game to a state with both
// towers and enemies on the board and checks each side's glyph appears in
// both renderings.
func TestFeedAndBoardUseTheSameGlyphs(t *testing.T) {
	g := buildSeededGameV2(t, true)

	board := strings.Join(renderFramedBoardV2(g, rect{w: boardMaxW, h: boardMaxH}, 0, "", ""), "\n")
	feed := strings.Join(RenderFeedV2(g.ReplayEvents, 74, 40), "\n")

	checked := 0
	for _, tw := range g.Towers {
		glyph := towerGlyph(tw.TowerType)
		if !strings.ContainsRune(board, glyph) {
			t.Fatalf("board does not draw %q for tower type %q", string(glyph), tw.TowerType)
		}
		if strings.Contains(feed, tw.TowerType) && !strings.ContainsRune(feed, glyph) {
			t.Fatalf("feed names tower type %q but never draws its glyph %q:\n%s",
				tw.TowerType, string(glyph), feed)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("the seeded game placed no towers, so this test checked nothing")
	}
}

// TestLegendCoversEveryGlyph checks the legend is exhaustive: every tower
// and enemy type in the vocabulary must appear in the wide legend, or a
// reader meets a character on the board with nothing to look it up with.
func TestLegendCoversEveryGlyph(t *testing.T) {
	g := buildSeededGameV2(t, true)
	legend := strings.Join(renderLegendV2(g, rect{w: 74, h: 8}), "\n")

	for typ, glyph := range towerGlyphV2 {
		if !strings.ContainsRune(legend, glyph) {
			t.Fatalf("legend omits the glyph %q for tower %q:\n%s", string(glyph), typ, legend)
		}
	}
	for typ, glyph := range enemyGlyphV2 {
		if !strings.ContainsRune(legend, glyph) {
			t.Fatalf("legend omits the glyph %q for enemy %q:\n%s", string(glyph), typ, legend)
		}
	}
	if lipgloss.Width(legend) == 0 {
		t.Fatal("legend rendered empty")
	}
}
