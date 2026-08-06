package main

import (
	"fmt"
	"strings"
	"testing"

	eng "tower-defense/engine"
)

// sampleWaves builds n complete waves plus one still in progress, with a
// life lost on every third wave -- enough structure that the banding,
// lives-column and result-column assertions below have something real to
// check rather than a run of identical rows.
func sampleWaves(n int) []eng.WaveSummary {
	out := make([]eng.WaveSummary, 0, n)
	lives := 10
	for i := 1; i <= n; i++ {
		lost := 0
		if i%3 == 0 && lives > 0 {
			lost = 1
		}
		ws := eng.WaveSummary{
			Wave: i, Sent: 10 + i, Leaked: lost, Killed: 9 + i,
			LivesLost: lost, Towers: 3 + i/3,
			LivesStart: lives, LivesEnd: lives - lost,
			Complete: i < n,
		}
		lives -= lost
		out = append(out, ws)
	}
	return out
}

func sampleTimelineData() TimelineData {
	return TimelineData{
		Waves:         sampleWaves(12),
		Wave:          12,
		MaxWave:       30,
		Tick:          117,
		MaxTick:       400,
		StartingLives: 10,
		Lives:         7,
		DefAuthored:   AuthoredShare{Known: true, HasData: true, Share: 0.84},
		AttAuthored:   AuthoredShare{Known: true, HasData: true, Share: 0.91},
		Substituted:   0, SubstitutedKnown: true,
		Assist: TrustState{
			AssistKnown: true, AssistsEnabled: true, AssistCount: 2,
			AssistDetail: "1 unrecorded", ProvenanceKnown: true,
		},
		RejectedDef: 38, RejectedAtt: 0,
		RejectedDefReason: "unaffordable placements",
		Ruleset:           "balance v3 a3f1c8   30 waves   assists on   pricing unset",
	}
}

// TestRenderTimelineV2FitsAtEverySize is the shared pane contract: exactly
// rc.h rows of exactly rc.w display columns, at every height the layout can
// hand it and every width a future mode might.
func TestRenderTimelineV2FitsAtEverySize(t *testing.T) {
	d := sampleTimelineData()
	for _, w := range []int{boardMaxW, 100, 60, 40, 24} {
		for h := 1; h <= 24; h++ {
			rows := RenderTimelineV2(rect{w: w, h: h}, d)
			if len(rows) != h {
				t.Fatalf("w=%d h=%d: got %d rows, want %d", w, h, len(rows), h)
			}
			if err := checkFits(strings.Join(rows, "\n"), w, h); err != nil {
				t.Fatalf("w=%d h=%d: %v", w, h, err)
			}
		}
	}
}

// TestBuildWaveTableRowsCoversEveryWave is the property the banding exists
// to preserve: however small the budget, the rendered table still accounts
// for every wave -- the sent counts sum to the same total, and the first
// row's lives value is still the match's starting lives. A table that
// dropped the oldest waves would fail both.
func TestBuildWaveTableRowsCoversEveryWave(t *testing.T) {
	waves := sampleWaves(12)
	totalSent := 0
	for _, ws := range waves {
		totalSent += ws.Sent
	}

	for budget := 1; budget <= 15; budget++ {
		rows := buildWaveTableRows(waves, budget)
		if len(rows) > budget {
			t.Fatalf("budget=%d: got %d rows", budget, len(rows))
		}
		if len(rows) == 0 {
			t.Fatalf("budget=%d: got no rows for 12 waves", budget)
		}

		sum := 0
		for _, row := range rows {
			var label string
			var sent, leaked, killed, towers int
			var lives, result string
			// The row format is fixed by timelineRowText; parsing it back is
			// what makes this a real conservation check rather than a
			// re-implementation of the same arithmetic.
			if _, err := fmt.Sscan(row, &label, &sent, &leaked, &killed, &towers, &lives, &result); err != nil {
				t.Fatalf("budget=%d: could not parse row %q: %v", budget, row, err)
			}
			sum += sent
		}
		if sum != totalSent {
			t.Fatalf("budget=%d: rows account for %d sent enemies, want %d (waves were dropped, not banded)",
				budget, sum, totalSent)
		}
		if !strings.Contains(rows[0], "10->") {
			t.Fatalf("budget=%d: first row %q does not start from the match's starting lives", budget, rows[0])
		}
	}
}

// TestBuildWaveTableRowsBandsOnlyWhenNeeded checks the table renders every
// wave individually while they fit, and only collapses into a "first-last"
// band row once they don't.
func TestBuildWaveTableRowsBandsOnlyWhenNeeded(t *testing.T) {
	waves := sampleWaves(5)

	fits := buildWaveTableRows(waves, 5)
	if len(fits) != 5 {
		t.Fatalf("5 waves in a budget of 5: got %d rows", len(fits))
	}
	for _, row := range fits {
		if strings.Contains(row, "-") && !strings.Contains(row, "->") {
			t.Fatalf("banded a table that already fitted: %q", row)
		}
	}

	squeezed := buildWaveTableRows(waves, 3)
	if len(squeezed) != 3 {
		t.Fatalf("5 waves in a budget of 3: got %d rows", len(squeezed))
	}
	if !strings.Contains(squeezed[0], "1-3") {
		t.Fatalf("expected a 1-3 band row, got %q", squeezed[0])
	}
}

// TestBuildWaveTableRowsEmptyAndDegenerate covers the boundaries: no waves,
// and non-positive budgets.
func TestBuildWaveTableRowsEmptyAndDegenerate(t *testing.T) {
	if rows := buildWaveTableRows(nil, 5); rows != nil {
		t.Fatalf("no waves: got %v", rows)
	}
	for _, budget := range []int{0, -1, -10} {
		if rows := buildWaveTableRows(sampleWaves(4), budget); rows != nil {
			t.Fatalf("budget=%d: got %v", budget, rows)
		}
	}
}

// TestWaveResultNamesTheOutcome pins the three result words, including that
// a wave which ran lives to zero is shouted rather than reported as "held".
func TestWaveResultNamesTheOutcome(t *testing.T) {
	cases := []struct {
		ws   eng.WaveSummary
		want string
	}{
		{eng.WaveSummary{Complete: false, LivesEnd: 7}, "in progress"},
		{eng.WaveSummary{Complete: true, LivesEnd: 7}, "held"},
		{eng.WaveSummary{Complete: true, LivesEnd: 0}, "CORE LOST"},
		// An in-progress wave that has already taken the last life still
		// reads "in progress": the wave is not closed to arrivals yet, and
		// the game-over card is what announces the end, not this column.
		{eng.WaveSummary{Complete: false, LivesEnd: 0}, "in progress"},
	}
	for _, c := range cases {
		if got := waveResult(c.ws); got != c.want {
			t.Fatalf("%+v: got %q, want %q", c.ws, got, c.want)
		}
	}
}

// TestRenderTimelineV2NeverStarvesTheSummary checks the row-budget policy:
// the whole-match summary rows are facts about the match and must survive a
// squeeze, so the table is what shrinks. At a height that fits the summary
// and nothing else, the ruleset stamp must still be on screen.
func TestRenderTimelineV2NeverStarvesTheSummary(t *testing.T) {
	d := sampleTimelineData()
	out := strings.Join(RenderTimelineV2(rect{w: boardMaxW, h: timelineFixedRows}, d), "\n")
	if !strings.Contains(out, "balance v3") {
		t.Fatalf("summary block was starved at the minimum height:\n%s", out)
	}
	if !strings.Contains(out, "MATCH TIMELINE") {
		t.Fatalf("title rule was starved at the minimum height:\n%s", out)
	}
}

// TestRenderTimelineV2UnknownsRenderAsWords is the same honesty contract the
// cards pane has: an untracked substitution count and an unmeasured
// provenance state must not render as zeros.
func TestRenderTimelineV2UnknownsRenderAsWords(t *testing.T) {
	d := sampleTimelineData()
	d.SubstitutedKnown = false
	d.Substituted = 0
	d.Assist = TrustState{AssistKnown: false, ProvenanceKnown: false}
	d.DefAuthored = AuthoredShare{}
	d.AttAuthored = AuthoredShare{}

	out := strings.Join(RenderTimelineV2(rect{w: boardMaxW, h: 13}, d), "\n")
	for _, want := range []string{"substitutions not tracked", "ENGINE ASSIST UNKNOWN", "provenance UNKNOWN", "unknown"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in unknown-state timeline:\n%s", want, out)
		}
	}
	if strings.Contains(out, "substituted 0 decisions") {
		t.Fatalf("untracked substitutions rendered as a measured zero:\n%s", out)
	}
}

// TestRenderTimelineV2NoWavesYet covers the pre-first-wave state: the table
// says so rather than rendering an empty column header with nothing under
// it, and the lives bar is still correct because it reads StartingLives/
// Lives rather than the (empty) wave list.
func TestRenderTimelineV2NoWavesYet(t *testing.T) {
	d := sampleTimelineData()
	d.Waves = nil
	d.Wave = 0
	d.Lives = 10

	out := strings.Join(RenderTimelineV2(rect{w: boardMaxW, h: 13}, d), "\n")
	if !strings.Contains(out, "no waves have started yet") {
		t.Fatalf("empty wave list did not say so:\n%s", out)
	}
	if !strings.Contains(out, "10 -> 10") {
		t.Fatalf("lives bar wrong before the first wave:\n%s", out)
	}
}

// TestRenderTimelineV2Demo prints the pane at its fixture size for visual
// review against testdata/mockups/160x50.txt lines 37-49.
func TestRenderTimelineV2Demo(t *testing.T) {
	rows := RenderTimelineV2(rect{w: boardMaxW, h: 13}, sampleTimelineData())
	t.Logf("timeline pane, %dx13:\n%s", boardMaxW, strings.Join(rows, "\n"))
}
