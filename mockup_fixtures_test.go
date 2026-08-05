package main

// Table-driven runs of the three fit-invariant checks (defined in
// mockup_fit_test.go) against every tracked mockup fixture in
// testdata/mockups/, plus the ASCII-fallback substitution-equivalence test.
//
// The mockups have already been externally verified as exact character
// grids before being copied into testdata/mockups/ byte-for-byte, so a
// failure here means a checker is wrong, not a fixture -- per the task
// brief, fixtures are not to be adjusted to make a failing checker pass.

import (
	"os"
	"testing"
)

// mockupFixture pairs a tracked fixture file with its expected grid
// dimensions. Dimensions are declared explicitly (not computed from the
// file's own content) so this table asserts something real -- for the
// files whose names spell out a WxH grid, the expectation below is read
// directly off the filename; for the handful that don't (trust-80.txt,
// trust-160.txt, resolution-ladder.txt), the row count was determined by
// manually inspecting the fixture once, then hard-coded here, so
// checkFits below performs an independent recomputation rather than
// checking the file against itself.
var mockupFixtures = []struct {
	file string
	cols int
	rows int
}{
	// Filename spells out cols x rows directly.
	{"80x24.txt", 80, 24},
	{"100x30.txt", 100, 30},
	{"160x50.txt", 160, 50},
	{"ascii-fallback-100x30.txt", 100, 30},
	{"gameover-100x30.txt", 100, 30},
	{"gameover-emblem-100x30.txt", 100, 30},
	{"title-80x24.txt", 80, 24},
	{"title-160x50.txt", 160, 50},
	// Filename only spells out width (or nothing); row count below was
	// counted by hand against the fixture content.
	{"trust-80.txt", 80, 19},
	{"trust-160.txt", 160, 30},
	{"resolution-ladder.txt", 78, 12},
}

func TestMockupFixturesFit(t *testing.T) {
	for _, tc := range mockupFixtures {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile("testdata/mockups/" + tc.file)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			frame := string(data)

			if err := checkFits(frame, tc.cols, tc.rows); err != nil {
				t.Errorf("checkFits(%s, %d, %d): %v", tc.file, tc.cols, tc.rows, err)
			}
			if err := checkNoOrphanDividers(frame); err != nil {
				t.Errorf("checkNoOrphanDividers(%s): %v", tc.file, err)
			}
		})
	}
}

// asciiSubstitution is the exact character-for-character mapping the task
// brief specifies as the property that lets the design claim its two render
// modes (Unicode box-drawing and plain-ASCII fallback) share one layout:
// applying it to the Unicode mockup must reproduce the ASCII fallback
// mockup exactly.
var asciiSubstitution = map[rune]rune{
	'─': '-',
	'│': '|',
	'┌': '+', '┐': '+', '└': '+', '┘': '+',
	'┬': '+', '┴': '+', '┼': '+',
	'█': '#',
	'░': '.',
	'╭': '+', '╮': '+', '╰': '+', '╯': '+',
}

func applyAsciiSubstitution(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if repl, ok := asciiSubstitution[r]; ok {
			out = append(out, repl)
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// TestAsciiFallbackIsSubstitutionOfUnicodeMockup asserts that
// testdata/mockups/ascii-fallback-100x30.txt is exactly
// testdata/mockups/100x30.txt with asciiSubstitution applied,
// character-for-character, with zero deviations -- and that the fallback
// contains no non-ASCII bytes at all, i.e. the substitution table above is
// a complete accounting of every non-ASCII glyph the design uses.
func TestAsciiFallbackIsSubstitutionOfUnicodeMockup(t *testing.T) {
	unicodeSrc, err := os.ReadFile("testdata/mockups/100x30.txt")
	if err != nil {
		t.Fatalf("read unicode mockup: %v", err)
	}
	fallback, err := os.ReadFile("testdata/mockups/ascii-fallback-100x30.txt")
	if err != nil {
		t.Fatalf("read ascii fallback mockup: %v", err)
	}

	got := applyAsciiSubstitution(string(unicodeSrc))
	want := string(fallback)

	if got != want {
		// Find the first differing rune index for a useful failure message.
		gr, wr := []rune(got), []rune(want)
		n := len(gr)
		if len(wr) < n {
			n = len(wr)
		}
		i := 0
		for i < n && gr[i] == wr[i] {
			i++
		}
		var gotRune, wantRune rune
		if i < len(gr) {
			gotRune = gr[i]
		}
		if i < len(wr) {
			wantRune = wr[i]
		}
		t.Fatalf("substituting 100x30.txt does not match ascii-fallback-100x30.txt exactly: first deviation at rune index %d: got %q, want %q (len got=%d, want=%d)",
			i, gotRune, wantRune, len(gr), len(wr))
	}

	for i, b := range fallback {
		if b > 127 {
			t.Fatalf("ascii-fallback-100x30.txt contains a non-ASCII byte 0x%02x at offset %d, want pure ASCII", b, i)
		}
	}
}
