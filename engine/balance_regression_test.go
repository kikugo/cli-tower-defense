package engine

import "testing"

// TestBaselineDuelBandWithDefaults is the contract of balance tuning: a
// competent-but-simple scripted defender must hold most (but not all)
// baseline duels under the default balance. Measured 15/20 at v2; the band
// [12,18] catches regressions in either direction (v1 numbers score ~0-2;
// an overpowered defense scores 19-20). Outcomes are bimodal per map layout,
// which is why the band is wider than the originally targeted 40-60%.
// Runtime ~3-6s (20 offline scripted duels).
func TestBaselineDuelBandWithDefaults(t *testing.T) {
	seeds := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	held := 0
	for _, seed := range seeds {
		result := RunScriptedDuel(ScriptedDuelConfig{
			Seed: seed, MaxTicks: 400,
			Ruleset: BaselineDuelRuleset(), Balance: DefaultBalanceConfig(),
			DefenderScript: "defender_baseline", AttackerScript: "attacker_baseline",
		})
		if result.DefenderHeld() {
			held++
		}
	}
	if held < 12 || held > 18 {
		t.Fatalf("defense held %d/20 baseline duels; outside the [12,18] balance band", held)
	}
}
