package engine

import (
	"sync/atomic"
	"testing"
	"time"
)

// countingProvider counts how many times each decision method is called, so
// tests can assert the provider was never dispatched to when there was
// nothing to decide. handlePlayerTurn calls GetTowerDecision/GetEnemyDecision
// from a background goroutine (see engine/core.go), while a test's poll loop
// reads the counts from the main goroutine, so the counters must be atomic.
type countingProvider struct {
	towerCalls     atomic.Int64
	enemyCalls     atomic.Int64
	defenderAction map[string]interface{}
	attackerAction map[string]interface{}
}

func (c *countingProvider) Name() string { return "counting" }

func (c *countingProvider) GetTowerDecision(map[string]interface{}) (map[string]interface{}, error) {
	c.towerCalls.Add(1)
	return c.defenderAction, nil
}

func (c *countingProvider) GetEnemyDecision(map[string]interface{}) (map[string]interface{}, error) {
	c.enemyCalls.Add(1)
	return c.attackerAction, nil
}

// brokeGame builds a game where both players are flat broke (0 resources, 0
// income) so affordableActions is exactly {"save"} for both roles: no tower
// is affordable, no tower exists to upgrade, slow zone/research/invest all
// need >=140, and on the attacker side no enemy/wave/ability/invest is
// affordable either. It wires a countingProvider on both seats so a test can
// assert whether the provider was actually dispatched to. skipForcedSaveTurns
// sets Game.SkipForcedSaveTurns, the opt-in ruleset flag that gates the
// short-circuit -- it defaults to false (today's behaviour: always dispatch)
// unless a caller turns it on.
func brokeGame(skipForcedSaveTurns bool) (*Game, *countingProvider) {
	g := NewGame("test", "test")
	g.PauseBetweenTurns = false
	g.AIDecisionInterval[g.Player1] = 0
	g.AIDecisionInterval[g.Player2] = 0
	g.Resources[g.Player1] = 0
	g.Resources[g.Player2] = 0
	g.Income[g.Player1] = 0
	g.Income[g.Player2] = 0
	g.SkipForcedSaveTurns = skipForcedSaveTurns

	cp := &countingProvider{
		defenderAction: map[string]interface{}{"action": "save"},
		attackerAction: map[string]interface{}{"action": "save"},
	}
	g.DecisionRouter.SetPlayerProvider(g.Player1, cp)
	g.DecisionRouter.SetPlayerProvider(g.Player2, cp)
	return g, cp
}

// TestForcedSaveSkipsProviderCall is the core contract of this change: with
// SkipForcedSaveTurns on, a player whose only legal action is "save" must
// never reach the provider. Both seats are broke here, so every turn on both
// sides should be forced without a single provider call across many turns.
func TestForcedSaveSkipsProviderCall(t *testing.T) {
	g, cp := brokeGame(true)

	turns := 0
	for i := 0; i < 30 && !g.GameOver; i++ {
		g.UpdateGameState()
		g.HandleAIDecisions()
		turns++
	}

	if cp.towerCalls.Load() != 0 || cp.enemyCalls.Load() != 0 {
		t.Fatalf("expected zero provider calls when only save is legal, got tower=%d enemy=%d over %d ticks", cp.towerCalls.Load(), cp.enemyCalls.Load(), turns)
	}

	// Sanity: the forced saves actually happened and were tagged as such,
	// not silently dropped.
	if g.DecisionSources[g.Player1+":"+string(SourceSkippedForcedSave)] == 0 {
		t.Fatalf("expected forced-save decisions to be recorded for %s, got sources=%v", g.Player1, g.DecisionSources)
	}
	if g.DecisionSources[g.Player2+":"+string(SourceSkippedForcedSave)] == 0 {
		t.Fatalf("expected forced-save decisions to be recorded for %s, got sources=%v", g.Player2, g.DecisionSources)
	}
}

// TestProviderIsCalledWhenSomethingIsAffordable is the counterpart to
// TestForcedSaveSkipsProviderCall: even with the flag on, the skip must be
// conditional on affordability, not a blanket "never call the provider"
// regression. With resources restored, the defender's first turn must reach
// the provider.
func TestProviderIsCalledWhenSomethingIsAffordable(t *testing.T) {
	g, cp := brokeGame(true)
	g.Resources[g.Player1] = 1000
	g.Resources[g.Player2] = 1000

	deadline := 50
	for i := 0; i < deadline && cp.towerCalls.Load() == 0 && cp.enemyCalls.Load() == 0 && !g.GameOver; i++ {
		if g.AIThinking[g.Player1] || g.AIThinking[g.Player2] {
			// Dispatch is asynchronous (a goroutine calls the provider and
			// posts the result on a channel): yield so it gets scheduled,
			// mirroring RunScriptedDuel's own poll loop.
			g.HandleAIDecisions()
			time.Sleep(time.Millisecond)
			continue
		}
		g.UpdateGameState()
		g.HandleAIDecisions()
	}

	if cp.towerCalls.Load() == 0 && cp.enemyCalls.Load() == 0 {
		t.Fatalf("expected at least one provider call once resources make more than save affordable")
	}
}

// TestForcedSaveStillSwitchesTurn is the deadlock regression test: with the
// flag on, skipping the provider call must never skip the turn switch too.
// If it did, the same player would face the identical all-save situation on
// the next tick forever and the match would never progress -- see Hazard 1
// in the task brief. With both seats broke, every single HandleAIDecisions
// call resolves synchronously (no async provider round trip), so the turn
// must flip on every call, and both players must actually get turns. This is
// the most important test in this file.
func TestForcedSaveStillSwitchesTurn(t *testing.T) {
	g, cp := brokeGame(true)

	seen := map[string]int{}
	prev := g.CurrentTurn
	for i := 0; i < 40; i++ {
		g.UpdateGameState()
		g.HandleAIDecisions()
		if g.GameOver {
			t.Fatalf("game ended unexpectedly at iteration %d before the alternation could be observed", i)
		}
		seen[g.CurrentTurn]++
		if g.CurrentTurn == prev {
			t.Fatalf("turn failed to switch on iteration %d: stuck on %s -- this is the deadlock this change must not introduce", i, g.CurrentTurn)
		}
		prev = g.CurrentTurn
	}

	if seen[g.Player1] == 0 || seen[g.Player2] == 0 {
		t.Fatalf("expected both players to receive turns, got %v", seen)
	}
	if cp.towerCalls.Load() != 0 || cp.enemyCalls.Load() != 0 {
		t.Fatalf("expected the alternation to happen without any provider calls, got tower=%d enemy=%d", cp.towerCalls.Load(), cp.enemyCalls.Load())
	}
}

// TestSkipDisabledStillReachesProvider is the inverse of
// TestForcedSaveSkipsProviderCall and protects the default path: with
// SkipForcedSaveTurns left at its zero value (false), a player whose only
// legal action is "save" must still reach the provider on every turn,
// exactly as it did before this feature existed.
func TestSkipDisabledStillReachesProvider(t *testing.T) {
	g, cp := brokeGame(false)

	// Loop until BOTH sides have been called at least once (not "either"):
	// CurrentTurn starts on the defender, so stopping as soon as one side has
	// a call would exit right after the defender's first dispatch, before
	// the attacker ever gets a turn.
	for i := 0; i < 40 && (cp.towerCalls.Load() == 0 || cp.enemyCalls.Load() == 0) && !g.GameOver; i++ {
		if g.AIThinking[g.Player1] || g.AIThinking[g.Player2] {
			g.HandleAIDecisions()
			time.Sleep(time.Millisecond)
			continue
		}
		g.UpdateGameState()
		g.HandleAIDecisions()
	}

	if cp.towerCalls.Load() == 0 {
		t.Fatalf("expected the defender's provider to still be called by default (flag off) even when only save is legal")
	}
	if cp.enemyCalls.Load() == 0 {
		t.Fatalf("expected the attacker's provider to still be called by default (flag off) even when only save is legal")
	}
	if g.DecisionSources[g.Player1+":"+string(SourceSkippedForcedSave)] != 0 {
		t.Fatalf("expected zero forced-save skips with the flag off, got sources=%v", g.DecisionSources)
	}
}

// TestSkipForcedSaveTurnsEliminatesRejectionsWhenEnabled is the whole reason
// this flag exists (see the coordinator's finding: same seed, same scripted
// pair, rejected_att goes from 157 to 0 once the skip is unconditional).
// Here both players are broke and a scripted provider blindly proposes an
// unaffordable "place" every turn without ever consulting affordable_actions
// -- mirroring ScriptedProvider's generic default branch in
// provider_scripted.go, the exact shape of provider that produced that
// rejection-rate divergence. With the flag off, that provider is asked every
// time and the placement is rejected every time. With the flag on, the
// provider is never asked once affordable_actions is exactly {"save"}, so no
// rejection is ever recorded, and provider calls collapse accordingly.
func TestSkipForcedSaveTurnsEliminatesRejectionsWhenEnabled(t *testing.T) {
	run := func(skip bool) (rejections int, providerCalls int64) {
		g := NewGame("test", "test")
		g.PauseBetweenTurns = false
		g.AIDecisionInterval[g.Player1] = 0
		g.AIDecisionInterval[g.Player2] = 0
		g.Resources[g.Player1] = 0
		g.Resources[g.Player2] = 0
		g.Income[g.Player1] = 0
		g.Income[g.Player2] = 0
		g.SkipForcedSaveTurns = skip

		cp := &countingProvider{
			// Always proposes placing a tower it can never afford at res=0,
			// regardless of what affordable_actions actually says -- this is
			// the class of provider the flag protects rejection accounting
			// for.
			defenderAction: map[string]interface{}{
				"action": "place", "tower_type": "basic",
				"position": []interface{}{float64(2), float64(2)},
			},
			attackerAction: map[string]interface{}{"action": "save"},
		}
		g.DecisionRouter.SetPlayerProvider(g.Player1, cp)
		g.DecisionRouter.SetPlayerProvider(g.Player2, cp)

		for i := 0; i < 30 && !g.GameOver; i++ {
			if g.AIThinking[g.Player1] || g.AIThinking[g.Player2] {
				g.HandleAIDecisions()
				time.Sleep(time.Millisecond)
				continue
			}
			g.UpdateGameState()
			g.HandleAIDecisions()
		}
		return g.RejectedActions[g.Player1+":place"], cp.towerCalls.Load()
	}

	offRejections, offCalls := run(false)
	if offRejections == 0 {
		t.Fatalf("expected rejections to be recorded with the flag off (default behaviour), got 0")
	}
	if offCalls == 0 {
		t.Fatalf("expected provider calls to keep happening with the flag off, got 0")
	}

	onRejections, onCalls := run(true)
	if onRejections != 0 {
		t.Fatalf("expected zero rejections with the flag on (nothing is ever proposed and rejected), got %d", onRejections)
	}
	if onCalls != 0 {
		t.Fatalf("expected zero provider calls with the flag on, got %d", onCalls)
	}
}

// TestModelAuthoredShareUnaffectedByForcedSaves asserts the accounting
// decision this change makes: SourceSkippedForcedSave is excluded from both
// the numerator and the denominator of ModelAuthored, so a skipped turn
// neither counts as model-authored nor as a substitution. Per Hazard 2 in
// the task brief, a match consisting entirely of forced saves must not
// report a crushed 0% authored share -- there is nothing to measure, which
// is different from measuring 0% authorship.
func TestModelAuthoredShareUnaffectedByForcedSaves(t *testing.T) {
	allForced := MatchResult{
		ProvenanceVersion: 1,
		DecisionSources: map[string]int{
			"p1:" + string(SourceSkippedForcedSave): 500,
		},
	}
	if share, ok := allForced.ModelAuthored("p1"); ok {
		t.Fatalf("expected ok=false (nothing measured) for a match with no genuine decisions, got share=%v ok=%v", share, ok)
	}

	// Every genuine decision here came from the model; the much larger pile
	// of forced saves alongside it must not dilute that into a lower share.
	mixed := MatchResult{
		ProvenanceVersion: 1,
		DecisionSources: map[string]int{
			"p1:" + string(SourceModel):             3,
			"p1:" + string(SourceSkippedForcedSave): 500,
		},
	}
	share, ok := mixed.ModelAuthored("p1")
	if !ok {
		t.Fatalf("expected ok=true once genuine decisions exist")
	}
	if share != 1.0 {
		t.Fatalf("expected forced saves excluded from the denominator giving share=1.0, got %v", share)
	}

	// A real substitution source, by contrast, must still count against the
	// share -- only the new skip source is exempted.
	withSubstitution := MatchResult{
		ProvenanceVersion: 1,
		DecisionSources: map[string]int{
			"p1:" + string(SourceModel):             1,
			"p1:" + string(SourceProviderFailure):   1,
			"p1:" + string(SourceSkippedForcedSave): 500,
		},
	}
	share2, ok2 := withSubstitution.ModelAuthored("p1")
	if !ok2 {
		t.Fatalf("expected ok=true")
	}
	if share2 != 0.5 {
		t.Fatalf("expected substitution source to still count against the share (0.5), got %v", share2)
	}
}
