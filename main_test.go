package main

import (
	"strings"
	"testing"
	"time"

	eng "tower-defense/engine"

	tea "github.com/charmbracelet/bubbletea"
)

func TestClampReplayIdx(t *testing.T) {
	cases := []struct {
		idx, total, want int
	}{
		{-3, 10, 0},
		{5, 10, 5},
		{20, 10, 9},
		{0, 0, 0},
		{4, 1, 0},
	}
	for _, c := range cases {
		if got := clampReplayIdx(c.idx, c.total); got != c.want {
			t.Fatalf("clampReplayIdx(%d,%d)=%d want %d", c.idx, c.total, got, c.want)
		}
	}
}

func TestReplayGameEndIndex(t *testing.T) {
	events := []eng.ReplayEvent{
		{Type: eng.ReplayTick},
		{Type: eng.ReplaySpawn},
		{Type: eng.ReplayGameEnd},
		{Type: eng.ReplayTick},
	}
	if got := replayGameEndIndex(events, 0); got != 2 {
		t.Fatalf("expected game end at index 2, got %d", got)
	}

	noEnd := []eng.ReplayEvent{{Type: eng.ReplayTick}, {Type: eng.ReplaySpawn}}
	if got := replayGameEndIndex(noEnd, 1); got != 1 {
		t.Fatalf("expected fallback index 1, got %d", got)
	}
}

func TestApplyBalanceOverride(t *testing.T) {
	base := eng.DefaultBalanceConfig()
	origBasic := base.Towers["basic"]
	origBounty := base.BreachResourceBounty
	bounty := 0
	out := applyBalanceOverride(base, balanceOverride{
		Towers:               map[string]eng.TowerStat{"basic": {Damage: 25, Range: 5, Cooldown: 3, Cost: 100}},
		BreachResourceBounty: &bounty,
	})
	if out.Towers["basic"].Damage != 25 || out.Towers["basic"].Cooldown != 3 {
		t.Fatalf("expected basic tower override, got %+v", out.Towers["basic"])
	}
	if out.BreachResourceBounty != 0 {
		t.Fatalf("expected bounty override 0, got %d", out.BreachResourceBounty)
	}
	// base must not be mutated (compare against captured pre-call values,
	// not literals, so the test survives default retuning)
	if base.Towers["basic"] != origBasic || base.BreachResourceBounty != origBounty {
		t.Fatalf("override mutated the base config: %+v", base.Towers["basic"])
	}
	if out.Enemies["tank"].Health != base.Enemies["tank"].Health {
		t.Fatalf("untouched enemies must carry over")
	}
}

// TestLayoutForSize and TestVisibleLogCount were deleted here: the
// layout-engine rewrite removed layoutForSize, visibleLogCount and
// sidebarStyle outright. The layout's own property tests live in
// main_layout_v2_test.go, which walks every cell of every frame size.
func TestWaveProgressBar(t *testing.T) {
	got := waveProgressBar(3, 10, 10)
	if got != "Wave 3/10 [███───────]" {
		t.Fatalf("got %q", got)
	}
	if got := waveProgressBar(12, 10, 10); got != "Wave 12/10 [██████████]" {
		t.Fatalf("overflow should clamp, got %q", got)
	}
	if got := waveProgressBar(2, 0, 10); got != "Wave 2" {
		t.Fatalf("zero max should degrade, got %q", got)
	}
}

func TestFmtCostMicros(t *testing.T) {
	if got := fmtCostMicros(0); got != "-" {
		t.Fatalf("expected - for zero cost, got %q", got)
	}
	if got := fmtCostMicros(-5); got != "-" {
		t.Fatalf("expected - for negative cost, got %q", got)
	}
	if got := fmtCostMicros(8000); got != "$0.0080" {
		t.Fatalf("expected $0.0080, got %q", got)
	}
}

// TestSpeedControlScalesTurnPause proves the +/- speed control keeps
// eng.Game.PauseDuration in lockstep with tickDur (at ratio turnPauseRatio),
// so the between-turn pause set by switchTurn actually speeds up/slows down
// along with the tick rate instead of staying fixed at 1s regardless of
// speed.
func TestSpeedControlScalesTurnPause(t *testing.T) {
	g := eng.NewGame("", "")
	m := model{game: g, tickDur: 100 * time.Millisecond}
	m.syncPauseDuration()
	if want := m.tickDur * turnPauseRatio; g.PauseDuration != want {
		t.Fatalf("initial sync: PauseDuration = %v, want %v", g.PauseDuration, want)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("+")})
	m2, ok := updated.(model)
	if !ok {
		t.Fatalf("Update did not return a model")
	}
	if want := m2.tickDur * turnPauseRatio; g.PauseDuration != want {
		t.Fatalf("after '+': PauseDuration = %v, want %v (tickDur=%v)", g.PauseDuration, want, m2.tickDur)
	}

	updated, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-")})
	m3, ok := updated.(model)
	if !ok {
		t.Fatalf("Update did not return a model")
	}
	if want := m3.tickDur * turnPauseRatio; g.PauseDuration != want {
		t.Fatalf("after '-': PauseDuration = %v, want %v (tickDur=%v)", g.PauseDuration, want, m3.tickDur)
	}

	// Replay mode has m.game == nil; the +/- handlers must not panic.
	nilGameModel := model{game: nil, tickDur: 100 * time.Millisecond}
	updated, _ = nilGameModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("+")})
	if _, ok := updated.(model); !ok {
		t.Fatalf("'+' with nil game did not return a model")
	}
	updated, _ = nilGameModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-")})
	if _, ok := updated.(model); !ok {
		t.Fatalf("'-' with nil game did not return a model")
	}
}

func TestProgressBar(t *testing.T) {
	if got := progressBar(0, 1, 40); got != "" {
		t.Fatalf("expected empty bar for single event, got %q", got)
	}
	bar := progressBar(0, 10, 20)
	if !strings.HasPrefix(bar, "[|") {
		t.Fatalf("expected marker at start, got %q", bar)
	}
	endBar := progressBar(9, 10, 20)
	if !strings.HasSuffix(endBar, "|]") {
		t.Fatalf("expected marker at end, got %q", endBar)
	}
}
