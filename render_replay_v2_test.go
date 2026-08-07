package main

import (
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	eng "tower-defense/engine"
)

// replayModel builds a replay-mode model over a real scripted match's event
// stream, positioned at `frac` through it.
func replayModel(t *testing.T, w, h int, frac float64) model {
	t.Helper()
	g := newScriptedGame(t, "o3", "gpt-4")
	if len(g.ReplayEvents) < 20 {
		t.Fatalf("setup: only %d replay events", len(g.ReplayEvents))
	}
	return model{
		replayMode: true,
		replay:     g.ReplayEvents,
		replayIdx:  int(float64(len(g.ReplayEvents)) * frac),
		width:      w, height: h,
	}
}

// TestReplayViewV2ExactFrameAtEverySize is the same whole-view contract the
// live screen has: exactly h rows of exactly w display columns, at every
// layout mode and both boundaries either side of each mode switch, with
// colour on. Measured with lipgloss.Width because the board and feed carry
// ANSI and frameDisplayWidth counts escape bytes as columns.
func TestReplayViewV2ExactFrameAtEverySize(t *testing.T) {
	withColorProfile(t, termenv.ANSI256, func() {
		for _, frac := range []float64{0, 0.25, 0.75, 1} {
			for _, sz := range viewSizes {
				wantW, wantH := sz.w, sz.h
				if wantW == 0 || wantH == 0 {
					wantW, wantH = 80, 24
				}
				if computeLayoutV2(wantW, wantH).mode == modeNotice {
					continue
				}

				m := replayModel(t, sz.w, sz.h, frac)
				rows := strings.Split(m.View(), "\n")

				if len(rows) != wantH {
					t.Fatalf("frac=%.2f %dx%d: %d rows, want %d", frac, sz.w, sz.h, len(rows), wantH)
				}
				for i, row := range rows {
					if got := lipgloss.Width(row); got != wantW {
						t.Fatalf("frac=%.2f %dx%d row %d: %d columns, want %d: %q",
							frac, sz.w, sz.h, i, got, wantW, row)
					}
				}
			}
		}
	})
}

// TestReplayViewV2BoardFrameIsTheWholePane is the defect a fit test cannot
// see and only looking at the screen found.
//
// A replay map is whatever the recording says -- the truncation fixtures use
// 12x6, against the live board's fixed 80x14. The first port padded each map
// row to the VIEWPORT width and closed the frame after the last map row, so a
// small map produced a box with an 84-column border, 16-column content rows,
// and eight blank rows hanging below it. Every width assertion still passed,
// because the pane padding had already squared the rows off.
//
// The property: the frame's corners are at the pane's corners, and every row
// between them starts and ends with a border column.
func TestReplayViewV2BoardFrameIsTheWholePane(t *testing.T) {
	events := truncatedReplayStream(12) // a 12x6 map: far smaller than the pane
	snap := eng.ReconstructSnapshot(events, len(events))

	for _, rc := range []rect{{w: boardMaxW, h: boardMaxH}, {w: boardMaxW, h: 20}, {w: 60, h: 12}} {
		rows := renderReplayBoardV2(snap, nil, rc, "left", "right")
		if len(rows) != rc.h {
			t.Fatalf("%dx%d: %d rows", rc.w, rc.h, len(rows))
		}

		first, last := stripANSI(rows[0]), stripANSI(rows[len(rows)-1])
		if !strings.HasPrefix(first, "┌") || !strings.HasSuffix(strings.TrimRight(first, " "), "┐") {
			t.Fatalf("%dx%d top border is not a full-width frame: %q", rc.w, rc.h, first)
		}
		if !strings.HasPrefix(last, "└") || !strings.HasSuffix(strings.TrimRight(last, " "), "┘") {
			t.Fatalf("%dx%d bottom border is not at the foot of the pane: %q", rc.w, rc.h, last)
		}
		for i, row := range rows[1 : len(rows)-1] {
			plain := []rune(stripANSI(row))
			if plain[0] != '│' || plain[len(plain)-1] != '│' {
				t.Fatalf("%dx%d interior row %d is not framed on both sides: %q", rc.w, rc.h, i+1, string(plain))
			}
		}
	}
}

// TestReplayViewV2NoMapSaysSo: before map_init (or in a replay recorded
// before map_init existed) the board must state that there is no board, not
// render an empty frame. An empty board and an unknown board are different
// facts, and the older replays on disk are exactly where confusing them
// misleads.
func TestReplayViewV2NoMapSaysSo(t *testing.T) {
	rows := renderReplayBoardV2(eng.ReplaySnapshot{}, nil, rect{w: boardMaxW, h: boardMaxH}, "", "")
	joined := stripANSI(strings.Join(rows, "\n"))
	if !strings.Contains(joined, "NO BOARD YET") {
		t.Fatalf("a snapshot with no map did not say so:\n%s", joined)
	}
	if err := checkFits(joined, boardMaxW, boardMaxH); err != nil {
		t.Fatal(err)
	}
}

// TestReplayTruncationTextAlwaysStatesTheConsequence: both lengths of the
// disclosure must say what is WRONG with the board, not merely that a
// truncation occurred. "858 events discarded" tells a reader something
// happened; it does not tell them the board in front of them is incomplete,
// which is the only thing they need to know.
func TestReplayTruncationTextAlwaysStatesTheConsequence(t *testing.T) {
	snap := eng.ReplaySnapshot{Truncated: true, TruncatedEvents: 858}
	for _, long := range []bool{true, false} {
		got := replayTruncationText(snap, long)
		for _, want := range []string{"TRUNCATED", "missing", "858"} {
			if !strings.Contains(got, want) {
				t.Fatalf("long=%v: %q does not contain %q", long, got, want)
			}
		}
	}

	// And with no count recorded it must still state the consequence.
	noCount := replayTruncationText(eng.ReplaySnapshot{Truncated: true}, false)
	if !strings.Contains(noCount, "missing") {
		t.Fatalf("truncation with no count lost the consequence: %q", noCount)
	}

	// An untruncated snapshot must not imply anything is wrong.
	clean := replayTruncationText(eng.ReplaySnapshot{}, true)
	if strings.Contains(clean, "TRUNCATED") || strings.Contains(clean, "missing") {
		t.Fatalf("a complete stream was described as incomplete: %q", clean)
	}
}

// TestReplayLivesAreNotFabricated: ReplaySnapshot reports DefenderLives as
// -1 until a breach event reveals it, because the stream records lives only
// when they change. Rendering that as "lives -1", or silently as the
// starting value, would be inventing a number the reconstruction does not
// have.
func TestReplayLivesAreNotFabricated(t *testing.T) {
	unknown := fmtReplayLives(-1)
	if strings.Contains(unknown, "-1") {
		t.Fatalf("unrevealed lives rendered as a number: %q", unknown)
	}
	if !strings.Contains(unknown, "not yet revealed") {
		t.Fatalf("unrevealed lives did not say so: %q", unknown)
	}
	if got := fmtReplayLives(7); got != "lives 7" {
		t.Fatalf("known lives rendered as %q", got)
	}
}

// TestReplayViewV2FeedShowsOnlyThePast is the property that makes stepping
// through a replay meaningful: the feed must end at the playhead. A feed
// showing events the playhead has not reached would let a reader see the
// breach before the decision that caused it.
func TestReplayViewV2FeedShowsOnlyThePast(t *testing.T) {
	g := newScriptedGame(t, "o3", "gpt-4")
	events := g.ReplayEvents

	// Find a late breach and confirm it is absent from a feed built well
	// before it, and present in one built after.
	lateBreach := -1
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == eng.ReplayBreach {
			lateBreach = i
			break
		}
	}
	if lateBreach < 10 {
		t.Skip("the scripted match has no late breach to test against")
	}

	early := model{replayMode: true, replay: events, replayIdx: 5, width: 160, height: 50}
	late := model{replayMode: true, replay: events, replayIdx: lateBreach, width: 160, height: 50}

	l := computeLayoutV2(160, 50)
	earlyFeed := strings.Join(early.replayFeedPaneV2(l, 5), "\n")
	lateFeed := strings.Join(late.replayFeedPaneV2(l, lateBreach), "\n")

	if strings.Contains(stripANSI(earlyFeed), "BREACH") {
		t.Fatalf("the feed at event 5 shows a breach that has not happened yet:\n%s", stripANSI(earlyFeed))
	}
	if !strings.Contains(stripANSI(lateFeed), "BREACH") {
		t.Fatalf("the feed at the breach event does not show it:\n%s", stripANSI(lateFeed))
	}
}

// TestReplayViewV2AsciiMode: --ascii covers the ported screen at every mode.
func TestReplayViewV2AsciiMode(t *testing.T) {
	withColorProfile(t, termenv.ANSI256, func() {
		for _, sz := range viewSizes {
			if sz.w == 0 || computeLayoutV2(sz.w, sz.h).mode == modeNotice {
				continue
			}
			m := replayModel(t, sz.w, sz.h, 0.5)
			m.asciiMode = true
			for _, r := range stripANSI(m.View()) {
				if r > unicode.MaxASCII && r != '\n' {
					t.Fatalf("%dx%d: %q (U+%04X) survived the fold", sz.w, sz.h, string(r), r)
				}
			}
		}
	})
}

// TestReplayViewV2Demo prints the ported screen at the design's own size for
// visual review; run with -v.
func TestReplayViewV2Demo(t *testing.T) {
	withColorProfile(t, termenv.Ascii, func() {
		m := replayModel(t, 160, 50, 0.75)
		t.Logf("replay inspector, 160x50:\n%s", m.View())
	})
}
