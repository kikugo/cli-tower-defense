package engine

import (
	"testing"
)

// TestFogOfWarHidesMostEnemies measures how much information fog of war
// actually withholds from the defender, rather than assuming either way.
//
// The measurement matters because fog looks inert from the outside: a
// -balance-sweep with fog on and one with fog off produce bit-identical
// results across 40 seeds, for defender_baseline AND for defender_slowzone
// (the only scripted defender that reads the enemy list at all). The
// natural reading is that fog does nothing. It is the wrong reading --
// measured, fog hides roughly 85% of live enemies from the defender on
// about 95% of the ticks where any enemy is alive.
//
// Both facts are true at once: fog withholds most of the board, and no
// scripted player's outcome depends on it, because no scripted player uses
// enemy positions for anything load-bearing. defender_slowzone only ever
// needs ONE visible enemy to name a legal tile, and at least one is
// essentially always visible (enemies spawn inside BaseVisionRange). So
// fog's entire effect lands on the model-facing prompt, where the scripted
// proxy cannot reach it.
//
// This asserts a loose lower bound rather than the exact share, so it stays
// a measurement and not a golden file. If it starts failing, fog has been
// weakened or disabled and the README's number needs re-deriving.
func TestFogOfWarHidesMostEnemies(t *testing.T) {
	const (
		seeds    = 8
		maxTicks = 400
	)

	totalEnemyObservations := 0
	hiddenEnemyObservations := 0
	ticksWithEnemies := 0
	ticksWithAnythingHidden := 0

	for seed := int64(1); seed <= seeds; seed++ {
		g := newFogMeasurementGame(t, seed)

		// Mirrors runHeadlessSimulation in main.go: a tick is only spent
		// when the simulation actually advances. Counting the spin-wait
		// iterations while a provider decision is outstanding burns the
		// whole budget before the first enemy ever spawns.
		for tick := 0; tick < maxTicks && !g.GameOver; {
			if g.AIThinking[g.Player1] || g.AIThinking[g.Player2] {
				g.HandleAIDecisions()
				continue
			}
			g.UpdateGameState()
			g.HandleAIDecisions()
			tick++

			live := len(g.Enemies)
			if live == 0 {
				continue
			}
			visible := len(g.visibleEnemiesForDefender())
			hidden := live - visible

			ticksWithEnemies++
			totalEnemyObservations += live
			hiddenEnemyObservations += hidden
			if hidden > 0 {
				ticksWithAnythingHidden++
			}
		}
	}

	if totalEnemyObservations == 0 {
		t.Fatalf("setup invariant violated: no enemies were ever alive across %d seeds, so this measures nothing", seeds)
	}

	hiddenShare := float64(hiddenEnemyObservations) / float64(totalEnemyObservations)
	tickShare := float64(ticksWithAnythingHidden) / float64(ticksWithEnemies)

	t.Logf("fog of war over %d seeds: %d/%d enemy-observations hidden (%.2f%%); %d/%d ticks-with-enemies had anything hidden at all (%.2f%%)",
		seeds, hiddenEnemyObservations, totalEnemyObservations, hiddenShare*100,
		ticksWithAnythingHidden, ticksWithEnemies, tickShare*100)

	if hiddenShare < 0.5 {
		t.Errorf("fog now hides only %.1f%% of live enemies from the defender, down from the 85.6%% this was measured at; fog has been weakened or disabled and the README's number needs re-deriving", hiddenShare*100)
	}
}

// newFogMeasurementGame builds the same scripted duel the balance sweep
// runs (defender_baseline vs attacker_baseline under BaselineDuelRuleset),
// with fog left at its default on-state.
func newFogMeasurementGame(t *testing.T, seed int64) *Game {
	t.Helper()
	resolved := ResolvedMatchConfig{
		Player1: ResolvedPlayerModelConfig{PlayerModelConfig: PlayerModelConfig{
			Provider: ProviderScripted, Model: "defender_baseline", APIKeyEnv: "NONE",
		}},
		Player2: ResolvedPlayerModelConfig{PlayerModelConfig: PlayerModelConfig{
			Provider: ProviderScripted, Model: "attacker_baseline", APIKeyEnv: "NONE",
		}},
	}
	g := NewGameFromResolvedConfig(resolved)
	g.ApplyRuleset(BaselineDuelRuleset())
	g.SetRandomSeed(seed)
	g.PauseBetweenTurns = false
	g.AIDecisionInterval[g.Player1] = 0
	g.AIDecisionInterval[g.Player2] = 0
	if !g.FogOfWar {
		t.Fatalf("setup invariant violated: BaselineDuelRuleset left FogOfWar off, so this test would measure nothing")
	}
	return g
}
