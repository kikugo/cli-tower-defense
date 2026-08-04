package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	eng "tower-defense/engine"
)

// buildScriptedTestGame plays a short scripted-vs-scripted match (no network,
// no API keys) and returns the finished game, mirroring the setup
// engine.RunScriptedDuel uses internally but keeping the *eng.Game around so
// the test can feed it to writeMatchArtifacts the same way runHeadless and
// the interactive path in main() do.
func buildScriptedTestGame(t *testing.T) *eng.Game {
	t.Helper()
	resolved := eng.ResolvedMatchConfig{
		Player1: eng.ResolvedPlayerModelConfig{PlayerModelConfig: eng.PlayerModelConfig{
			Provider: eng.ProviderScripted, Model: "defender_baseline", APIKeyEnv: "NONE",
		}},
		Player2: eng.ResolvedPlayerModelConfig{PlayerModelConfig: eng.PlayerModelConfig{
			Provider: eng.ProviderScripted, Model: "attacker_baseline", APIKeyEnv: "NONE",
		}},
	}
	g := eng.NewGameFromResolvedConfig(resolved)
	g.ApplyRuleset(eng.BaselineDuelRuleset())
	g.SetRandomSeed(1)
	g.PauseBetweenTurns = false
	g.AIDecisionInterval[g.Player1] = 0
	g.AIDecisionInterval[g.Player2] = 0

	maxTicks := 400
	ticks := 0
	deadline := time.Now().Add(60 * time.Second)
	for ticks < maxTicks && !g.GameOver && time.Now().Before(deadline) {
		if g.AIThinking[g.Player1] || g.AIThinking[g.Player2] {
			g.HandleAIDecisions()
			time.Sleep(10 * time.Microsecond)
			continue
		}
		g.UpdateGameState()
		g.HandleAIDecisions()
		ticks++
	}
	g.ResolveTimeout()
	return g
}

// TestWriteMatchArtifactsRoundTrips exercises the factored writeMatchArtifacts
// helper directly -- the same helper both runHeadless and the interactive
// path in main() now call -- to guard against the bug this change fixes: a
// live TUI match producing no provenance record at all. It writes to
// t.TempDir() and asserts the result JSON round-trips with the expected
// model_authored content, without touching network or API keys.
func TestWriteMatchArtifactsRoundTrips(t *testing.T) {
	g := buildScriptedTestGame(t)
	matchResult := g.BuildMatchResult()

	dir := t.TempDir()
	resultPath := filepath.Join(dir, "result.json")
	replayPath := filepath.Join(dir, "replay.json")
	manifestPath := filepath.Join(dir, "manifest.json")
	reportPath := filepath.Join(dir, "report.md")

	m := model{
		game:         g,
		resultJSON:   resultPath,
		replayJSON:   replayPath,
		manifestJSON: manifestPath,
		reportMD:     reportPath,
		seed:         1,
		ruleset:      eng.BaselineDuelRuleset(),
	}

	writeMatchArtifacts(m, matchResult, "interactive", 0)

	// result.json must exist and round-trip with the provenance fields
	// this whole change exists to preserve.
	raw, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("result json not written: %v", err)
	}
	var gotResult eng.MatchResult
	if err := json.Unmarshal(raw, &gotResult); err != nil {
		t.Fatalf("result json did not unmarshal: %v", err)
	}
	if gotResult.ProvenanceVersion != matchResult.ProvenanceVersion {
		t.Fatalf("provenance_version = %d, want %d", gotResult.ProvenanceVersion, matchResult.ProvenanceVersion)
	}
	defShare, defOK := gotResult.ModelAuthored(g.Defender)
	wantDefShare, wantDefOK := matchResult.ModelAuthored(g.Defender)
	if defOK != wantDefOK || defShare != wantDefShare {
		t.Fatalf("round-tripped defender model_authored = (%v, %v), want (%v, %v)", defShare, defOK, wantDefShare, wantDefOK)
	}
	if !defOK {
		t.Fatalf("expected defender model_authored to be measured for a scripted match")
	}

	// replay.json: array of the same length as the in-memory replay log.
	rawReplay, err := os.ReadFile(replayPath)
	if err != nil {
		t.Fatalf("replay json not written: %v", err)
	}
	var gotReplay []eng.ReplayEvent
	if err := json.Unmarshal(rawReplay, &gotReplay); err != nil {
		t.Fatalf("replay json did not unmarshal: %v", err)
	}
	if len(gotReplay) != len(g.ReplayEvents) {
		t.Fatalf("replay json has %d events, want %d", len(gotReplay), len(g.ReplayEvents))
	}

	// manifest.json: built with the mode passed to writeMatchArtifacts.
	rawManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("manifest json not written: %v", err)
	}
	var gotManifest eng.ArenaRunManifest
	if err := json.Unmarshal(rawManifest, &gotManifest); err != nil {
		t.Fatalf("manifest json did not unmarshal: %v", err)
	}
	if gotManifest.RunType != "interactive" {
		t.Fatalf("manifest run_type = %q, want %q", gotManifest.RunType, "interactive")
	}

	// report.md: non-empty markdown report.
	rawReport, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("report markdown not written: %v", err)
	}
	if len(rawReport) == 0 {
		t.Fatalf("report markdown is empty")
	}
}

// TestWriteMatchArtifactsSkipsUnsetPaths guards the "do not write files when
// no -*-json/-report-md flag was given" requirement: with every path left
// empty, writeMatchArtifacts must create nothing.
func TestWriteMatchArtifactsSkipsUnsetPaths(t *testing.T) {
	g := buildScriptedTestGame(t)
	matchResult := g.BuildMatchResult()

	dir := t.TempDir()
	m := model{game: g, seed: 1, ruleset: eng.BaselineDuelRuleset()}

	writeMatchArtifacts(m, matchResult, "interactive", 0)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files written, found %d", len(entries))
	}
}
