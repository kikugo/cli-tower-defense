package main

import (
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	tea "github.com/charmbracelet/bubbletea"

	eng "tower-defense/engine"
)

// viewSizes are the terminal sizes every view-level test sweeps: one per
// layout mode, plus the boundaries either side of each mode switch and the
// zero size Bubble Tea reports before its first WindowSizeMsg.
var viewSizes = []struct{ w, h int }{
	{0, 0},
	{40, 10}, {59, 20}, {80, 14}, // notice
	{60, 15}, {70, 24}, {79, 30}, // compact
	{80, 24}, {83, 40}, // minimum
	{84, 24}, {99, 50}, // narrow
	{100, 30}, {144, 40}, // mid
	{145, 40}, {160, 50}, {204, 60}, // wide
}

// TestViewV2ExactFrameAtEverySize is the whole-view contract: exactly h rows
// of exactly w display columns, at every mode and every boundary, live and
// at game over, with colour on.
//
// Measured with lipgloss.Width rather than checkFits: since Phase 3 the
// board and feed carry ANSI, and frameDisplayWidth counts escape bytes as
// columns.
func TestViewV2ExactFrameAtEverySize(t *testing.T) {
	live := newScriptedGame(t, "o3", "gpt-4")

	over := newScriptedGame(t, "o3", "gpt-4")
	over.ResolveTimeout()
	if !over.GameOver {
		t.Fatal("setup: expected GameOver after ResolveTimeout")
	}

	withColorProfile(t, termenv.ANSI256, func() {
		for _, g := range map[string]*gameUnderTest{
			"live":      {game: live},
			"game_over": {game: over},
		} {
			for _, sz := range viewSizes {
				m := model{game: g.game, width: sz.w, height: sz.h, tickDur: defaultTickDurV2}
				out := m.ViewV2()
				rows := strings.Split(out, "\n")

				wantW, wantH := sz.w, sz.h
				if wantW == 0 || wantH == 0 {
					wantW, wantH = 80, 24
				}
				// Notice mode has its own shape and its own test
				// (TestTooSmallNoticeFits); it is not a full frame.
				if computeLayoutV2(wantW, wantH).mode == modeNotice {
					continue
				}

				if len(rows) != wantH {
					t.Fatalf("%dx%d: %d rows, want %d", sz.w, sz.h, len(rows), wantH)
				}
				for i, row := range rows {
					if got := lipgloss.Width(row); got != wantW {
						t.Fatalf("%dx%d row %d: %d columns, want %d: %q", sz.w, sz.h, i, got, wantW, row)
					}
				}
			}
		}
	})
}

// gameUnderTest exists only to give the map above stable value semantics;
// ranging over a map of pointers directly would be fine, but naming the
// cases makes a failure message say which one broke.
type gameUnderTest struct{ game *eng.Game }

// TestViewV2AsciiModeEmitsOnlyASCII is the --ascii flag end to end: with the
// mode on, no character anywhere in a rendered frame is outside ASCII, at
// every layout mode and at game over.
//
// The per-pane fold is already covered in render_theme_v2_test.go; what this
// adds is that the fold is actually REACHED from the flag, for every pane
// the view composes, including the game-over overlay.
func TestViewV2AsciiModeEmitsOnlyASCII(t *testing.T) {
	g := newScriptedGame(t, "o3", "gpt-4")
	g.ResolveTimeout()

	withColorProfile(t, termenv.ANSI256, func() {
		for _, sz := range viewSizes {
			if sz.w == 0 {
				continue
			}
			if computeLayoutV2(sz.w, sz.h).mode == modeNotice {
				continue
			}
			m := model{game: g, width: sz.w, height: sz.h, tickDur: defaultTickDurV2, asciiMode: true}
			for _, r := range stripANSI(m.ViewV2()) {
				if r > unicode.MaxASCII && r != '\n' {
					t.Fatalf("%dx%d: --ascii output contains %q (U+%04X)", sz.w, sz.h, string(r), r)
				}
			}
		}
	})
}

// TestViewV2ZeroSizeNormalizes pins the behaviour computeLayout had and the
// first cutover dropped: a zero terminal size means Bubble Tea has not sent
// a WindowSizeMsg yet, and the first frame should be drawn at the standard
// default rather than saying the terminal is too small.
func TestViewV2ZeroSizeNormalizes(t *testing.T) {
	g := newScriptedGame(t, "o3", "gpt-4")
	m := model{game: g, width: 0, height: 0, tickDur: defaultTickDurV2}
	rows := strings.Split(m.ViewV2(), "\n")
	if len(rows) != 24 {
		t.Fatalf("zero size rendered %d rows, want 24", len(rows))
	}
	if strings.Contains(rows[0], "too small") {
		t.Fatalf("zero size rendered the too-small notice: %q", rows[0])
	}
}

// TestViewV2PanesDoNotBleed checks the blit composition: wide mode's
// vertical rule must appear at exactly boardMaxW on every body row, which is
// the single column that proves the left column did not overrun into the
// right one or vice versa.
func TestViewV2PanesDoNotBleed(t *testing.T) {
	g := newScriptedGame(t, "o3", "gpt-4")
	m := model{game: g, width: 160, height: 50, tickDur: defaultTickDurV2}

	l := computeLayoutV2(160, 50)
	if l.mode != modeWide {
		t.Fatalf("expected wide mode, got %v", l.mode)
	}

	rows := strings.Split(m.ViewV2(), "\n")
	for i := l.rule.y; i < l.rule.y+l.rule.h; i++ {
		plain := []rune(stripANSI(rows[i]))
		if len(plain) <= boardMaxW {
			t.Fatalf("row %d is only %d columns", i, len(plain))
		}
		if plain[boardMaxW] != '│' {
			t.Fatalf("row %d column %d is %q, want the column rule -- a pane bled across the split:\n%q",
				i, boardMaxW, string(plain[boardMaxW]), stripANSI(rows[i]))
		}
	}
}

// TestReplayViewAsciiModeEmitsOnlyASCII: --ascii is a statement about the
// terminal, not about one screen. The replay inspector keeps the old layout
// (see board_render.go), but a terminal that cannot draw a box character
// cannot draw one here either -- and this screen draws plenty of them.
func TestReplayViewAsciiModeEmitsOnlyASCII(t *testing.T) {
	events := replayFixtureEvents(t)

	withColorProfile(t, termenv.ANSI256, func() {
		for _, sz := range []struct{ w, h int }{{80, 24}, {100, 30}, {120, 40}, {160, 50}} {
			m := model{
				replayMode: true, replay: events, replayIdx: len(events) / 2,
				width: sz.w, height: sz.h, asciiMode: true,
			}
			out := m.View()
			for _, r := range stripANSI(out) {
				if r > unicode.MaxASCII && r != '\n' {
					t.Fatalf("%dx%d: replay --ascii output contains %q (U+%04X)", sz.w, sz.h, string(r), r)
				}
			}

			// And with the flag OFF it must still be the Unicode design --
			// otherwise this test would pass against a renderer that had
			// simply lost its box characters entirely.
			plain := model{
				replayMode: true, replay: events, replayIdx: len(events) / 2,
				width: sz.w, height: sz.h,
			}.View()
			if !strings.ContainsAny(plain, "┌│└") {
				t.Fatalf("%dx%d: replay view draws no box characters even without --ascii", sz.w, sz.h)
			}
		}
	})
}

// replayFixtureEvents builds a short event stream with a map, a tower and a
// breach -- enough that the reconstructed board draws something.
func replayFixtureEvents(t *testing.T) []eng.ReplayEvent {
	t.Helper()
	g := newScriptedGame(t, "o3", "gpt-4")
	if len(g.ReplayEvents) == 0 {
		t.Fatal("setup: the scripted game recorded no replay events")
	}
	return g.ReplayEvents
}

// TestAdvertisedKeysAreHandled: every key the UI names in a key bar or a
// pane border must have a case in Update. This is the guard for the defect
// that prompted it -- the board border said "? legend" and the wide legend's
// title row said "? toggles" from the day the redesign landed, and nothing
// handled '?' at all, so the UI advertised a control that did nothing.
func TestAdvertisedKeysAreHandled(t *testing.T) {
	// The keys the UI names to a user. Not an exhaustive list of handled
	// keys -- aliases like k/j/l/h/arrows exist and need no advertisement.
	advertised := []string{"q", "space", "+", "-", "a", "r", "L", "?", "n", "b", "[", "]", "g", "G", "e"}

	g := newScriptedGame(t, "o3", "gpt-4")
	for _, key := range advertised {
		before := model{game: g, width: 160, height: 50, tickDur: defaultTickDurV2}
		after, _ := before.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune(key)}))
		if key == "space" {
			after, _ = before.Update(tea.KeyMsg(tea.Key{Type: tea.KeySpace}))
		}
		if _, ok := after.(model); !ok {
			t.Fatalf("key %q: Update returned a non-model", key)
		}
	}

	// '?' specifically must CHANGE something -- a handled-but-inert key is
	// the same lie as an unhandled one.
	m := model{game: g, width: 160, height: 50, tickDur: defaultTickDurV2}
	updated, _ := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("?")}))
	toggled, ok := updated.(model)
	if !ok {
		t.Fatal("'?' did not return a model")
	}
	if toggled.hideLegend == m.hideLegend {
		t.Fatal("'?' did not toggle the legend")
	}
}

// TestLegendToggleGivesItsRowsToTheFeed: in wide mode the legend is pinned
// to the bottom of the right column, so hiding it must hand those rows to
// the feed. Leaving them blank would make '?' look like it only shrank the
// screen.
func TestLegendToggleGivesItsRowsToTheFeed(t *testing.T) {
	l := computeLayoutV2(160, 50)
	if l.mode != modeWide || l.legend.area() <= 0 {
		t.Fatalf("expected wide mode with a legend, got mode=%v legend=%+v", l.mode, l.legend)
	}

	shown := feedRectWithLegendHidden(l, false)
	hidden := feedRectWithLegendHidden(l, true)

	if shown != l.feed {
		t.Fatalf("with the legend shown the feed rect changed: %+v vs %+v", shown, l.feed)
	}
	if hidden.h != l.feed.h+l.gap.h+l.legend.h {
		t.Fatalf("hidden feed height %d, want %d (feed %d + gap %d + legend %d)",
			hidden.h, l.feed.h+l.gap.h+l.legend.h, l.feed.h, l.gap.h, l.legend.h)
	}

	// And the frame must still be exact with the legend off.
	g := newScriptedGame(t, "o3", "gpt-4")
	m := model{game: g, width: 160, height: 50, tickDur: defaultTickDurV2, hideLegend: true}
	rows := strings.Split(m.ViewV2(), "\n")
	if len(rows) != 50 {
		t.Fatalf("legend hidden: %d rows, want 50", len(rows))
	}
	for i, row := range rows {
		if w := lipgloss.Width(row); w != 160 {
			t.Fatalf("legend hidden, row %d: %d columns, want 160", i, w)
		}
	}
	if strings.Contains(stripANSI(strings.Join(rows, "\n")), "basic tower") {
		t.Fatal("the legend is still drawn with hideLegend set")
	}
}
