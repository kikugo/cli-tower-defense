package main

// Tests for view_render.go: T2.3 (stats pane row-count independence from
// model-name length), T2.4 (move feed exact-budget rendering), and T2.5
// (status/key bar exactly one row at every tested width).

import (
	"fmt"
	"testing"
	"time"

	eng "tower-defense/engine"

	"github.com/charmbracelet/lipgloss"
)

// --- T2.3: stats pane -------------------------------------------------

// TestStatsRowCountIndependentOfModelName is the regression test for the
// 43->58 row swing the task brief measures between "gpt-4o-mini" and
// "qwen/qwen3-next-80b-a3b-instruct": rendering the stats pane with all five
// real model names this project configures must produce IDENTICAL row
// counts, for both the normal (5-row) and cramped (3-row) forms.
func TestStatsRowCountIndependentOfModelName(t *testing.T) {
	names := []string{"o3", "gpt-4o-mini", "gemini-2.5-flash", "gemini-3-flash-preview", "qwen/qwen3-next-80b-a3b-instruct"}

	for _, rc := range []rect{{w: 84, h: 5}, {w: 84, h: 3}, {w: 60, h: 5}} {
		var firstLen int
		for i, name := range names {
			g := eng.NewGame("", "")
			g.ModelNames[g.Defender] = name
			g.ModelNames[g.Attacker] = "gpt-4o-mini"
			rows := renderStats(g, rc)
			if i == 0 {
				firstLen = len(rows)
			}
			if len(rows) != rc.h {
				t.Fatalf("rc=%+v name=%q: got %d rows, want exactly rc.h=%d", rc, name, len(rows), rc.h)
			}
			if len(rows) != firstLen {
				t.Fatalf("rc=%+v name=%q: row count %d differs from first name's %d", rc, name, len(rows), firstLen)
			}
			for _, row := range rows {
				if w := lipgloss.Width(row); w > rc.w {
					t.Fatalf("rc=%+v name=%q: row width %d exceeds %d", rc, name, w, rc.w)
				}
			}
		}
	}
}

func TestStatsRowCountExactBudgetAtZeroAndNegativeHeight(t *testing.T) {
	g := eng.NewGame("", "")
	for _, h := range []int{0, -1} {
		rows := renderStats(g, rect{w: 84, h: h})
		if len(rows) != 0 {
			t.Fatalf("h=%d: got %d rows, want 0", h, len(rows))
		}
	}
}

// --- T2.4: move feed ----------------------------------------------------

func sampleReplayFeed(n int) []eng.ReplayEvent {
	events := make([]eng.ReplayEvent, n)
	for i := range events {
		role := "defender"
		if i%2 == 1 {
			role = "attacker"
		}
		events[i] = eng.ReplayEvent{
			Tick:     int64(i),
			Type:     eng.ReplayDecision,
			PlayerID: "p1",
			Role:     role,
			Action:   "place",
			Position: &eng.Position{Y: i % 14, X: i % 80},
		}
	}
	return events
}

// TestRenderMoveFeedExactBudget is T2.4's test: exactly budget rows for feed
// lengths 0/1/5/1000, and every row within the pane width.
func TestRenderMoveFeedExactBudget(t *testing.T) {
	for _, feedLen := range []int{0, 1, 5, 1000} {
		feed := sampleReplayFeed(feedLen)
		for _, budget := range []int{0, 1, 3, 10, 40} {
			for _, width := range []int{20, 40, 84} {
				rows := renderMoveFeed(feed, width, budget)
				if len(rows) != budget {
					t.Fatalf("feedLen=%d budget=%d width=%d: got %d rows, want %d", feedLen, budget, width, len(rows), budget)
				}
				for i, row := range rows {
					if w := lipgloss.Width(row); w > width {
						t.Fatalf("feedLen=%d budget=%d width=%d row %d: width %d exceeds %d (%q)", feedLen, budget, width, i, w, width, row)
					}
				}
			}
		}
	}
}

// TestRenderMoveFeedNewestAtBottom checks ordering: with more events than
// budget, the LAST (highest tick) events must be the ones shown, in order,
// with the newest at the bottom row.
func TestRenderMoveFeedNewestAtBottom(t *testing.T) {
	feed := sampleReplayFeed(20)
	rows := renderMoveFeed(feed, 40, 3)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	// Ticks 17, 18, 19 should appear in that order across the 3 rows.
	for i, wantTick := range []int{17, 18, 19} {
		want := fmt.Sprintf("%5d", wantTick)
		if len(rows[i]) < 5 || rows[i][:5] != want {
			t.Fatalf("row %d = %q, want it to start with tick %d", i, rows[i], wantTick)
		}
	}
}

// TestBuildMoveFeedFiltersNonMoveEvents checks the tick/map_init exclusion
// documented on isMoveFeedEvent: a raw periodic tick snapshot and the
// one-time map layout record are not "moves" and must not appear in the
// feed (map_init's Details field is what balloons to 396 rows if ever
// rendered raw -- it must never reach the feed).
func TestBuildMoveFeedFiltersNonMoveEvents(t *testing.T) {
	events := []eng.ReplayEvent{
		{Type: eng.ReplayMapInit, Details: map[string]interface{}{"paths": "big"}},
		{Type: eng.ReplayTick},
		{Type: eng.ReplayDecision, Role: "defender", Action: "place"},
		{Type: eng.ReplayRejected, Role: "attacker", Action: "spawn"},
	}
	feed := buildMoveFeed(events)
	if len(feed) != 2 {
		t.Fatalf("got %d feed events, want 2 (decision + rejected); feed=%+v", len(feed), feed)
	}
	for _, ev := range feed {
		if ev.Type == eng.ReplayMapInit || ev.Type == eng.ReplayTick {
			t.Fatalf("feed retained a non-move event: %+v", ev)
		}
	}
}

// TestBuildMoveFeedIncludesEngineAssist confirms ReplayEngineAssist rows
// (the engine acting on the attacker's behalf via applyAdaptivePressure --
// see engine/assist.go) reach the move feed like any other move, rather
// than being silently dropped the way ReplayTick/ReplayMapInit are.
func TestBuildMoveFeedIncludesEngineAssist(t *testing.T) {
	events := []eng.ReplayEvent{
		{Type: eng.ReplayTick},
		{Type: eng.ReplayEngineAssist, PlayerID: "p2", Role: "attacker", Action: "assist_auto_wave", Reason: "auto_wave"},
	}
	feed := buildMoveFeed(events)
	if len(feed) != 1 {
		t.Fatalf("got %d feed events, want 1 (engine assist); feed=%+v", len(feed), feed)
	}
	if feed[0].Type != eng.ReplayEngineAssist {
		t.Fatalf("expected the surviving feed event to be ReplayEngineAssist, got %q", feed[0].Type)
	}
}

// --- T2.5: status bar and key bar ----------------------------------------

func TestStatusAndKeyBarExactlyOneRowAtEveryWidth(t *testing.T) {
	g := eng.NewGame("", "")
	for w := 40; w <= 250; w += 7 {
		status := padCells(renderStatusText(g, 100*time.Millisecond, false), w)
		if h := lipgloss.Height(status); h != 1 {
			t.Fatalf("w=%d: status bar height %d, want 1", w, h)
		}
		if got := lipgloss.Width(status); got != w {
			t.Fatalf("w=%d: status bar width %d, want exactly %d", w, got, w)
		}

		key := padCells(renderKeyText(false, true, false), w)
		if h := lipgloss.Height(key); h != 1 {
			t.Fatalf("w=%d: key bar height %d, want 1", w, h)
		}
		if got := lipgloss.Width(key); got != w {
			t.Fatalf("w=%d: key bar width %d, want exactly %d", w, got, w)
		}
	}
}
