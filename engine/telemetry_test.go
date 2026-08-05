package engine

import "testing"

// --- breach count + per-wave aggregation + leak window, end to end --------

// findWave returns a pointer to the row for wave, or nil.
func findWave(summaries []WaveSummary, wave int) *WaveSummary {
	for i := range summaries {
		if summaries[i].Wave == wave {
			return &summaries[i]
		}
	}
	return nil
}

// countAlive returns how many of g.Enemies are alive (Health > 0) and
// tagged with the given wave number.
func countAlive(g *Game, wave int) int {
	n := 0
	for _, e := range g.Enemies {
		if e.Health > 0 && e.WaveNumber == wave {
			n++
		}
	}
	return n
}

// sumLivesLost totals LivesLost across every wave row -- this should always
// equal BreachCount (see the cheap-check test below), since LivesLost only
// ever increments at the single site that decrements Lives.
func sumLivesLost(summaries []WaveSummary) int {
	n := 0
	for _, w := range summaries {
		n += w.LivesLost
	}
	return n
}

// TestWaveSummaryForCurrentWaveTracksSentKilledLeaked drives one kill and one
// leak through the real UpdateGameState code paths (not the helpers
// directly) while wave 0 is still current, and checks the accounting
// identity Sent-Killed-Leaked == still-alive-count -- here that's 2-1-1=0,
// since both enemies resolved. It also cross-checks BreachCount against
// actual lives lost and RecentLeaks against the same two resolutions.
func TestWaveSummaryForCurrentWaveTracksSentKilledLeaked(t *testing.T) {
	g := newAssistGame()
	g.AssistsDisabled = true
	g.Resources[g.Attacker] = 1000

	// Spawn two "basic" enemies directly (bypassing the wave queue), so
	// both land in wave 0 -- see Enemy.WaveNumber's doc for why a direct
	// spawn is tagged with the current (not-yet-launched) wave.
	if !g.spawnEnemy("basic", nil) {
		t.Fatalf("expected first spawnEnemy to succeed")
	}
	if !g.spawnEnemy("basic", nil) {
		t.Fatalf("expected second spawnEnemy to succeed")
	}
	if len(g.Enemies) != 2 {
		t.Fatalf("expected 2 enemies on the field, got %d", len(g.Enemies))
	}
	killMe, leakMe := g.Enemies[0], g.Enemies[1]

	// Force killMe to die to a tower hit this tick: drop it to 1 HP and
	// place a tower on the nearest legal cell to it (basic's range of 5
	// comfortably covers whatever findNearestTowerPlacement returns).
	killMe.Health = 1
	fy, fx, ok := g.findNearestTowerPlacement(killMe.Pos.Y, killMe.Pos.X, 5)
	if !ok || !g.placeTower(fy, fx, "basic") {
		t.Fatalf("expected to place a tower near the kill-test enemy")
	}

	// Force leakMe to breach this tick regardless of speed, the same way a
	// real match eventually gets an enemy there: jump it to the last path
	// index (and matching position, so it is nowhere near the tower placed
	// above) rather than ticking it across the whole board.
	leakPath := g.Paths[leakMe.PathID]
	leakMe.PathIndex = len(leakPath) - 1
	leakMe.Pos = Position{Y: leakPath[len(leakPath)-1].Y, X: leakPath[len(leakPath)-1].X}

	livesBefore := g.Lives[g.Defender]
	g.UpdateGameState()

	if killMe.Health > 0 {
		t.Fatalf("expected the kill-test enemy to have been killed by the tower")
	}
	livesLost := livesBefore - g.Lives[g.Defender]
	if livesLost != 1 {
		t.Fatalf("expected exactly one life lost to the leak, got %d -> %d", livesBefore, g.Lives[g.Defender])
	}

	result := g.BuildMatchResult()

	// Breach count against lives lost -- they must agree. BreachCount is a
	// single whole-match int specifically so there is no per-player map to
	// mis-sum here.
	if result.BreachCount != livesLost {
		t.Fatalf("breach count %d does not match lives lost %d", result.BreachCount, livesLost)
	}

	wave0 := findWave(result.WaveSummaries, 0)
	if wave0 == nil {
		t.Fatalf("expected a wave 0 summary row, got %v", result.WaveSummaries)
	}
	if wave0.Sent != 2 {
		t.Fatalf("expected wave 0 sent=2, got %d", wave0.Sent)
	}
	if wave0.Killed != 1 {
		t.Fatalf("expected wave 0 killed=1, got %d", wave0.Killed)
	}
	if wave0.Leaked != 1 {
		t.Fatalf("expected wave 0 leaked=1, got %d", wave0.Leaked)
	}
	// The universal accounting identity: Sent-Killed-Leaked is always the
	// count of that wave's enemies still alive right now. Both resolved
	// here, so it is 0 -- countAlive confirms directly off g.Enemies rather
	// than trusting the arithmetic alone.
	if remaining := wave0.Sent - wave0.Killed - wave0.Leaked; remaining != countAlive(g, 0) {
		t.Fatalf("Sent-Killed-Leaked=%d does not match countAlive=%d", remaining, countAlive(g, 0))
	}
	if wave0.Towers != len(g.Towers) {
		t.Fatalf("expected wave 0 towers snapshot %d, got %d", len(g.Towers), wave0.Towers)
	}
	// LivesLost is the ordinary case's exact life cost: one real breach.
	if wave0.LivesLost != 1 {
		t.Fatalf("expected wave 0 LivesLost=1, got %d", wave0.LivesLost)
	}
	// LivesStart/LivesEnd are walked forward from Game.StartingLives, not
	// sampled: wave 0 is the only row, so LivesStart must be the game's
	// actual starting life total and LivesEnd = LivesStart - LivesLost.
	if wave0.LivesStart != g.StartingLives {
		t.Fatalf("expected wave 0 LivesStart=%d (Game.StartingLives), got %d", g.StartingLives, wave0.LivesStart)
	}
	if wave0.LivesEnd != wave0.LivesStart-wave0.LivesLost {
		t.Fatalf("expected wave 0 LivesEnd = LivesStart(%d) - LivesLost(%d) = %d, got %d", wave0.LivesStart, wave0.LivesLost, wave0.LivesStart-wave0.LivesLost, wave0.LivesEnd)
	}
	// This is also the ordinary-case check that the final (here, only) row's
	// LivesEnd equals the actual current/final Lives -- no separate "match
	// end" handling is needed for the ledger, since it is anchored forward
	// from a fixed origin rather than sampled at any point in time.
	if wave0.LivesEnd != g.Lives[g.Defender] {
		t.Fatalf("expected wave 0 LivesEnd to equal actual Lives %d, got %d", g.Lives[g.Defender], wave0.LivesEnd)
	}
	// The cheap check: total LivesLost across every wave equals BreachCount,
	// by construction (both come from the same site), but assert it anyway
	// so a future second life-decrementing site gets caught here.
	if got := sumLivesLost(result.WaveSummaries); got != result.BreachCount {
		t.Fatalf("expected sum(LivesLost)=%d to equal BreachCount=%d", got, result.BreachCount)
	}
	// Wave 0 is still the current wave (no spawnWave call has ever
	// succeeded, so g.Wave is still 0) -- Complete means "superseded", not
	// "fully resolved", and wave 0 has not been superseded yet.
	if wave0.Complete {
		t.Fatalf("expected wave 0 to read as not-yet-complete (still the current wave), got Complete=true")
	}

	// The rolling leak window recorded exactly these two resolutions, in
	// order: the tower kill first (runTowerPhase runs before movement),
	// then the leak.
	leaked, window, full := result.RecentLeaks()
	if window != 2 {
		t.Fatalf("expected a window of 2 resolutions, got %d", window)
	}
	if full {
		t.Fatalf("expected full=false with only 2 of LeakWindowSize=%d resolutions recorded", LeakWindowSize)
	}
	if leaked != 1 {
		t.Fatalf("expected 1 leaked in the recent window, got %d", leaked)
	}
}

// TestWaveSummaryCompleteMeansSupersededNotResolved is the case the
// coordinator flagged directly: Complete must mean "the match moved past
// this wave", not "every enemy sent into it resolved". It drives wave 0 to
// have one straggler still alive when wave 1 launches and checks that (a)
// wave 0 reads Complete=true anyway, (b) Sent-Killed-Leaked equals exactly
// the straggler count (the demo run's wave 4 showed 22-2-4=16 for the same
// reason), and (c) Towers/LivesEnd are frozen at the moment of supersession
// rather than drifting with events that belong to other waves. It also
// checks the LivesEnd -> LivesStart handoff between consecutive waves is
// monotone, the way the design's "10->9, 9->8" column requires.
func TestWaveSummaryCompleteMeansSupersededNotResolved(t *testing.T) {
	g := newAssistGame()
	g.AssistsDisabled = true
	g.Resources[g.Attacker] = 2000

	if !g.spawnEnemy("basic", nil) {
		t.Fatalf("expected first spawnEnemy to succeed")
	}
	if !g.spawnEnemy("basic", nil) {
		t.Fatalf("expected second spawnEnemy to succeed")
	}
	leakMe, straggler := g.Enemies[0], g.Enemies[1]

	// leakMe breaches this tick; straggler is left alone (starts at
	// PathIndex 0 with no tower anywhere near it) and stays alive.
	leakPath := g.Paths[leakMe.PathID]
	leakMe.PathIndex = len(leakPath) - 1
	leakMe.Pos = Position{Y: leakPath[len(leakPath)-1].Y, X: leakPath[len(leakPath)-1].X}

	livesAtStart := g.Lives[g.Defender]
	g.UpdateGameState()
	livesAfterBreach := g.Lives[g.Defender]
	if livesAfterBreach != livesAtStart-1 {
		t.Fatalf("expected exactly one life lost, got %d -> %d", livesAtStart, livesAfterBreach)
	}
	if straggler.Health <= 0 {
		t.Fatalf("expected the straggler to still be alive")
	}

	// Sanity check before superseding: wave 0 is not yet Complete (still
	// current), and its one unresolved enemy is exactly the straggler.
	preResult := g.BuildMatchResult()
	wave0Pre := findWave(preResult.WaveSummaries, 0)
	if wave0Pre == nil || wave0Pre.Complete {
		t.Fatalf("expected wave 0 present and Complete=false before wave 1 launches, got %+v", wave0Pre)
	}
	if remaining := wave0Pre.Sent - wave0Pre.Killed - wave0Pre.Leaked; remaining != 1 || countAlive(g, 0) != 1 {
		t.Fatalf("expected exactly 1 straggler pre-supersede, got remaining=%d countAlive=%d", remaining, countAlive(g, 0))
	}

	// Launch wave 1: this supersedes wave 0 while the straggler is still on
	// the board.
	if !g.spawnWave(false) {
		t.Fatalf("expected spawnWave to succeed")
	}
	if g.Wave != 1 {
		t.Fatalf("expected g.Wave to advance to 1, got %d", g.Wave)
	}
	// Nothing about Lives/Enemies changes as a side effect of launching a
	// wave -- the straggler is still alive and lives are unchanged, so any
	// difference below is coming from the telemetry fix, not new game state.
	if g.Lives[g.Defender] != livesAfterBreach {
		t.Fatalf("expected launching wave 1 not to change Lives, got %d -> %d", livesAfterBreach, g.Lives[g.Defender])
	}

	result := g.BuildMatchResult()
	wave0 := findWave(result.WaveSummaries, 0)
	if wave0 == nil {
		t.Fatalf("expected wave 0's row to still be present after wave 1 launched")
	}
	// (a) Complete despite an unresolved straggler.
	if !wave0.Complete {
		t.Fatalf("expected wave 0 Complete=true once superseded, even with a straggler still alive, got false")
	}
	// (b) The universal identity still holds, and still counts the
	// straggler -- being Complete does not zero it out.
	remaining := wave0.Sent - wave0.Killed - wave0.Leaked
	if remaining != 1 {
		t.Fatalf("expected 1 unresolved enemy (Sent=%d Killed=%d Leaked=%d), got remaining=%d", wave0.Sent, wave0.Killed, wave0.Leaked, remaining)
	}
	if countAlive(g, 0) != remaining {
		t.Fatalf("countAlive(wave 0)=%d does not match Sent-Killed-Leaked=%d", countAlive(g, 0), remaining)
	}
	// (c) LivesStart/LivesEnd are walked forward from Game.StartingLives and
	// wave 0's own LivesLost -- not sampled at supersede time -- so they
	// read the true values regardless of when supersedeWave ran.
	if wave0.LivesStart != livesAtStart {
		t.Fatalf("expected wave 0 LivesStart=%d, got %d", livesAtStart, wave0.LivesStart)
	}
	if wave0.LivesLost != 1 {
		t.Fatalf("expected wave 0 LivesLost=1 (the one real breach), got %d", wave0.LivesLost)
	}
	if wave0.LivesEnd != livesAfterBreach {
		t.Fatalf("expected wave 0 LivesEnd=%d (LivesStart-LivesLost), got %d", livesAfterBreach, wave0.LivesEnd)
	}
	if wave0.Towers != len(g.Towers) {
		t.Fatalf("expected wave 0 Towers frozen at %d, got %d", len(g.Towers), wave0.Towers)
	}

	// Spawn one enemy into wave 1 so its row exists, and confirm the
	// LivesEnd -> LivesStart handoff between consecutive waves.
	if !g.spawnEnemy("basic", nil) {
		t.Fatalf("expected an enemy to spawn into wave 1")
	}
	result = g.BuildMatchResult()
	wave1 := findWave(result.WaveSummaries, 1)
	if wave1 == nil {
		t.Fatalf("expected a wave 1 row once something was sent into it")
	}
	if wave1.LivesStart != wave0.LivesEnd {
		t.Fatalf("expected wave 1 LivesStart(%d) to equal wave 0 LivesEnd(%d)", wave1.LivesStart, wave0.LivesEnd)
	}

	// The cheap check, again, now across two waves.
	if got := sumLivesLost(result.WaveSummaries); got != result.BreachCount {
		t.Fatalf("expected sum(LivesLost)=%d to equal BreachCount=%d", got, result.BreachCount)
	}
}

// TestWaveSummaryLivesLedgerAnchoredForwardNotReconstructed is repro 2 from
// the review, turned into a regression test. An earlier version derived
// starting lives backward as "current lives + every leak ever recorded",
// which assumed every leak costs exactly one life -- false whenever a
// second enemy reaches the end of its path in the same tick as the breach
// that already ended the match (see the g.GameOver branch in
// UpdateGameState: Lives is deliberately not decremented twice for a match
// that is already over). That one bad input corrupted every earlier wave's
// LivesStart/LivesEnd, not just the wave it came from, because the walk ran
// backward from a moving endpoint. This test drives exactly that scenario
// (true starting lives 2; wave 0 has one real breach; wave 1, the final
// wave, has two simultaneous breaches, one real and one "free") and asserts
// the corrected numbers: wave 0 reads LivesStart=2, LivesEnd=1 -- not the
// LivesStart=3, LivesEnd=2 the backward walk produced.
func TestWaveSummaryLivesLedgerAnchoredForwardNotReconstructed(t *testing.T) {
	g := newAssistGame()
	g.AssistsDisabled = true
	g.Resources[g.Attacker] = 5000
	// True starting lives = 2, set explicitly alongside StartingLives the
	// same way ApplyRuleset sets both together.
	g.Lives[g.Defender] = 2
	g.StartingLives = 2

	// Wave 0: one real breach (costs 1 life, 2 -> 1).
	if !g.spawnEnemy("basic", nil) {
		t.Fatalf("expected wave-0 enemy to spawn")
	}
	w0e := g.Enemies[0]
	p0 := g.Paths[w0e.PathID]
	w0e.PathIndex = len(p0) - 1
	w0e.Pos = Position{Y: p0[len(p0)-1].Y, X: p0[len(p0)-1].X}
	g.UpdateGameState()
	if g.Lives[g.Defender] != 1 {
		t.Fatalf("expected Lives 2 -> 1 after wave 0's breach, got %d", g.Lives[g.Defender])
	}

	// Launch wave 1 (supersedes wave 0).
	if !g.spawnWave(false) {
		t.Fatalf("expected spawnWave to succeed")
	}

	// Wave 1 (final/current): two enemies breach in the SAME tick. The
	// first drops Lives 1 -> 0 (a real breach, GameOver becomes true mid
	// loop); the second is the "free" leak -- GameOver is already true when
	// it is processed, so Lives does not move for it. A third enemy is left
	// alone so wave 1's row exists with an unresolved entry too, matching
	// the shape of the earlier straggler test.
	for i := 0; i < 2; i++ {
		if !g.spawnEnemy("basic", nil) {
			t.Fatalf("expected wave-1 breach enemy %d to spawn", i)
		}
	}
	for _, e := range g.Enemies {
		p := g.Paths[e.PathID]
		e.PathIndex = len(p) - 1
		e.Pos = Position{Y: p[len(p)-1].Y, X: p[len(p)-1].X}
	}
	g.UpdateGameState()

	if !g.GameOver {
		t.Fatalf("expected the match to be over")
	}
	if g.Lives[g.Defender] != 0 {
		t.Fatalf("expected final Lives=0, got %d", g.Lives[g.Defender])
	}
	if g.BreachCount != 2 {
		t.Fatalf("expected BreachCount=2 (one per wave's real breach), got %d", g.BreachCount)
	}

	result := g.BuildMatchResult()
	wave0 := findWave(result.WaveSummaries, 0)
	wave1 := findWave(result.WaveSummaries, 1)
	if wave0 == nil || wave1 == nil {
		t.Fatalf("expected both wave 0 and wave 1 rows, got %v", result.WaveSummaries)
	}

	// The corrected numbers -- not the 3, 2 the backward walk produced.
	if wave0.LivesStart != 2 {
		t.Fatalf("expected wave 0 LivesStart=2 (true starting lives), got %d", wave0.LivesStart)
	}
	if wave0.LivesEnd != 1 {
		t.Fatalf("expected wave 0 LivesEnd=1, got %d", wave0.LivesEnd)
	}
	if wave0.LivesLost != 1 {
		t.Fatalf("expected wave 0 LivesLost=1, got %d", wave0.LivesLost)
	}

	// wave 1 has 2 leaked resolutions but only 1 of them cost a life -- this
	// is Leaked vs LivesLost differing exactly the way WaveSummary's doc
	// says they can, on the tick that ends the match.
	if wave1.Leaked != 2 {
		t.Fatalf("expected wave 1 Leaked=2 (both resolutions counted), got %d", wave1.Leaked)
	}
	if wave1.LivesLost != 1 {
		t.Fatalf("expected wave 1 LivesLost=1 (only the real breach), got %d", wave1.LivesLost)
	}
	if wave1.LivesStart != wave0.LivesEnd {
		t.Fatalf("expected wave 1 LivesStart(%d) to equal wave 0 LivesEnd(%d)", wave1.LivesStart, wave0.LivesEnd)
	}
	// The simultaneous-breach case of "final row's LivesEnd equals actual
	// final Lives at match end": the match ended mid-wave (wave 1 was never
	// superseded), and the ledger still lands on the true value.
	if wave1.LivesEnd != g.Lives[g.Defender] {
		t.Fatalf("expected wave 1 LivesEnd to equal actual final Lives %d, got %d", g.Lives[g.Defender], wave1.LivesEnd)
	}
	if wave1.LivesEnd != 0 {
		t.Fatalf("expected wave 1 LivesEnd=0, got %d", wave1.LivesEnd)
	}

	// The cheap check holds even here, where Leaked and LivesLost diverge:
	// LivesLost is what must sum to BreachCount, not Leaked.
	if got := sumLivesLost(result.WaveSummaries); got != result.BreachCount {
		t.Fatalf("expected sum(LivesLost)=%d to equal BreachCount=%d", got, result.BreachCount)
	}
}

// --- authored saves vs NoopStreak -------------------------------------------

// TestAuthoredSavesMatchesNoopStreakExactly drives applyDecision directly
// (the same entry point processTurn uses) with each of the three source
// combinations that matter and checks AuthoredSaves/DecisionsResolved track
// NoopStreak in lockstep -- counting anything else would make "saves N of M
// model-authored" tell a false causal story about what arms the engine's
// save-streak assists, which is exactly what this task exists to prevent.
func TestAuthoredSavesMatchesNoopStreakExactly(t *testing.T) {
	g := newAssistGame()
	g.AssistsDisabled = true
	playerID := g.Attacker
	role := "attacker"

	// 1. A model-authored save (SourceModel is the default when no source
	// tag is present) increments NoopStreak, AuthoredSaves, and
	// DecisionsResolved together.
	g.applyDecision(playerID, role, map[string]interface{}{"action": "save"})
	if g.NoopStreak[playerID] != 1 {
		t.Fatalf("expected NoopStreak 1 after a model save, got %d", g.NoopStreak[playerID])
	}
	if g.AuthoredSaves[playerID] != 1 {
		t.Fatalf("expected AuthoredSaves 1 after a model save, got %d", g.AuthoredSaves[playerID])
	}
	if g.DecisionsResolved[playerID] != 1 {
		t.Fatalf("expected DecisionsResolved 1, got %d", g.DecisionsResolved[playerID])
	}

	// 2. A forced save (SourceSkippedForcedSave) also counts: it is a real
	// player turn that resolved to "save", exactly like NoopStreak treats it.
	forced := map[string]interface{}{"action": "save"}
	markDecisionSource(forced, SourceSkippedForcedSave)
	g.applyDecision(playerID, role, forced)
	if g.NoopStreak[playerID] != 2 {
		t.Fatalf("expected NoopStreak 2 after a forced save, got %d", g.NoopStreak[playerID])
	}
	if g.AuthoredSaves[playerID] != 2 {
		t.Fatalf("expected AuthoredSaves 2 after a forced save, got %d", g.AuthoredSaves[playerID])
	}
	if g.DecisionsResolved[playerID] != 2 {
		t.Fatalf("expected DecisionsResolved 2, got %d", g.DecisionsResolved[playerID])
	}

	// 3. A substituted save (e.g. a provider failure normalized to "save")
	// must break the streak and must NOT count as authored -- but it still
	// resolved a decision, so DecisionsResolved keeps counting it. This is
	// the case the task brief calls out explicitly: the two numbers must
	// differ here, or the causal story "saves arm assists" would be false.
	substituted := map[string]interface{}{"action": "save"}
	markDecisionSource(substituted, SourceProviderFailure)
	g.applyDecision(playerID, role, substituted)
	if g.NoopStreak[playerID] != 0 {
		t.Fatalf("expected NoopStreak reset to 0 after a substituted save, got %d", g.NoopStreak[playerID])
	}
	if g.AuthoredSaves[playerID] != 2 {
		t.Fatalf("expected AuthoredSaves to stay at 2 (substituted save must not count), got %d", g.AuthoredSaves[playerID])
	}
	if g.DecisionsResolved[playerID] != 3 {
		t.Fatalf("expected DecisionsResolved 3, got %d", g.DecisionsResolved[playerID])
	}

	result := g.BuildMatchResult()
	authored, total, ok := result.AuthoredSaves(playerID)
	if !ok {
		t.Fatalf("expected AuthoredSaves to report measured (ok=true)")
	}
	if authored != 2 || total != 3 {
		t.Fatalf("expected AuthoredSaves(playerID) = (2, 3), got (%d, %d)", authored, total)
	}
}

// TestAuthoredSavesNotMeasuredBelowProvenanceVersion3 mirrors
// TestEngineAssistTotalNotMeasuredAtIntermediateProvenanceVersion: a
// MatchResult built before this telemetry existed must read as "not
// measured", never as a false zero.
func TestAuthoredSavesNotMeasuredBelowProvenanceVersion3(t *testing.T) {
	r := MatchResult{
		ProvenanceVersion:  2,
		AuthoredSaveCounts: map[string]int{"p2": 5},
		DecisionsResolved:  map[string]int{"p2": 10},
	}
	if authored, total, ok := r.AuthoredSaves("p2"); ok || authored != 0 || total != 0 {
		t.Fatalf("expected unmeasured (0,0,false) at ProvenanceVersion 2, got (%d,%d,%v)", authored, total, ok)
	}
	r.ProvenanceVersion = 3
	authored, total, ok := r.AuthoredSaves("p2")
	if !ok || authored != 5 || total != 10 {
		t.Fatalf("expected (5,10,true) at ProvenanceVersion 3, got (%d,%d,%v)", authored, total, ok)
	}
}

// --- rolling leak window -----------------------------------------------------

// TestRecentLeaksWindowFullnessAndEviction exercises recordResolution
// directly to pin down the window's fixed-size behaviour: not-full before
// LeakWindowSize resolutions (so the UI renders "leaked none yet" instead of
// a fabricated ratio), full at exactly LeakWindowSize, and oldest-evicted
// beyond that -- while LeakWindowTotal keeps counting uncapped.
func TestRecentLeaksWindowFullnessAndEviction(t *testing.T) {
	g := newAssistGame()

	result := g.BuildMatchResult()
	leaked, window, full := result.RecentLeaks()
	if window != 0 || full || leaked != 0 {
		t.Fatalf("expected an empty window (0 leaked, 0 window, full=false), got leaked=%d window=%d full=%v", leaked, window, full)
	}

	// 7 resolutions, 3 of them leaks -- window not yet full.
	pattern := []bool{true, false, false, true, false, true, false}
	for _, leak := range pattern {
		g.recordResolution(leak)
	}
	result = g.BuildMatchResult()
	leaked, window, full = result.RecentLeaks()
	if window != 7 || full {
		t.Fatalf("expected window=7 full=false after 7 resolutions, got window=%d full=%v", window, full)
	}
	if leaked != 3 {
		t.Fatalf("expected 3 leaked of 7, got %d", leaked)
	}

	// An 8th resolution fills the window exactly.
	g.recordResolution(false)
	result = g.BuildMatchResult()
	leaked, window, full = result.RecentLeaks()
	if window != LeakWindowSize || !full {
		t.Fatalf("expected window=%d full=true after 8 resolutions, got window=%d full=%v", LeakWindowSize, window, full)
	}
	if leaked != 3 {
		t.Fatalf("expected 3 leaked of 8 (unchanged), got %d", leaked)
	}

	// A 9th resolution evicts the oldest entry (a leak), so the window
	// stays at capacity but the leaked count drops.
	g.recordResolution(false)
	result = g.BuildMatchResult()
	leaked, window, full = result.RecentLeaks()
	if window != LeakWindowSize || !full {
		t.Fatalf("expected window to stay at %d (full) after a 9th resolution, got window=%d full=%v", LeakWindowSize, window, full)
	}
	if leaked != 2 {
		t.Fatalf("expected the oldest leak to be evicted, leaving 2, got %d", leaked)
	}
	if g.LeakWindowTotal != 9 {
		t.Fatalf("expected LeakWindowTotal 9 (all-time, uncapped by the window), got %d", g.LeakWindowTotal)
	}
}
