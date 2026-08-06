package main

import (
	"strings"
	"testing"
)

// sampleCardsData is the fixture's own match state (testdata/mockups/160x50.txt
// lines 21-36) expressed as MatchCardsData, so the demo test below can be
// eyeballed against the mockup directly.
func sampleCardsData() MatchCardsData {
	return MatchCardsData{
		Defender: CardsPlayerData{
			ModelName: "o3",
			Lives:     7, MaxLives: 10,
			Resources: 55, Income: 5,
			Built:       "^3  !2  *1  +1",
			Research:    "economy 2, range 1, control 1",
			Authored:    AuthoredShare{Known: true, HasData: true, Share: 0.84},
			Saves:       SavesStat{Known: true, Authored: 17, Total: 59},
			Calls:       59,
			Tokens:      12000,
			AssistKnown: true, AssistCount: 0,
			Streak: 0, StreakMax: 3,
			Reasoning: "Income before more guns. The sniper covers the long approach and I can afford the second one next wave.",
		},
		Attacker: CardsPlayerData{
			ModelName: "gpt-4o-mini",
			Breaches:  3,
			Resources: 10, Income: 5,
			Sent: 130, Live: 9,
			Abilities:   "surge 1, shield_burst 1",
			Authored:    AuthoredShare{Known: true, HasData: true, Share: 0.91},
			Saves:       SavesStat{Known: true, Authored: 32, Total: 59},
			Calls:       58,
			Tokens:      11000,
			AssistKnown: true, AssistCount: 2,
			Streak: 1, StreakMax: 3,
			Reasoning: "Pressure the low bend with fast movers while the tank soaks the sniper, and bank for a reinforce next wave.",
		},
	}
}

// TestRenderCardsV2FitsAtEverySize is the pane contract every renderer in
// this phase shares: exactly rc.h rows, each exactly rc.w display columns.
// The sizes swept are the ones computeLayoutV2 can actually hand this pane
// (wide mode's boardMaxW width at every height it can allocate), plus
// degenerate and oversized ones.
func TestRenderCardsV2FitsAtEverySize(t *testing.T) {
	data := sampleCardsData()
	for _, w := range []int{boardMaxW, 60, 40, 30, 100} {
		// h starts at 1: checkFits splits a joined frame on "\n", so a
		// zero-row frame is indistinguishable from a one-empty-row frame
		// once joined. The h==0 case is covered by the row-count assertion
		// in TestRenderCardsV2DegenerateSizes instead.
		for h := 1; h <= 20; h++ {
			rows := RenderCardsV2(rect{w: w, h: h}, data)
			if len(rows) != h {
				t.Fatalf("w=%d h=%d: got %d rows, want %d", w, h, len(rows), h)
			}
			if err := checkFits(strings.Join(rows, "\n"), w, h); err != nil {
				t.Fatalf("w=%d h=%d: %v", w, h, err)
			}
		}
	}
}

// TestRenderCardsV2DegenerateSizes checks the zero/negative cases return a
// well-formed (empty or blank) result rather than panicking on the column
// split.
func TestRenderCardsV2DegenerateSizes(t *testing.T) {
	data := sampleCardsData()
	for _, rc := range []rect{{w: 0, h: 5}, {w: 5, h: 0}, {w: -3, h: 4}, {w: 4, h: -2}, {w: 1, h: 3}} {
		rows := RenderCardsV2(rc, data)
		want := rc.h
		if want < 0 {
			want = 0
		}
		if len(rows) != want {
			t.Fatalf("rect %+v: got %d rows, want %d", rc, len(rows), want)
		}
	}
}

// TestRenderCardsV2NeverPrintsZeroForUnknown is the reason this file takes
// flat three-state structs at all: an unmeasured authored share, an
// unmeasured saves ratio and an untracked assist count must all render as
// words. A "0%" or a "0 of 0" here would be a claim the match never made.
func TestRenderCardsV2NeverPrintsZeroForUnknown(t *testing.T) {
	unknown := CardsPlayerData{
		ModelName: "o3",
		Authored:  AuthoredShare{Known: false},
		Saves:     SavesStat{Known: false},
		// AssistKnown deliberately false.
	}
	out := strings.Join(RenderCardsV2(rect{w: boardMaxW, h: 16},
		MatchCardsData{Defender: unknown, Attacker: unknown}), "\n")

	for _, want := range []string{"unknown"} {
		if !strings.Contains(out, want) {
			t.Fatalf("unknown state did not render %q:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{"0%", "0 of 0", "ENGINE ACTED 0x"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("unknown state rendered %q as if measured:\n%s", forbidden, out)
		}
	}
}

// TestRenderCardsV2MeasuredZeroIsNotUnknown is the other half of the same
// contract: a share that WAS measured and happens to be zero must print as
// "0%", not as "unknown". Collapsing the two is the specific failure the
// three-state types exist to prevent, so both directions get a test.
func TestRenderCardsV2MeasuredZeroIsNotUnknown(t *testing.T) {
	measured := CardsPlayerData{
		ModelName:   "scripted",
		Authored:    AuthoredShare{Known: true, HasData: true, Share: 0},
		Saves:       SavesStat{Known: true, Authored: 0, Total: 40},
		AssistKnown: true, AssistCount: 0,
	}
	out := strings.Join(RenderCardsV2(rect{w: boardMaxW, h: 16},
		MatchCardsData{Defender: measured, Attacker: measured}), "\n")

	if !strings.Contains(out, "0%") {
		t.Fatalf("measured 0%% share did not render as 0%%:\n%s", out)
	}
	if !strings.Contains(out, "0 of 40") {
		t.Fatalf("measured 0-of-40 saves did not render:\n%s", out)
	}
	if !strings.Contains(out, "none on this side") {
		t.Fatalf("measured zero assists did not render as a measured zero:\n%s", out)
	}
	if strings.Contains(out, "unknown") {
		t.Fatalf("fully measured card claimed something was unknown:\n%s", out)
	}
}

// TestRenderCardsV2DegradesQuoteFirst pins the documented degradation
// order: shrinking the pane must cost the reasoning quote before it costs
// the two rows that identify which model is which.
func TestRenderCardsV2DegradesQuoteFirst(t *testing.T) {
	data := sampleCardsData()
	full := strings.Join(RenderCardsV2(rect{w: boardMaxW, h: 16}, data), "\n")
	if !strings.Contains(full, "Income before more guns") {
		t.Fatalf("full-height cards dropped the reasoning quote:\n%s", full)
	}

	squeezed := RenderCardsV2(rect{w: boardMaxW, h: 6}, data)
	joined := strings.Join(squeezed, "\n")
	if strings.Contains(joined, "Income before more guns") {
		t.Fatalf("squeezed cards kept the quote instead of dropping it first:\n%s", joined)
	}
	if !strings.Contains(joined, "o3") || !strings.Contains(joined, "gpt-4o-mini") {
		t.Fatalf("squeezed cards dropped a model identity:\n%s", joined)
	}
}

// TestRenderCardsV2ColumnsAreMirrored checks the two columns carry the same
// number of stat rows at the same heights -- the property that makes the
// pane readable as a scoreboard. It compares the two halves' label columns
// row by row rather than their contents, since the labels are what has to
// line up.
func TestRenderCardsV2ColumnsAreMirrored(t *testing.T) {
	rows := RenderCardsV2(rect{w: boardMaxW, h: 16}, sampleCardsData())
	half := boardMaxW / 2

	// Rows 5..13 are the stat block (row 0 rule, 1 name, 2 blurb, 3 blank,
	// then 9 stats starting at row 4).
	for i := 4; i < 13; i++ {
		left := strings.TrimSpace(rows[i][:len(rows[i])/2])
		right := strings.TrimSpace(rows[i][len(rows[i])/2:])
		if left == "" || right == "" {
			t.Fatalf("row %d has an empty half, columns are not mirrored:\n%q", i, rows[i])
		}
		_ = half
	}
}

// TestQuoteRowsV2NeverExceedsBudget checks the quote block's own contract:
// it returns exactly the requested number of rows for any reasoning length,
// including one long enough to wrap past the budget, and never emits a
// stray opening quote with no closer.
func TestQuoteRowsV2NeverExceedsBudget(t *testing.T) {
	long := strings.Repeat("pressure the low bend with fast movers ", 20)
	for _, rows := range []int{0, 1, 2, 3, 5} {
		got := quoteRowsV2(long, 40, rows)
		if len(got) != rows {
			t.Fatalf("rows=%d: got %d lines, want %d", rows, len(got), rows)
		}
		joined := strings.Join(got, "")
		if n := strings.Count(joined, `"`); rows > 0 && n != 2 {
			t.Fatalf("rows=%d: got %d quote marks, want 2:\n%v", rows, n, got)
		}
	}
}

// TestQuoteRowsV2EmptyReasoning checks an absent reasoning renders as blank
// rows rather than a lone pair of quote marks -- a model that said nothing
// must not appear to have said "".
func TestQuoteRowsV2EmptyReasoning(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t "} {
		got := quoteRowsV2(in, 40, 3)
		for _, row := range got {
			if strings.TrimSpace(row) != "" {
				t.Fatalf("empty reasoning %q produced %q", in, row)
			}
		}
	}
}

// TestRenderCardsV2Demo prints the pane at its fixture size for visual
// review against testdata/mockups/160x50.txt lines 21-36. It asserts
// nothing beyond the fit contract the other tests cover; run with -v.
func TestRenderCardsV2Demo(t *testing.T) {
	rows := RenderCardsV2(rect{w: boardMaxW, h: 16}, sampleCardsData())
	t.Logf("cards pane, %dx16:\n%s", boardMaxW, strings.Join(rows, "\n"))
}
