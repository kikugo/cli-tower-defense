package main

import (
	"strings"
	"testing"

	eng "tower-defense/engine"
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

func TestLayoutForSize(t *testing.T) {
	cases := []struct {
		width int
		want  layoutMode
	}{
		{0, layoutWide}, // size unknown before first WindowSizeMsg
		{200, layoutWide},
		{119, layoutStacked},
		{84, layoutStacked},
		{83, layoutTooSmall},
	}
	for _, c := range cases {
		if got := layoutForSize(c.width); got != c.want {
			t.Fatalf("layoutForSize(%d)=%v want %v", c.width, got, c.want)
		}
	}
}

func TestVisibleLogCount(t *testing.T) {
	if got := visibleLogCount(0); got != 10 {
		t.Fatalf("unknown height should default to 10, got %d", got)
	}
	if got := visibleLogCount(24); got != 3 {
		t.Fatalf("short terminal should clamp to 3, got %d", got)
	}
	if got := visibleLogCount(100); got != 15 {
		t.Fatalf("tall terminal should clamp to 15, got %d", got)
	}
	if got := visibleLogCount(38); got != 8 {
		t.Fatalf("expected 8 for height 38, got %d", got)
	}
}

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
