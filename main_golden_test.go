package main

// T2.6's golden snapshot: a full View() render at 160x50, ANSI stripped,
// checked against testdata/view_160x50.golden. This catches unintended
// visual/layout drift in a way the fit-invariant tests can't -- those only
// check dimensions, not content or arrangement.
//
// The golden game is built the same way as main_view_test.go's
// newScriptedGame (offline scripted providers, seeded rng, PauseBetweenTurns
// disabled, zero AI decision interval), which is fully deterministic: no
// wall-clock-dependent branch affects the simulation when the decision
// interval is 0, so re-running the exact same tick loop against the exact
// same seed reproduces byte-identical output every time.

import (
	"os"
	"regexp"
	"testing"
	"time"
)

// ansiStripRE strips SGR escape sequences (the only kind lipgloss emits in
// this codebase, for Foreground/Bold/Underline styling) so the golden file
// is plain text and diffs cleanly in a normal editor/terminal.
var ansiStripRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func TestViewGoldenSnapshot160x50(t *testing.T) {
	g := newScriptedGame(t, "o3", "gpt-4")
	// No maxTicks: the interactive view reports no tick cap because the
	// interactive Update loop enforces none (model.tickHorizon), so setting
	// one here would pin a value the view deliberately ignores.
	m := model{game: g, tickDur: 100 * time.Millisecond, width: 160, height: 50}

	got := ansiStripRE.ReplaceAllString(m.View(), "")

	want, err := os.ReadFile("testdata/view_160x50.golden")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	if got != string(want) {
		t.Fatalf("View() at 160x50 (ANSI stripped) does not match testdata/view_160x50.golden.\n--- got ---\n%s\n--- want ---\n%s", got, string(want))
	}
}
