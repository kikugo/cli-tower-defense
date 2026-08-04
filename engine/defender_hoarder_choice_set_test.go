package engine

import (
	"testing"
	"time"
)

// choiceCountingProvider wraps a real DecisionProvider and records, for
// every GetTowerDecision call, whether more than one action was legal
// (len(affordable_actions) > 1) before delegating to the wrapped provider
// unchanged. It never alters a decision -- only observes the gameState the
// real provider was handed -- so wrapping a provider in this type must not
// change match outcomes at all; see
// TestChoiceCountingProviderDoesNotChangeMatchOutcome.
type choiceCountingProvider struct {
	inner       DecisionProvider
	decisions   int
	multiChoice int
}

func (c *choiceCountingProvider) Name() string { return c.inner.Name() }

func (c *choiceCountingProvider) GetTowerDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	c.decisions++
	if actions, ok := gameState["affordable_actions"].([]string); ok && len(actions) > 1 {
		c.multiChoice++
	}
	return c.inner.GetTowerDecision(gameState)
}

func (c *choiceCountingProvider) GetEnemyDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	return c.inner.GetEnemyDecision(gameState)
}

// runInstrumentedDuel plays one offline scripted-vs-scripted match, identical
// to RunScriptedDuel, except the defender's (Player1's) provider is wrapped
// in a choiceCountingProvider so every defender decision point's choice set
// size is recorded. Kept as a separate copy of RunScriptedDuel's loop rather
// than a change to scripted_duel.go, since instrumentation for a measurement
// task has no business changing the shared driver every other test and the
// balance sweep depend on.
func runInstrumentedDuel(seed int64, maxTicks int, defenderScript, attackerScript string) (MatchResult, *choiceCountingProvider) {
	resolved := ResolvedMatchConfig{
		Player1: ResolvedPlayerModelConfig{PlayerModelConfig: PlayerModelConfig{
			Provider: ProviderScripted, Model: defenderScript, APIKeyEnv: "NONE",
		}},
		Player2: ResolvedPlayerModelConfig{PlayerModelConfig: PlayerModelConfig{
			Provider: ProviderScripted, Model: attackerScript, APIKeyEnv: "NONE",
		}},
	}
	g := NewGameFromResolvedConfig(resolved)
	g.Balance = DefaultBalanceConfig()
	g.ApplyRuleset(BaselineDuelRuleset())
	if seed != 0 {
		g.SetRandomSeed(seed)
	}
	g.PauseBetweenTurns = false
	g.AIDecisionInterval[g.Player1] = 0
	g.AIDecisionInterval[g.Player2] = 0

	tracker := &choiceCountingProvider{inner: NewScriptedProvider(resolved.Player1)}
	g.DecisionRouter.SetPlayerProvider(g.Player1, tracker)

	if maxTicks <= 0 {
		maxTicks = 400
	}
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
	return g.BuildMatchResult(), tracker
}

// TestChoiceCountingProviderDoesNotChangeMatchOutcome guards the instrument
// itself: wrapping defender_baseline in choiceCountingProvider (which only
// reads gameState and otherwise delegates every call unchanged) must produce
// byte-identical match results to the unwrapped duel, for the same seed.
// Without this, Task 2's choice-set measurement could not be trusted to
// describe the same match the balance sweep actually measures.
func TestChoiceCountingProviderDoesNotChangeMatchOutcome(t *testing.T) {
	plain := RunScriptedDuel(ScriptedDuelConfig{
		Seed: 7, MaxTicks: 400,
		Ruleset: BaselineDuelRuleset(), Balance: DefaultBalanceConfig(),
		DefenderScript: "defender_baseline", AttackerScript: "attacker_baseline",
	})
	instrumented, tracker := runInstrumentedDuel(7, 400, "defender_baseline", "attacker_baseline")
	if plain.Ticks != instrumented.Ticks || plain.Score[plain.Defender] != instrumented.Score[instrumented.Defender] || plain.DefenderHeld() != instrumented.DefenderHeld() {
		t.Fatalf("instrumentation changed match outcome: plain ticks=%d score=%d held=%v; instrumented ticks=%d score=%d held=%v",
			plain.Ticks, plain.Score[plain.Defender], plain.DefenderHeld(),
			instrumented.Ticks, instrumented.Score[instrumented.Defender], instrumented.DefenderHeld())
	}
	if tracker.decisions == 0 {
		t.Fatalf("expected at least one recorded defender decision")
	}
}

// TestDefenderHoarderChoiceSetIsMateriallyRicherThanBaseline is the Task 2
// acceptance criterion from the review brief: defender_baseline's
// spend-the-instant-you-can-afford-it strategy is broke almost always, so it
// should face a real choice (more than one legal action) on only a small
// slice of its turns; defender_hoarder banks to hoarderBankThreshold before
// spending, so it should face a real choice on a substantially larger slice.
// If defender_hoarder does not show a materially different choice-set
// profile here, the script does not do what it claims and no balance
// conclusion drawn from it would mean anything -- this test is the
// documented gate for that.
//
// Measured over the same 40 seeds x attacker_baseline x default balance
// used for the Task 3 re-measurement, matching the brief's expected shape:
// defender_baseline ~2-3%, defender_hoarder substantially higher.
func TestDefenderHoarderChoiceSetIsMateriallyRicherThanBaseline(t *testing.T) {
	seeds := make([]int64, 40)
	for i := range seeds {
		seeds[i] = int64(i + 1)
	}

	measure := func(script string) (decisions, multiChoice int) {
		for _, seed := range seeds {
			_, tracker := runInstrumentedDuel(seed, 400, script, "attacker_baseline")
			decisions += tracker.decisions
			multiChoice += tracker.multiChoice
		}
		return
	}

	baselineDecisions, baselineMulti := measure("defender_baseline")
	hoarderDecisions, hoarderMulti := measure("defender_hoarder")

	if baselineDecisions == 0 || hoarderDecisions == 0 {
		t.Fatalf("expected recorded decisions for both scripts, got baseline=%d hoarder=%d", baselineDecisions, hoarderDecisions)
	}

	baselinePct := 100 * float64(baselineMulti) / float64(baselineDecisions)
	hoarderPct := 100 * float64(hoarderMulti) / float64(hoarderDecisions)

	t.Logf("choice-set profile (fraction of defender decision points with >1 legal action), 40 seeds, attacker_baseline, default balance:")
	t.Logf("  defender_baseline: %d/%d = %.1f%%", baselineMulti, baselineDecisions, baselinePct)
	t.Logf("  defender_hoarder:  %d/%d = %.1f%%", hoarderMulti, hoarderDecisions, hoarderPct)

	// The brief's expected shape: baseline in the low single digits.
	if baselinePct > 10 {
		t.Fatalf("defender_baseline's choice-set share is %.1f%%, expected roughly 2-3%% (a spendthrift defender should be broke almost always)", baselinePct)
	}
	// The acceptance criterion: hoarder must be substantially richer, not a
	// rounding difference. Require at least a 5x multiple over baseline's
	// measured share, and an absolute floor so a near-zero baseline can't
	// trivially satisfy a ratio check.
	if hoarderPct < 20 || hoarderPct < baselinePct*5 {
		t.Fatalf("defender_hoarder's choice-set share (%.1f%%) is not materially richer than defender_baseline's (%.1f%%); the script does not do its job", hoarderPct, baselinePct)
	}
}

// TestDefenderBaselineExactRegressionGuard pins defender_baseline's
// behaviour on a fixed seed to exact values, as a direct regression guard
// for the determinism gate: defender_hoarder was added as a new switch case
// in provider_scripted.go and must not perturb defender_baseline's own
// branch at all. TestBaselineDuelBandWithDefaults (balance_regression_test.go)
// already guards the aggregate band; this guards the same script down to
// exact tick/score/outcome numbers for one seed, so any change to
// defender_baseline's behavior -- however small -- fails loudly here rather
// than only showing up as sweep table drift.
func TestDefenderBaselineExactRegressionGuard(t *testing.T) {
	result := RunScriptedDuel(ScriptedDuelConfig{
		Seed: 1, MaxTicks: 400,
		Ruleset: BaselineDuelRuleset(), Balance: DefaultBalanceConfig(),
		DefenderScript: "defender_baseline", AttackerScript: "attacker_baseline",
	})
	if result.Ticks != 189 {
		t.Errorf("ticks = %d, want 189", result.Ticks)
	}
	if got := result.Score[result.Defender]; got != 525 {
		t.Errorf("defender score = %d, want 525", got)
	}
	if result.DefenderHeld() {
		t.Errorf("DefenderHeld() = true, want false")
	}
	if lanes := result.Strata["lanes"]; lanes != "2" {
		t.Errorf("lanes stratum = %q, want \"2\"", lanes)
	}
}
