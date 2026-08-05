package engine

import "testing"

// newAssistGame returns a fresh two-player game with no between-turn pause,
// positioned at TickCount 0 (0 % 20 == 0, so applyAdaptivePressure's own
// cadence gate never has to be worked around) and a quiet board, so each
// test below only has to set up the resource/cooldown/streak state that
// picks out the one branch under test.
func newAssistGame() *Game {
	g := NewGame("k1", "k2")
	g.PauseBetweenTurns = false
	g.WaveQueue = nil
	g.Enemies = nil
	return g
}

// lastReplayEvent returns the most recently recorded event, failing the
// test if none exists.
func lastReplayEvent(t *testing.T, g *Game) ReplayEvent {
	t.Helper()
	if len(g.ReplayEvents) == 0 {
		t.Fatalf("expected at least one replay event, got none")
	}
	return g.ReplayEvents[len(g.ReplayEvents)-1]
}

// --- branch 1: reinforce_wave -----------------------------------------------

// TestApplyAdaptivePressureReinforceWaveRecordsAssist covers
// applyAdaptivePressure's first branch: before this task it called
// g.useAttackerAbility("reinforce_wave") directly, which bypasses
// applyDecision entirely, so nothing -- not ActionCounters, not
// DecisionSources, not the replay stream -- ever recorded it happening.
func TestApplyAdaptivePressureReinforceWaveRecordsAssist(t *testing.T) {
	g := newAssistGame()
	g.Resources[g.Attacker] = 200
	g.AbilityCooldowns["reinforce_wave"] = 0
	g.NoopStreak[g.Attacker] = 3

	g.applyAdaptivePressure()

	key := g.Attacker + ":" + string(AssistReinforceWave)
	if g.EngineAssists[key] != 1 {
		t.Fatalf("expected EngineAssists[%q] = 1, got %d (all: %v)", key, g.EngineAssists[key], g.EngineAssists)
	}
	if g.AbilityCooldowns["reinforce_wave"] == 0 {
		t.Fatalf("expected reinforce_wave to actually fire (cooldown set), it did not")
	}

	ev := lastReplayEvent(t, g)
	if ev.Type != ReplayEngineAssist {
		t.Fatalf("expected last event type %q, got %q", ReplayEngineAssist, ev.Type)
	}
	if ev.PlayerID != g.Attacker {
		t.Fatalf("expected PlayerID %q, got %q", g.Attacker, ev.PlayerID)
	}
	if ev.Role != "attacker" {
		t.Fatalf("expected Role \"attacker\", got %q", ev.Role)
	}
	if ev.Reason != string(AssistReinforceWave) {
		t.Fatalf("expected Reason %q, got %q", AssistReinforceWave, ev.Reason)
	}
	if got, _ := ev.Details["branch"].(string); got != string(AssistReinforceWave) {
		t.Fatalf("expected Details[branch] %q, got %v", AssistReinforceWave, ev.Details["branch"])
	}
	if got, _ := ev.Details["ability"].(string); got != "reinforce_wave" {
		t.Fatalf("expected Details[ability] \"reinforce_wave\", got %v", ev.Details["ability"])
	}
}

// --- branch 2: auto_wave ----------------------------------------------------

// TestApplyAdaptivePressureAutoWaveRecordsAssistAndMarksWaveEvent covers
// applyAdaptivePressure's second branch: before this task its g.spawnWave()
// call emitted a ReplayWave event attributed to the attacker that was
// byte-for-byte indistinguishable from a wave the attacker's model chose on
// its own turn. Both the accompanying ReplayEngineAssist event and the
// EngineAssisted field on the ReplayWave event itself must reflect that
// this one came from the engine.
func TestApplyAdaptivePressureAutoWaveRecordsAssistAndMarksWaveEvent(t *testing.T) {
	g := newAssistGame()
	g.Resources[g.Attacker] = 200
	// Nonzero cooldown keeps branch 1 (reinforce_wave, cost 70) from firing
	// first even though resources alone would otherwise afford it.
	g.AbilityCooldowns["reinforce_wave"] = 5
	g.NoopStreak[g.Attacker] = 3

	beforeWave := g.Wave
	g.applyAdaptivePressure()

	if g.Wave != beforeWave+1 {
		t.Fatalf("expected spawnWave to actually fire (Wave incremented), got before=%d after=%d", beforeWave, g.Wave)
	}

	key := g.Attacker + ":" + string(AssistAutoWave)
	if g.EngineAssists[key] != 1 {
		t.Fatalf("expected EngineAssists[%q] = 1, got %d (all: %v)", key, g.EngineAssists[key], g.EngineAssists)
	}

	// Two events should have been recorded for this call: the ReplayWave
	// event spawnWave always emits, followed by the ReplayEngineAssist
	// event recordEngineAssist adds.
	if len(g.ReplayEvents) < 2 {
		t.Fatalf("expected at least 2 replay events (wave + assist), got %d", len(g.ReplayEvents))
	}
	waveEvent := g.ReplayEvents[len(g.ReplayEvents)-2]
	assistEvent := g.ReplayEvents[len(g.ReplayEvents)-1]

	if waveEvent.Type != ReplayWave {
		t.Fatalf("expected the event before the assist event to be ReplayWave, got %q", waveEvent.Type)
	}
	if !waveEvent.EngineAssisted {
		t.Fatalf("expected the engine-initiated ReplayWave event to have EngineAssisted = true")
	}

	if assistEvent.Type != ReplayEngineAssist {
		t.Fatalf("expected last event type %q, got %q", ReplayEngineAssist, assistEvent.Type)
	}
	if assistEvent.Reason != string(AssistAutoWave) {
		t.Fatalf("expected Reason %q, got %q", AssistAutoWave, assistEvent.Reason)
	}
}

// TestSpawnWaveModelChosenWaveIsNotMarkedAssisted confirms the two other
// spawnWave call sites (a model's own "wave" action, and
// shouldAutoLaunchWave's applied_auto_wave path in applyDecision, which the
// task brief explicitly leaves alone) never set EngineAssisted -- only
// applyAdaptivePressure's call does.
func TestSpawnWaveModelChosenWaveIsNotMarkedAssisted(t *testing.T) {
	g := newAssistGame()
	g.Resources[g.Attacker] = 200

	if !g.spawnWave(false) {
		t.Fatalf("expected spawnWave to succeed with ample resources")
	}
	ev := lastReplayEvent(t, g)
	if ev.Type != ReplayWave {
		t.Fatalf("expected a ReplayWave event, got %q", ev.Type)
	}
	if ev.EngineAssisted {
		t.Fatalf("expected a model-chosen wave to have EngineAssisted = false")
	}
}

// --- branch 3: queue_enemy ---------------------------------------------------

// TestApplyAdaptivePressureQueueEnemyRecordsAssist covers
// applyAdaptivePressure's third branch: before this task the raw
// `g.WaveQueue = append(...)` left absolutely nothing behind -- no event,
// no counter of any kind.
func TestApplyAdaptivePressureQueueEnemyRecordsAssist(t *testing.T) {
	g := newAssistGame()
	// Too little for reinforce_wave (70) or the auto-wave floor with a
	// streak >= AutoDefendMinStreak (160), but enough for the 20-cost
	// fallback queue append.
	g.Resources[g.Attacker] = 20
	g.NoopStreak[g.Attacker] = 3

	beforeLen := len(g.WaveQueue)
	g.applyAdaptivePressure()

	if len(g.WaveQueue) != beforeLen+1 {
		t.Fatalf("expected one enemy queued, queue went from %d to %d", beforeLen, len(g.WaveQueue))
	}
	if g.WaveQueue[len(g.WaveQueue)-1] != "basic" {
		t.Fatalf("expected the queued enemy to be \"basic\", got %q", g.WaveQueue[len(g.WaveQueue)-1])
	}

	key := g.Attacker + ":" + string(AssistQueueEnemy)
	if g.EngineAssists[key] != 1 {
		t.Fatalf("expected EngineAssists[%q] = 1, got %d (all: %v)", key, g.EngineAssists[key], g.EngineAssists)
	}

	ev := lastReplayEvent(t, g)
	if ev.Type != ReplayEngineAssist {
		t.Fatalf("expected last event type %q, got %q", ReplayEngineAssist, ev.Type)
	}
	if ev.Reason != string(AssistQueueEnemy) {
		t.Fatalf("expected Reason %q, got %q", AssistQueueEnemy, ev.Reason)
	}
	if got, _ := ev.Details["enemy_type"].(string); got != "basic" {
		t.Fatalf("expected Details[enemy_type] \"basic\", got %v", ev.Details["enemy_type"])
	}
}

// --- AssistsDisabled gate ----------------------------------------------------

// TestApplyAdaptivePressureDisabledRecordsNothing is the unit-level half of
// verifying the task brief's central prediction: BaselineDuelRuleset sets
// DisableAssists (AssistsDisabled), and applyAdaptivePressure returns
// immediately when it is set, before touching EngineAssists or the replay
// stream at all -- even when every other trigger condition is satisfied.
// See also the balance-sweep determinism gate, which exercises this at the
// full-match level.
func TestApplyAdaptivePressureDisabledRecordsNothing(t *testing.T) {
	g := newAssistGame()
	g.AssistsDisabled = true
	g.Resources[g.Attacker] = 500
	g.NoopStreak[g.Attacker] = 10

	eventsBefore := len(g.ReplayEvents)
	g.applyAdaptivePressure()

	if len(g.EngineAssists) != 0 {
		t.Fatalf("expected no engine assists recorded while AssistsDisabled, got %v", g.EngineAssists)
	}
	if len(g.ReplayEvents) != eventsBefore {
		t.Fatalf("expected no new replay events while AssistsDisabled, had %d now have %d", eventsBefore, len(g.ReplayEvents))
	}
}

// --- MatchResult.EngineAssistTotal ------------------------------------------

// TestEngineAssistTotalNotMeasuredWithoutProvenance mirrors
// TestModelAuthoredNotMeasuredWithoutProvenance: a MatchResult unmarshaled
// from JSON written before this feature existed carries ProvenanceVersion
// 0, and must read as "not measured", never as "0 assists".
func TestEngineAssistTotalNotMeasuredWithoutProvenance(t *testing.T) {
	legacy := MatchResult{Defender: "p1", Attacker: "p2"}
	total, ok := legacy.EngineAssistTotal("p2")
	if ok {
		t.Fatalf("expected a provenance-less result to report unmeasured (ok=false), got total=%d ok=true", total)
	}
	if total != 0 {
		t.Fatalf("expected the zero value 0 alongside ok=false, got %d", total)
	}
}

// TestEngineAssistTotalNotMeasuredAtIntermediateProvenanceVersion covers the
// specific gap the task brief calls out: a MatchResult built by the code
// that added decision-source tracking (ProvenanceVersion 1) but predates
// assist counting must also read as unmeasured, not as zero -- ProvenanceVersion
// == 1 alone is not proof assists were ever counted.
func TestEngineAssistTotalNotMeasuredAtIntermediateProvenanceVersion(t *testing.T) {
	r := MatchResult{ProvenanceVersion: 1, DecisionSources: map[string]int{"p2:model": 5}}
	total, ok := r.EngineAssistTotal("p2")
	if ok {
		t.Fatalf("expected ProvenanceVersion 1 to report unmeasured for assists (ok=false), got total=%d ok=true", total)
	}
	if total != 0 {
		t.Fatalf("expected 0 alongside ok=false, got %d", total)
	}
}

// TestEngineAssistTotalGenuinelyZeroIsMeasured confirms the flip side: once
// ProvenanceVersion is 2, a player with no keys in EngineAssistCounts at all
// is correctly reported as a measured zero, not unmeasured.
func TestEngineAssistTotalGenuinelyZeroIsMeasured(t *testing.T) {
	r := MatchResult{ProvenanceVersion: 2, EngineAssistCounts: map[string]int{}}
	total, ok := r.EngineAssistTotal("p2")
	if !ok {
		t.Fatalf("expected a measured zero (ok=true) at ProvenanceVersion 2, got ok=false")
	}
	if total != 0 {
		t.Fatalf("expected total 0, got %d", total)
	}
}

// TestEngineAssistTotalSumsAcrossBranches confirms the per-branch keys for
// one player are summed, and a differently-prefixed player's keys are not
// double-counted into the total (the same prefix-matching discipline
// ModelAuthored already applies to DecisionSources).
func TestEngineAssistTotalSumsAcrossBranches(t *testing.T) {
	r := MatchResult{
		ProvenanceVersion: 2,
		EngineAssistCounts: map[string]int{
			"p2:reinforce_wave": 2,
			"p2:auto_wave":      1,
			"p2:queue_enemy":    4,
			"p1:queue_enemy":    99, // must not leak into p2's total
		},
	}
	total, ok := r.EngineAssistTotal("p2")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if total != 7 {
		t.Fatalf("expected total 7, got %d", total)
	}
}

// --- end-to-end: BuildMatchResult surfaces what the engine actually did ----

// TestBuildMatchResultSurfacesEngineAssistCounts drives a real quiet-board
// scenario through UpdateGameState (the same setup
// TestAdaptiveWaveDirectorAddsPressureOnQuietBoard uses) and confirms
// BuildMatchResult's EngineAssistCounts and EngineAssistTotal reflect what
// actually fired, with ProvenanceVersion bumped to 2.
func TestBuildMatchResultSurfacesEngineAssistCounts(t *testing.T) {
	g := NewGame("test", "test")
	g.Resources[g.Player2] = 500
	g.NoopStreak[g.Player2] = 4
	g.WaveQueue = nil
	g.Enemies = nil

	for i := 0; i < 40; i++ {
		g.UpdateGameState()
	}

	result := g.BuildMatchResult()
	if result.ProvenanceVersion != 2 {
		t.Fatalf("expected ProvenanceVersion 2, got %d", result.ProvenanceVersion)
	}
	total, ok := result.EngineAssistTotal(g.Attacker)
	if !ok {
		t.Fatalf("expected EngineAssistTotal to report measured (ok=true)")
	}
	if total == 0 {
		t.Fatalf("expected at least one engine assist on a 40-tick quiet board with a save streak, got 0 (counts=%v)", result.EngineAssistCounts)
	}
}
