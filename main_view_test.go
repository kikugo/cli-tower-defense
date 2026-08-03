package main

// These tests exist to document a known rendering defect: Bubble Tea 0.26.1
// does NOT scroll -- its standardRenderer.flush (charmbracelet/bubbletea
// v0.26.1 standard_renderer.go:167-169) keeps only the LAST `height` lines of
// a frame and (standard_renderer.go:244-246) truncates each surviving line to
// `width` columns. main.go's View() composes the game board (rendered first)
// followed by a sidebar and footer, with no awareness of the terminal size,
// so the sidebar/footer routinely push the board off the top of the clipped
// frame.
//
// Per the task that produced this file: these tests MUST FAIL on the current
// code. Nobody should "fix" them by relaxing the assertions -- a later change
// to non-test source is expected to make them pass.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	eng "tower-defense/engine"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

// renderAt renders the model's View() at a given terminal size, exactly as
// Bubble Tea would after delivering a tea.WindowSizeMsg{Width: w, Height: h}.
func renderAt(t *testing.T, g *eng.Game, w, h int) string {
	t.Helper()
	return model{game: g, tickDur: 100 * time.Millisecond, width: w, height: h}.View()
}

// clipToTerminal reproduces bubbletea standardRenderer.flush (v0.26.1
// standard_renderer.go:167-169 and :244-246): keep the last h lines of the
// frame, then truncate each surviving line to w columns. Bubble Tea does NOT
// scroll -- anything above the last h lines is discarded outright, and this
// is done with a plain string.Split/slice, not aware of box borders or
// semantic content.
func clipToTerminal(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if h > 0 && len(lines) > h {
		lines = lines[len(lines)-h:]
	}
	if w > 0 {
		for i, line := range lines {
			lines[i] = truncate.String(line, uint(w))
		}
	}
	return strings.Join(lines, "\n")
}

// boardMarkerRunes are glyphs that main.go's View() only ever writes into the
// game-board rune grid (see the switch in View() around the "·", "⬡", "^",
// "⊕", "⌖" cases). None of these appear in the sidebar/footer text, so their
// presence after clipping is a direct signal that at least part of the board
// survived Bubble Tea's clip-to-last-N-lines behavior. '·' (path waypoint,
// written for every path's first/last tile regardless of game state) and '⬡'
// (obstacle, present from game creation via generateObstacles) are the most
// reliable since they don't depend on the scripted AI actually building
// anything; the tower glyphs are a bonus signal once towers exist.
var boardMarkerRunes = []rune{'·', '⬡', '^', '⊕', '⌖'}

func containsBoardMarker(s string) (rune, bool) {
	for _, r := range boardMarkerRunes {
		if strings.ContainsRune(s, r) {
			return r, true
		}
	}
	return 0, false
}

// poolTail returns a copy of the last n entries of pool (or the whole pool,
// copied, if it has fewer than n entries).
func poolTail(pool []string, n int) []string {
	if n <= 0 {
		return []string{}
	}
	if n >= len(pool) {
		out := make([]string, len(pool))
		copy(out, pool)
		return out
	}
	out := make([]string, n)
	copy(out, pool[len(pool)-n:])
	return out
}

// gameStateBlockLog reproduces, verbatim, the format string engine/core.go:840
// passes to the unexported g.logf -- the periodic "=== Game State ===" block
// that HandleAIDecisions appends as a SINGLE g.Logs entry every 10 real
// wall-clock seconds of play (see core.go:839, `currentTime.Sub(
// g.lastStatePrintTime) > 10*time.Second`). Waiting 10 real seconds in a unit
// test to trigger it naturally is impractical, so this helper reproduces the
// exact format engine/core.go uses (read, not modified) to synthesize the
// same entry against a live *eng.Game's current fields. This entry alone
// costs ~12 rendered rows once word-wrapped into the 35-column sidebar.
func gameStateBlockLog(g *eng.Game) string {
	return fmt.Sprintf("\n=== Game State ===\nWave: %d\nCurrent Turn: %s (%s)\n%s (Def) - Lives: %d, Res: %d\n%s (Att) - Res: %d\nActive Towers: %d, Enemies: %d\n==================\n",
		g.Wave, g.CurrentTurn, g.ModelNames[g.CurrentTurn], g.ModelNames[g.Defender], g.Lives[g.Defender], g.Resources[g.Defender],
		g.ModelNames[g.Attacker], g.Resources[g.Attacker], len(g.Towers), len(g.Enemies))
}

// newScriptedGame builds a real, fully-playing *eng.Game using the offline
// scripted providers in engine/provider_scripted.go (the same
// defender_baseline/attacker_baseline/default-case behaviors
// engine/scripted_duel.go's RunScriptedDuel exercises). We can't call
// RunScriptedDuel itself here: it returns only an eng.MatchResult, not the
// *eng.Game, so it can't hand back g.Logs. This function replicates
// RunScriptedDuel's tick loop (read from engine/scripted_duel.go, which is
// owned by another agent right now and is not modified here) against the
// exported Game API so the test can inspect the resulting Logs, board state,
// and telemetry directly.
//
// p1Model/p2Model double as both the ScriptedProvider behavior key (only
// "defender_invest", "defender_baseline", and "attacker_spawn" are handled
// specially; anything else -- including real model names like "o3" or
// "qwen/qwen3-next-80b-a3b-instruct" -- hits a sensible default: defenders
// place at the first valid candidate, attackers spawn/wave off resources)
// and the display name shown in the sidebar, so the real row-cost-scales-
// with-model-name-length coupling described in the task is exercised
// end-to-end rather than patched on after the fact.
func newScriptedGame(t *testing.T, p1Model, p2Model string) *eng.Game {
	t.Helper()
	resolved := eng.ResolvedMatchConfig{
		Player1: eng.ResolvedPlayerModelConfig{PlayerModelConfig: eng.PlayerModelConfig{
			Provider: eng.ProviderScripted, Model: p1Model, APIKeyEnv: "NONE",
		}},
		Player2: eng.ResolvedPlayerModelConfig{PlayerModelConfig: eng.PlayerModelConfig{
			Provider: eng.ProviderScripted, Model: p2Model, APIKeyEnv: "NONE",
		}},
	}
	g := eng.NewGameFromResolvedConfig(resolved)
	g.Balance = eng.DefaultBalanceConfig()
	g.ApplyRuleset(eng.BaselineDuelRuleset())
	g.SetRandomSeed(7)
	g.PauseBetweenTurns = false
	g.AIDecisionInterval[g.Player1] = 0
	g.AIDecisionInterval[g.Player2] = 0

	// Stop as soon as Logs has saturated its MaxLogs cap (250; empirically
	// ~120 ticks for this seed/ruleset) rather than running to GameOver:
	// GameOver comes only ~55 ticks later for these unbalanced default-case
	// scripted behaviors (BaselineDuelRuleset, seed 7 -> defender loses
	// around tick 175), and TestBoardVisibleAfterClip specifically wants a
	// live, mid-match board (GameOver=false), not the post-game two-line
	// screen. len(g.Logs) plateaus at exactly g.MaxLogs once the cap is hit
	// (engine/core.go's logf trims it back down every call), so "< MaxLogs"
	// is the correct stop condition, not "< MaxLogs+slack".
	ticks := 0
	deadline := time.Now().Add(20 * time.Second)
	for ticks < 6000 && !g.GameOver && len(g.Logs) < g.MaxLogs && time.Now().Before(deadline) {
		if g.AIThinking[g.Player1] || g.AIThinking[g.Player2] {
			g.HandleAIDecisions()
			time.Sleep(10 * time.Microsecond)
			continue
		}
		g.UpdateGameState()
		g.HandleAIDecisions()
		ticks++
	}
	return g
}

// logVariant names one shape of the Logs slice used in the TestViewNeverExceedsTerminal matrix.
type logVariant struct {
	label string
	build func(pool []string, g *eng.Game) []string
}

var logVariants = []logVariant{
	{"logs0", func(pool []string, g *eng.Game) []string { return poolTail(pool, 0) }},
	{"logs1", func(pool []string, g *eng.Game) []string { return poolTail(pool, 1) }},
	{"logs20", func(pool []string, g *eng.Game) []string { return poolTail(pool, 20) }},
	{"logs250", func(pool []string, g *eng.Game) []string { return poolTail(pool, 250) }},
	// The last entry is the multi-line "=== Game State ===" block so it sits
	// in the visible log tail regardless of how small visibleLogCount(h) is.
	{"logs250_gamestate", func(pool []string, g *eng.Game) []string {
		base := poolTail(pool, 249)
		return append(base, gameStateBlockLog(g))
	}},
}

func sanitizeName(s string) string {
	return strings.ReplaceAll(s, "/", "_")
}

// TestViewNeverExceedsTerminal is the fit invariant: whatever View() produces
// must already fit inside the terminal Bubble Tea told us about, because
// Bubble Tea will not scroll it into view -- it clips. This is checked
// directly against View()'s raw output (not the clipped output), since a
// conforming renderer would never need to be clipped in the first place.
func TestViewNeverExceedsTerminal(t *testing.T) {
	sizes := []struct{ w, h int }{
		{0, 0}, // frame rendered before the first WindowSizeMsg; layoutForSize(0)==layoutWide
		{60, 15},
		{80, 24},
		{84, 24},
		{100, 30},
		{119, 40},
		{120, 40},
		{160, 50},
		{204, 60},
	}
	pairs := [][2]string{
		{"o3", "gpt-4"},
		{"qwen/qwen3-next-80b-a3b-instruct", "gemini-3-flash-preview"},
	}

	total := 0

	for _, pair := range pairs {
		pair := pair
		game := newScriptedGame(t, pair[0], pair[1])
		pool := append([]string{}, game.Logs...)
		t.Logf("pair %s/%s: scripted run produced %d Logs entries (engine MaxLogs=%d), Wave=%d Towers=%d Enemies=%d GameOver=%v",
			pair[0], pair[1], len(pool), game.MaxLogs, game.Wave, len(game.Towers), len(game.Enemies), game.GameOver)

		for _, variant := range logVariants {
			variant := variant
			game.Logs = variant.build(pool, game)
			logCount := len(game.Logs)

			for _, sz := range sizes {
				sz := sz
				total++
				name := fmt.Sprintf("%s/%s_%dx%d", sanitizeName(pair[0]+"-"+pair[1]), variant.label, sz.w, sz.h)
				t.Run(name, func(t *testing.T) {
					out := renderAt(t, game, sz.w, sz.h)
					gotH := lipgloss.Height(out)
					gotW := lipgloss.Width(out)
					if gotH > sz.h {
						t.Errorf("rendered height %d exceeds terminal height %d (w=%d h=%d logs=%d pair=%s/%s)",
							gotH, sz.h, sz.w, sz.h, logCount, pair[0], pair[1])
					}
					if gotW > sz.w {
						t.Errorf("rendered width %d exceeds terminal width %d (w=%d h=%d logs=%d pair=%s/%s)",
							gotW, sz.w, sz.w, sz.h, logCount, pair[0], pair[1])
					}
				})
			}
		}
	}
	t.Logf("matrix size: %d cases", total)
}

// TestBoardVisibleAfterClip asserts the actual user-facing requirement,
// independent of row-counting: after Bubble Tea's real clip-to-last-N-lines
// behavior is applied to a realistic frame (a scripted match with a deep log
// history, rendered at a normal terminal size), at least one glyph that only
// the board draws must still be present. This is the concrete symptom a
// player sees: the map scrolls off the top and never comes back.
func TestBoardVisibleAfterClip(t *testing.T) {
	game := newScriptedGame(t, "o3", "gpt-4")
	pool := append([]string{}, game.Logs...)
	t.Logf("scripted run produced %d Logs entries before shaping to ~250", len(pool))
	game.Logs = poolTail(pool, 250)

	w, h := 120, 40
	out := renderAt(t, game, w, h)
	rawHeight := lipgloss.Height(out)
	rawWidth := lipgloss.Width(out)

	if _, ok := containsBoardMarker(out); !ok {
		t.Fatalf("setup invariant violated: raw View() output before clipping contains no board marker at all (checked %q); the render itself is broken, not just the clip", string(boardMarkerRunes))
	}

	clipped := clipToTerminal(out, w, h)
	clippedHeight := lipgloss.Height(clipped)
	t.Logf("raw View() output: %d rows x %d cols for a %dx%d terminal; after bubbletea-style clipping: %d rows", rawHeight, rawWidth, w, h, clippedHeight)

	marker, ok := containsBoardMarker(clipped)
	if !ok {
		t.Errorf("no board marker (any of %q) survived clipping a %d-row frame to a %d-row terminal (w=%d h=%d, logs=%d); the board, rendered first in View(), was pushed entirely off-screen by the sidebar/logs beneath it",
			string(boardMarkerRunes), rawHeight, h, w, h, len(game.Logs))
	} else {
		t.Logf("board marker %q survived clipping", string(marker))
	}
}

// TestGameOverAndReplayFitInvariant applies the same fit invariant as
// TestViewNeverExceedsTerminal to two more View() paths the matrix above
// doesn't cover: the GameOver screen and replay mode at event 0.
func TestGameOverAndReplayFitInvariant(t *testing.T) {
	sizes := []struct{ w, h int }{
		{0, 0}, {60, 15}, {80, 24}, {84, 24}, {100, 30},
		{119, 40}, {120, 40}, {160, 50}, {204, 60},
	}

	// main.go:298-303 special-cases GameOver into a bare two-line string,
	// discarding the board and every statistic at the moment they matter
	// most. Applying the fit invariant here is expected to mostly PASS
	// (two short lines fit almost any terminal) -- the defect this
	// documents is loss of information, not overflow, and the {0,0} case
	// (width/height both zero) still fails the literal invariant.
	t.Run("game_over", func(t *testing.T) {
		game := newScriptedGame(t, "o3", "gpt-4")
		game.ResolveTimeout() // forces a decisive GameOver verdict, same as headless/tournament paths use
		if !game.GameOver {
			t.Fatalf("test setup failed: expected GameOver=true after ResolveTimeout()")
		}
		out0 := renderAt(t, game, 100, 30)
		t.Logf("GameOver view at 100x30: %d rows x %d cols -- %q", lipgloss.Height(out0), lipgloss.Width(out0), out0)

		for _, sz := range sizes {
			sz := sz
			t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
				out := renderAt(t, game, sz.w, sz.h)
				gotH, gotW := lipgloss.Height(out), lipgloss.Width(out)
				if gotH > sz.h {
					t.Errorf("rendered height %d exceeds terminal height %d", gotH, sz.h)
				}
				if gotW > sz.w {
					t.Errorf("rendered width %d exceeds terminal width %d", gotW, sz.w)
				}
			})
		}
	})

	// main.go:690-694 pretty-prints the map_init event's full paths array
	// via json.MarshalIndent, which puts each coordinate on its own line.
	// The task brief cites an external measurement of 396 rendered rows at
	// event 0; this sub-test measures the real number directly.
	t.Run("replay_event_zero", func(t *testing.T) {
		raw, err := os.ReadFile("live_fair_replay.json")
		if err != nil {
			t.Fatalf("read live_fair_replay.json: %v", err)
		}
		var events []eng.ReplayEvent
		if err := json.Unmarshal(raw, &events); err != nil {
			t.Fatalf("parse live_fair_replay.json: %v", err)
		}
		if len(events) == 0 {
			t.Fatalf("live_fair_replay.json has no events")
		}
		t.Logf("live_fair_replay.json: %d events, event 0 type=%q", len(events), events[0].Type)
		if events[0].Type != eng.ReplayMapInit {
			t.Logf("note: event 0 is not %q as the task brief assumed; measuring it as-is", eng.ReplayMapInit)
		}

		base := model{replayMode: true, replay: events, replayIdx: 0, tickDur: 100 * time.Millisecond}

		measure := base
		measure.width, measure.height = 120, 40
		measuredOut := measure.View()
		measuredHeight := lipgloss.Height(measuredOut)
		measuredWidth := lipgloss.Width(measuredOut)
		t.Logf("MEASURED: replay view at event 0, rendered at 120x40, is %d rows x %d cols (task brief's external measurement: 396 rows)", measuredHeight, measuredWidth)

		for _, sz := range sizes {
			sz := sz
			t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
				m := base
				m.width, m.height = sz.w, sz.h
				out := m.View()
				gotH, gotW := lipgloss.Height(out), lipgloss.Width(out)
				if gotH > sz.h {
					t.Errorf("rendered height %d exceeds terminal height %d (replay event 0, map_init details dump)", gotH, sz.h)
				}
				if gotW > sz.w {
					t.Errorf("rendered width %d exceeds terminal width %d", gotW, sz.w)
				}
			})
		}
	})
}
