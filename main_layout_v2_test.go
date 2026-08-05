package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// --- sweep ranges -----------------------------------------------------

// sweepWidthsV2 and sweepHeightsV2 build the size grid the task brief
// specifies: every width 60-200 and every height 15-60 (the range that
// exercises every mode band at least once), plus the boundary values named
// explicitly in the brief. Duplicates from the boundary list overlapping
// the dense range are harmless -- they just re-run the same case.
func sweepWidthsV2() []int {
	ws := make([]int, 0, 210)
	for w := 60; w <= 200; w++ {
		ws = append(ws, w)
	}
	ws = append(ws, 59, 60, 79, 80, 83, 84, 99, 100, 144, 145)
	return ws
}

func sweepHeightsV2() []int {
	hs := make([]int, 0, 50)
	for h := 15; h <= 60; h++ {
		hs = append(hs, h)
	}
	hs = append(hs, 14, 15, 23, 24)
	return hs
}

// --- grid coverage check ------------------------------------------------

// tileCoverage marks a w x h grid of cells by walking every rect's actual
// cell range (not by summing areas -- an arithmetic sum can agree with a
// buggy layout that made the same arithmetic error in two places, e.g. a
// pane one row too short paired with its neighbour starting one row too
// early would still sum correctly while leaving a real gap or overlap
// somewhere else). Returns every uncovered and double-covered cell,
// capped at 10 of each so a failing test's message stays readable.
func tileCoverage(rects []paneRectV2, w, h int) (uncovered, doubleCovered []string) {
	if w <= 0 || h <= 0 {
		return nil, nil
	}
	counts := make([][]int, h)
	for i := range counts {
		counts[i] = make([]int, w)
	}
	for _, r := range rects {
		if r.area() == 0 {
			continue
		}
		for yy := r.y; yy < r.y+r.h; yy++ {
			for xx := r.x; xx < r.x+r.w; xx++ {
				if yy >= 0 && yy < h && xx >= 0 && xx < w {
					counts[yy][xx]++
				}
				// Off-frame cells are the responsibility of
				// TestComputeLayoutV2NoNegativeOrOffFrameRects, not this
				// coverage check.
			}
		}
	}
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			switch {
			case counts[yy][xx] == 0:
				if len(uncovered) < 10 {
					uncovered = append(uncovered, fmt.Sprintf("(row %d, col %d)", yy, xx))
				}
			case counts[yy][xx] > 1:
				if len(doubleCovered) < 10 {
					doubleCovered = append(doubleCovered, fmt.Sprintf("(row %d, col %d) x%d", yy, xx, counts[yy][xx]))
				}
			}
		}
	}
	return uncovered, doubleCovered
}

// TestComputeLayoutV2PartitionsFrameExactly is test 1 from the task brief:
// over the full size sweep, every pane rect returned by computeLayoutV2
// must exactly tile the w x h frame -- no gap, no overlap, no cell left
// over. This is checked via a real per-cell coverage count (tileCoverage
// above), not an arithmetic sum-of-areas identity, specifically so it can
// catch the bug class where two independent off-by-one errors cancel out
// in a sum but still leave a real hole or overlap on the grid.
func TestComputeLayoutV2PartitionsFrameExactly(t *testing.T) {
	for _, h := range sweepHeightsV2() {
		for _, w := range sweepWidthsV2() {
			l := computeLayoutV2(w, h)
			uncovered, double := tileCoverage(l.rects(), w, h)
			if len(uncovered) > 0 || len(double) > 0 {
				t.Fatalf("w=%d h=%d mode=%v: does not exactly tile the frame -- uncovered=%v double-covered=%v",
					w, h, l.mode, uncovered, double)
			}
		}
	}
}

// TestComputeLayoutV2NoNegativeOrOffFrameRects is test 2 from the task
// brief, kept as an explicit standalone check (rather than relying on it
// being implied by the partition test) because a negative or off-frame
// rect is the specific bug class that made the board disappear before this
// rewrite -- it deserves a test that names exactly that failure mode.
func TestComputeLayoutV2NoNegativeOrOffFrameRects(t *testing.T) {
	for _, h := range sweepHeightsV2() {
		for _, w := range sweepWidthsV2() {
			l := computeLayoutV2(w, h)
			for _, r := range l.rects() {
				if r.x < 0 || r.y < 0 || r.w < 0 || r.h < 0 {
					t.Fatalf("w=%d h=%d mode=%v: negative rect %+v", w, h, l.mode, r)
				}
				if r.area() == 0 {
					continue
				}
				if r.x+r.w > w || r.y+r.h > h {
					t.Fatalf("w=%d h=%d mode=%v: rect %+v extends off-frame (frame is %dx%d)", w, h, l.mode, r, w, h)
				}
			}
		}
	}
}

// TestComputeLayoutV2ModeSelectionBoundaries is test 3: mode selection
// matches the design table exactly at every boundary, tested at both sides
// of each edge (79 vs 80, 83 vs 84, 99 vs 100, 144 vs 145, h 14 vs 15).
func TestComputeLayoutV2ModeSelectionBoundaries(t *testing.T) {
	cases := []struct {
		w, h int
		want layoutModeV2
	}{
		{59, 20, modeNotice},
		{60, 20, modeCompact},
		{79, 20, modeCompact},
		{80, 20, modeMinimum},
		{83, 20, modeMinimum},
		{84, 20, modeNarrow},
		{99, 20, modeNarrow},
		{100, 20, modeMid},
		{144, 20, modeMid},
		{145, 20, modeWide},
		// h boundary, tested at a width unambiguously above every width
		// threshold so only the height edge is under test.
		{200, 14, modeNotice},
		{200, 15, modeWide},
	}
	for _, c := range cases {
		if got := computeLayoutV2(c.w, c.h).mode; got != c.want {
			t.Fatalf("computeLayoutV2(%d,%d).mode = %v, want %v", c.w, c.h, got, c.want)
		}
	}
}

// --- mockup agreement ---------------------------------------------------

// mustReadMockup reads a tracked fixture and splits it into lines (no
// trailing empty line from a final "\n", matching checkFits' own
// TrimSuffix convention in mockup_fit_test.go).
func mustReadMockup(t *testing.T, file string) []string {
	t.Helper()
	data, err := os.ReadFile("testdata/mockups/" + file)
	if err != nil {
		t.Fatalf("read fixture %s: %v", file, err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// firstLineContaining returns the 0-indexed line number of the first line
// containing substr, failing the test if none matches.
func firstLineContaining(t *testing.T, lines []string, substr string) int {
	t.Helper()
	for i, line := range lines {
		if strings.Contains(line, substr) {
			return i
		}
	}
	t.Fatalf("no line contains %q", substr)
	return -1
}

// TestComputeLayoutV2MockupsAgree is test 4: for each of the three sized
// mockups, the mode computeLayoutV2 selects for those exact dimensions must
// be the one the design table calls for, and the row positions it computes
// for the panes whose top edge is visually identifiable in the fixture
// (the board's top border, the MOVE FEED divider, the MATCH TIMELINE
// divider, the cards-column header, the LEGEND divider, and the keybar's
// row) must equal that visual row's actual 0-indexed line number in the
// fixture. This isn't a content comparison (this phase renders nothing);
// it's checking the arithmetic against the one part of the fixture's
// structure that's unambiguous without rendering: which row a distinctive,
// grep-able divider line sits on.
func TestComputeLayoutV2MockupsAgree(t *testing.T) {
	t.Run("80x24_minimum", func(t *testing.T) {
		lines := mustReadMockup(t, "80x24.txt")
		if len(lines) != 24 {
			t.Fatalf("fixture has %d lines, want 24", len(lines))
		}
		l := computeLayoutV2(80, 24)
		if l.mode != modeMinimum {
			t.Fatalf("computeLayoutV2(80,24).mode = %v, want modeMinimum", l.mode)
		}
		if want := firstLineContaining(t, lines, "MOVE FEED"); l.feed.y != want {
			t.Fatalf("feed.y = %d, want %d (the fixture's MOVE FEED divider row)", l.feed.y, want)
		}
		if l.keys.y != len(lines)-1 {
			t.Fatalf("keys.y = %d, want %d (the fixture's last row)", l.keys.y, len(lines)-1)
		}
		// Row allocation per the design table: header 2, label 1, map 14,
		// feed h-18=6, keys 1.
		if l.header.h != 2 || l.label.h != 1 || l.mapPane.h != 14 || l.feed.h != 6 || l.keys.h != 1 {
			t.Fatalf("minimum mode row allocation at h=24: header=%d label=%d map=%d feed=%d keys=%d, want 2/1/14/6/1",
				l.header.h, l.label.h, l.mapPane.h, l.feed.h, l.keys.h)
		}
	})

	t.Run("100x30_mid", func(t *testing.T) {
		lines := mustReadMockup(t, "100x30.txt")
		if len(lines) != 30 {
			t.Fatalf("fixture has %d lines, want 30", len(lines))
		}
		l := computeLayoutV2(100, 30)
		if l.mode != modeMid {
			t.Fatalf("computeLayoutV2(100,30).mode = %v, want modeMid", l.mode)
		}
		if want := firstLineContaining(t, lines, "┌"); l.board.y != want {
			t.Fatalf("board.y = %d, want %d (the fixture's board top border row)", l.board.y, want)
		}
		if want := firstLineContaining(t, lines, "MOVE FEED"); l.feed.y != want {
			t.Fatalf("feed.y = %d, want %d (the fixture's MOVE FEED divider row)", l.feed.y, want)
		}
		if l.keys.y != len(lines)-1 {
			t.Fatalf("keys.y = %d, want %d (the fixture's last row)", l.keys.y, len(lines)-1)
		}
		// Row allocation per the design table: header 3, board 16,
		// feed h-20=10, keys 1. Legend shares the board's row span.
		if l.header.h != 3 || l.board.h != 16 || l.legend.h != 16 || l.feed.h != 10 || l.keys.h != 1 {
			t.Fatalf("mid mode row allocation at h=30: header=%d board=%d legend=%d feed=%d keys=%d, want 3/16/16/10/1",
				l.header.h, l.board.h, l.legend.h, l.feed.h, l.keys.h)
		}
	})

	t.Run("160x50_wide", func(t *testing.T) {
		lines := mustReadMockup(t, "160x50.txt")
		if len(lines) != 50 {
			t.Fatalf("fixture has %d lines, want 50", len(lines))
		}
		l := computeLayoutV2(160, 50)
		if l.mode != modeWide {
			t.Fatalf("computeLayoutV2(160,50).mode = %v, want modeWide", l.mode)
		}
		if want := firstLineContaining(t, lines, "┌"); l.board.y != want {
			t.Fatalf("board.y = %d, want %d (the fixture's board top border row)", l.board.y, want)
		}
		if want := firstLineContaining(t, lines, "─ DEFENDER"); l.cards.y != want {
			t.Fatalf("cards.y = %d, want %d (the fixture's cards-column header row)", l.cards.y, want)
		}
		if want := firstLineContaining(t, lines, "MATCH TIMELINE"); l.timeline.y != want {
			t.Fatalf("timeline.y = %d, want %d (the fixture's MATCH TIMELINE divider row)", l.timeline.y, want)
		}
		if want := firstLineContaining(t, lines, "LEGEND"); l.legend.y != want {
			t.Fatalf("legend.y = %d, want %d (the fixture's LEGEND divider row)", l.legend.y, want)
		}
		if l.keys.y != len(lines)-1 {
			t.Fatalf("keys.y = %d, want %d (the fixture's last row)", l.keys.y, len(lines)-1)
		}
		// Row allocation per the design table: header 4, body h-5=45
		// (board 16 + cards 16 + timeline 13 on the left; feed 36 + gap 1
		// + legend 8 on the right, since body-9=36 >= 20 so the legend is
		// shown), keys 1.
		if l.header.h != 4 || l.board.h != 16 || l.cards.h != 16 || l.timeline.h != 13 ||
			l.feed.h != 36 || l.gap.h != 1 || l.legend.h != 8 || l.keys.h != 1 {
			t.Fatalf("wide mode row allocation at h=50: header=%d board=%d cards=%d timeline=%d feed=%d gap=%d legend=%d keys=%d, want 4/16/16/13/36/1/8/1",
				l.header.h, l.board.h, l.cards.h, l.timeline.h, l.feed.h, l.gap.h, l.legend.h, l.keys.h)
		}
	})
}

// --- feed-slack behaviour -------------------------------------------------

// TestComputeLayoutV2FeedIsSlackAbsorberInSingleColumnModes is (the
// single-column half of) test 5: in every mode with just one column
// (compact, minimum, narrow, mid), growing h by 1 must grow the feed pane
// by exactly 1 row and leave every other pane's height unchanged, once
// every fixed pane is already fully paid (i.e. away from the h=15 low end
// where clamping can still be biting -- that transition is a separate,
// deliberate behaviour, not a bug, so it's excluded here by starting well
// past it).
func TestComputeLayoutV2FeedIsSlackAbsorberInSingleColumnModes(t *testing.T) {
	widths := map[string]int{"compact": 70, "minimum": 80, "narrow": 90, "mid": 120}
	for name, w := range widths {
		t.Run(name, func(t *testing.T) {
			prev := computeLayoutV2(w, 40)
			for h := 41; h <= 60; h++ {
				cur := computeLayoutV2(w, h)
				if cur.feed.h != prev.feed.h+1 {
					t.Fatalf("w=%d h=%d: feed.h = %d, want prev(%d)+1 = %d", w, h, cur.feed.h, prev.feed.h, prev.feed.h+1)
				}
				// Compare heights only, not full rects: keys.y (and every
				// pane below the growing feed) legitimately shifts down as
				// feed grows -- that's the expected consequence of feed
				// being the slack absorber, not a bug. What must NOT
				// change is any pane's own HEIGHT.
				if cur.header.h != prev.header.h || cur.keys.h != prev.keys.h || cur.label.h != prev.label.h ||
					cur.mapPane.h != prev.mapPane.h || cur.board.h != prev.board.h || cur.legend.h != prev.legend.h {
					t.Fatalf("w=%d h=%d: a non-feed pane changed height when only h grew (mode=%v)\n  prev=%+v\n  cur =%+v", w, h, cur.mode, prev, cur)
				}
				prev = cur
			}
		})
	}
}

// TestComputeLayoutV2WideModeLegendDisappearsBelow20FeedRows is (the
// wide-mode half of) test 5: the legend must be present exactly when
// body-9 (the feed height it would leave behind) is >= 20, and absent
// exactly when it's below that -- checked at the literal threshold
// (body 28 vs 29, i.e. h 32 vs 33 at header=4+keys=1) rather than inferred.
//
// NOTE on a brief discrepancy: the design's additional-rules prose says
// "only the feed absorbs slack", but the wide-mode column table lists BOTH
// "timeline rest" (left column) and "feed rest" (right column) as the
// last, and TestComputeLayoutV2MockupsAgree's 160x50 case confirms the
// mockup's actual row math matches "timeline also grows with h", not
// "only feed grows". This test therefore checks feed's own growth and the
// legend threshold (both true), and separately below confirms timeline
// also grows -- it does NOT assert feed is the only pane whose height
// changes in wide mode, because that isn't what the mockup shows.
func TestComputeLayoutV2WideModeLegendDisappearsBelow20FeedRows(t *testing.T) {
	const w = 160 // comfortably >= 145
	for _, tc := range []struct {
		h          int
		wantLegend bool
		wantFeedH  int
	}{
		{h: 4 + 28 + 1, wantLegend: false, wantFeedH: 28}, // body=28, body-9=19 < 20
		{h: 4 + 29 + 1, wantLegend: true, wantFeedH: 20},  // body=29, body-9=20 >= 20
	} {
		l := computeLayoutV2(w, tc.h)
		gotLegend := l.legend.area() > 0
		if gotLegend != tc.wantLegend {
			t.Fatalf("h=%d (body=%d): legend present = %v, want %v", tc.h, tc.h-5, gotLegend, tc.wantLegend)
		}
		if l.feed.h != tc.wantFeedH {
			t.Fatalf("h=%d: feed.h = %d, want %d", tc.h, l.feed.h, tc.wantFeedH)
		}
		if tc.wantLegend {
			if l.gap.area() == 0 {
				t.Fatalf("h=%d: legend shown but gap is absent", tc.h)
			}
		} else {
			if l.gap.area() != 0 {
				t.Fatalf("h=%d: legend hidden but gap is still present: %+v", tc.h, l.gap)
			}
		}
	}
}

// TestComputeLayoutV2WideModeFeedAndTimelineBothGrow documents and locks
// down the discrepancy noted above: in wide mode, holding w fixed and
// growing h (once board+cards are already fully paid, i.e. body >= 32),
// BOTH feed and timeline grow -- board, cards, header, keys, rule and (once
// past the legend threshold) legend/gap stay constant. This is the
// mockup-verified behaviour, not the "only the feed absorbs slack" prose.
func TestComputeLayoutV2WideModeFeedAndTimelineBothGrow(t *testing.T) {
	const w = 160
	prev := computeLayoutV2(w, 50) // body=45, well past both the board+cards floor and the legend threshold
	for h := 51; h <= 60; h++ {
		cur := computeLayoutV2(w, h)
		if cur.feed.h != prev.feed.h+1 {
			t.Fatalf("h=%d: feed.h = %d, want %d", h, cur.feed.h, prev.feed.h+1)
		}
		if cur.timeline.h != prev.timeline.h+1 {
			t.Fatalf("h=%d: timeline.h = %d, want %d (timeline is also a rest-pane per the wide-mode column table)", h, cur.timeline.h, prev.timeline.h+1)
		}
		// rule.h is deliberately excluded: the rule pane spans the full
		// body height by definition (it's the column divider), so it
		// grows in lockstep with body -- same as feed and timeline, not a
		// "constant" pane. Heights only, for the same reason as the
		// single-column check above: positions legitimately shift.
		if cur.board.h != prev.board.h || cur.cards.h != prev.cards.h || cur.header.h != prev.header.h || cur.keys.h != prev.keys.h || cur.legend.h != prev.legend.h {
			t.Fatalf("h=%d: an expected-constant pane changed\n  prev=%+v\n  cur =%+v", h, prev, cur)
		}
		prev = cur
	}
}
