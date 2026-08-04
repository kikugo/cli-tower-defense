package engine

import (
	"fmt"
	"math"
	"testing"
)

// --- attacker_live_like -----------------------------------------------
//
// The tests below cover attacker_live_like's contract end to end:
// determinism of the schedule, the emitted spawn composition matching the
// measured live-attacker shares over a long run, refusal to substitute a
// cheaper unit when the scheduled one is unaffordable, the stickiness of
// both the spawn cursor and the pending-wave flag while banking toward an
// unaffordable target, and that none of this touched attacker_baseline's
// own behaviour.

// liveLikeAlwaysAffordableState is a fixed affordable_actions list wide
// enough that every unit type in liveLikeSpawnSchedule and "wave" are
// always legal, so tests that walk many decisions exercise the
// wave-timing/spawn-schedule mechanics without affordability noise.
var liveLikeAlwaysAffordableState = map[string]interface{}{
	"affordable_actions": []string{
		"save", "wave",
		"spawn:basic", "spawn:fast", "spawn:tank", "spawn:shielded", "spawn:healer",
	},
}

// TestScriptedProviderAttackerLiveLikeIsDeterministic exercises the
// mandatory determinism property: two independent providers fed the exact
// same sequence of game states must produce the exact same sequence of
// decisions, call for call. attacker_live_like feeds 40-seed balance
// sweeps, so any nondeterminism (e.g. a map-range-driven tie-break) would
// show up here as flakiness rather than in a sweep months later.
func TestScriptedProviderAttackerLiveLikeIsDeterministic(t *testing.T) {
	newProvider := func() *ScriptedProvider {
		return NewScriptedProvider(ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_live_like"},
		})
	}
	p1 := newProvider()
	p2 := newProvider()

	const decisions = 200
	for i := 0; i < decisions; i++ {
		d1, err := p1.GetEnemyDecision(liveLikeAlwaysAffordableState)
		if err != nil {
			t.Fatalf("decision %d: p1 unexpected error: %v", i, err)
		}
		d2, err := p2.GetEnemyDecision(liveLikeAlwaysAffordableState)
		if err != nil {
			t.Fatalf("decision %d: p2 unexpected error: %v", i, err)
		}
		if fmt.Sprint(d1) != fmt.Sprint(d2) {
			t.Fatalf("decision %d: nondeterministic, p1=%v p2=%v", i, d1, d2)
		}
	}
}

// TestScriptedProviderAttackerLiveLikeSpawnCompositionMatchesTargetShares
// checks the emitted spawn composition against the measured live-attacker
// shares (shielded 39.8%, basic 33.0%, fast 22.7%, tank 4.5% of spawns --
// see liveLikeUnitWeights) over a long run where everything, including
// wave, is always affordable. Because wave is affordable here, roughly
// 1-in-14 decisions is a wave (which does not advance the spawn schedule
// cursor -- see the comment on liveLikeSpawnCursor), so 400 decisions walk
// a little over 9 full 40-slot cycles rather than exactly 10; the
// tolerance below is looser than the schedule's own ~half-point design
// margin to account for that partial-cycle remainder while still catching
// a materially wrong composition.
func TestScriptedProviderAttackerLiveLikeSpawnCompositionMatchesTargetShares(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_live_like"},
	})

	spawnCounts := map[string]int{}
	totalSpawns := 0
	const decisions = 400 // 10 full 40-slot schedule cycles
	for i := 0; i < decisions; i++ {
		d, err := p.GetEnemyDecision(liveLikeAlwaysAffordableState)
		if err != nil {
			t.Fatalf("decision %d: unexpected error: %v", i, err)
		}
		if d["action"] == "spawn" {
			enemyType, _ := d["enemy_type"].(string)
			spawnCounts[enemyType]++
			totalSpawns++
		}
	}

	if totalSpawns == 0 {
		t.Fatalf("expected at least one spawn over %d decisions, got none", decisions)
	}

	targets := map[string]float64{
		"shielded": 0.398,
		"basic":    0.330,
		"fast":     0.227,
		"tank":     0.045,
	}
	const tolerance = 0.02 // 2 points, looser than the schedule's own 0.5pt design margin
	for unitType, target := range targets {
		got := float64(spawnCounts[unitType]) / float64(totalSpawns)
		diff := got - target
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			t.Fatalf("spawn share for %s = %.4f (%d/%d), want ~%.4f (target), diff %.4f exceeds tolerance %.4f",
				unitType, got, spawnCounts[unitType], totalSpawns, target, diff, tolerance)
		}
	}
}

// TestScriptedProviderAttackerLiveLikeSavesWithoutSubstitutingWhenUnaffordable
// exercises the refusal-to-substitute rule that is the whole reason
// attacker_live_like's save share is an emergent property rather than a
// dial: when the scheduled unit isn't in affordable_actions, the script
// must return save, never spawn a cheaper alternative that happens to be
// affordable. liveLikeSpawnSchedule's first entry is "shielded" (see
// buildLiveLikeSpawnSchedule), so a fresh provider's very first decision
// consults that slot; the state here affords everything except shielded.
func TestScriptedProviderAttackerLiveLikeSavesWithoutSubstitutingWhenUnaffordable(t *testing.T) {
	if liveLikeSpawnSchedule[0] != "shielded" {
		t.Fatalf("test assumes liveLikeSpawnSchedule[0] == \"shielded\", got %q -- update the state below to match", liveLikeSpawnSchedule[0])
	}
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_live_like"},
	})
	state := map[string]interface{}{
		// Everything except spawn:shielded (the scheduled unit) is
		// affordable, including cheaper units -- a substitution bug would
		// spawn one of these instead of saving.
		"affordable_actions": []string{"save", "spawn:basic", "spawn:fast", "spawn:tank"},
	}
	decision, err := p.GetEnemyDecision(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "save" {
		t.Fatalf("expected save when the scheduled unit (shielded) is unaffordable (no substitution), got %v", decision)
	}
}

// TestScriptedProviderAttackerLiveLikeSpawnCursorDoesNotAdvanceWhileUnaffordable
// pins the "bank toward it" fix: liveLikeSpawnCursor must NOT move forward
// on a "save" decision. liveLikeSpawnSchedule[0] is "shielded"; a fresh
// provider fed 10 consecutive decisions where spawn:shielded is missing
// from affordable_actions (but other, cheaper spawns ARE present, so a
// cursor-advancing bug would let one of them clear) must still be sitting
// on schedule slot 0 afterward -- proven by then affording ONLY
// spawn:shielded and observing it spawn immediately, rather than save
// (which is what would happen if the cursor had silently walked ahead to
// schedule[10], "basic", while banking).
func TestScriptedProviderAttackerLiveLikeSpawnCursorDoesNotAdvanceWhileUnaffordable(t *testing.T) {
	if liveLikeSpawnSchedule[0] != "shielded" {
		t.Fatalf("test assumes liveLikeSpawnSchedule[0] == \"shielded\", got %q", liveLikeSpawnSchedule[0])
	}
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_live_like"},
	})
	unaffordableState := map[string]interface{}{
		// Every other spawn type is affordable EXCEPT shielded (the
		// scheduled unit) -- a cursor that advanced past an unaffordable
		// slot would let one of these clear instead.
		"affordable_actions": []string{"save", "spawn:basic", "spawn:fast", "spawn:tank"},
	}
	for i := 0; i < 10; i++ {
		d, err := p.GetEnemyDecision(unaffordableState)
		if err != nil {
			t.Fatalf("decision %d: unexpected error: %v", i, err)
		}
		if d["action"] != "save" {
			t.Fatalf("decision %d: expected save while banking for shielded (no substitution), got %v", i, d)
		}
	}

	onlyShieldedState := map[string]interface{}{
		"affordable_actions": []string{"save", "spawn:shielded"},
	}
	d, err := p.GetEnemyDecision(onlyShieldedState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d["action"] != "spawn" || d["enemy_type"] != "shielded" {
		t.Fatalf("expected the cursor to still be on schedule slot 0 (shielded) after 10 unaffordable decisions, got %v", d)
	}
}

// TestScriptedProviderAttackerLiveLikePendingWaveSurvivesUnaffordableTurns
// pins the other half of the sticky-banking fix: once liveLikeWaveInterval
// decisions have elapsed and a wave is armed, it must stay armed -- and be
// re-attempted on every subsequent decision -- until "wave" is actually
// affordable, rather than the one-shot check silently dropping it for that
// game. The 14th decision here has wave unaffordable (must fall through to
// spawn, not save-on-principle); several further decisions also have wave
// unaffordable; only once wave becomes affordable, decisions later, must it
// finally fire.
func TestScriptedProviderAttackerLiveLikePendingWaveSurvivesUnaffordableTurns(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_live_like"},
	})
	waveUnaffordableState := map[string]interface{}{
		// wave deliberately absent; spawns affordable so the fall-through
		// after an armed-but-unaffordable wave attempt has somewhere to go.
		"affordable_actions": []string{"save", "spawn:basic", "spawn:fast", "spawn:tank", "spawn:shielded"},
	}

	// Decisions 1-13: below liveLikeWaveInterval, no wave should be
	// attempted (and none is affordable anyway).
	for i := 1; i <= liveLikeWaveInterval-1; i++ {
		d, err := p.GetEnemyDecision(waveUnaffordableState)
		if err != nil {
			t.Fatalf("decision %d: unexpected error: %v", i, err)
		}
		if d["action"] == "wave" {
			t.Fatalf("decision %d: wave fired before liveLikeWaveInterval elapsed, got %v", i, d)
		}
	}

	// Decision 14: the wave arms, but is unaffordable this turn -- must
	// fall through to a spawn (or save), never wave, and never silently
	// drop the pending wave.
	d, err := p.GetEnemyDecision(waveUnaffordableState)
	if err != nil {
		t.Fatalf("decision %d: unexpected error: %v", liveLikeWaveInterval, err)
	}
	if d["action"] == "wave" {
		t.Fatalf("decision %d: expected no wave (unaffordable), got %v", liveLikeWaveInterval, d)
	}

	// Decisions 15-19: still unaffordable -- the pending wave must keep
	// being retried (never wave, since still unaffordable) rather than
	// expiring or waiting for another 14-decision cycle.
	for i := liveLikeWaveInterval + 1; i < liveLikeWaveInterval+6; i++ {
		d, err := p.GetEnemyDecision(waveUnaffordableState)
		if err != nil {
			t.Fatalf("decision %d: unexpected error: %v", i, err)
		}
		if d["action"] == "wave" {
			t.Fatalf("decision %d: expected no wave (still unaffordable), got %v", i, d)
		}
	}

	// Now wave becomes affordable, well past the original 14th decision --
	// the pending wave must still be armed and fire immediately.
	waveAffordableState := map[string]interface{}{
		"affordable_actions": []string{"save", "wave", "spawn:basic", "spawn:fast", "spawn:tank", "spawn:shielded"},
	}
	d, err = p.GetEnemyDecision(waveAffordableState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d["action"] != "wave" {
		t.Fatalf("expected the pending wave (armed at decision %d) to still fire once affordable, got %v", liveLikeWaveInterval, d)
	}
}

// TestScriptedProviderAttackerBaselineUnchangedByLiveLikeAddition pins
// attacker_baseline's behaviour against a fixed game state, directly, so
// this change (adding attacker_live_like alongside it) cannot be shown to
// have altered it: below the wave threshold it spawns basic, at/above it
// it waves, exactly as scriptedAttackerDefault always has.
func TestScriptedProviderAttackerBaselineUnchangedByLiveLikeAddition(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_baseline"},
	})

	belowThreshold := map[string]interface{}{
		"your_resources":     100.0,
		"affordable_actions": []string{"save", "spawn:basic"},
	}
	d, err := p.GetEnemyDecision(belowThreshold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d["action"] != "spawn" || d["enemy_type"] != "basic" || d["reason"] != "scripted" {
		t.Fatalf("expected exactly {action: spawn, enemy_type: basic, reason: scripted} below threshold, got %v", d)
	}

	atThreshold := map[string]interface{}{
		"your_resources":     260.0,
		"affordable_actions": []string{"save"},
	}
	d, err = p.GetEnemyDecision(atThreshold)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d["action"] != "wave" || d["reason"] != "scripted" {
		t.Fatalf("expected exactly {action: wave, reason: scripted} at threshold, got %v", d)
	}
}

func TestScriptedProviderReturnsDeterministicActions(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_basic"},
	})
	state := map[string]interface{}{
		"valid_tower_candidates": [][]int{{3, 4}},
	}
	decision, err := p.GetTowerDecision(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "place" {
		t.Fatalf("expected place action, got %v", decision["action"])
	}
}

func TestScriptedProviderAllowedByValidation(t *testing.T) {
	err := ValidateMatchConfig(MatchConfig{
		Player1: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_basic", APIKeyEnv: "K1"},
		Player2: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_wave", APIKeyEnv: "K2"},
	})
	if err != nil {
		t.Fatalf("expected scripted provider to validate: %v", err)
	}
}

// TestScriptedProviderAttackerShieldedSpawnsShieldedWhenAffordable exercises
// the attacker_shielded script added for the shielded-dominance sweep: it
// must always spawn shielded (never fall back to basic/fast) whenever
// spawn:shielded appears in affordable_actions.
func TestScriptedProviderAttackerShieldedSpawnsShieldedWhenAffordable(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_shielded"},
	})
	state := map[string]interface{}{
		"affordable_actions": []string{"save", "spawn:basic", "spawn:shielded"},
	}
	decision, err := p.GetEnemyDecision(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "spawn" || decision["enemy_type"] != "shielded" {
		t.Fatalf("expected spawn shielded, got %v", decision)
	}
}

func TestScriptedProviderAttackerShieldedSavesWhenUnaffordable(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_shielded"},
	})
	state := map[string]interface{}{
		"affordable_actions": []string{"save", "spawn:basic"},
	}
	decision, err := p.GetEnemyDecision(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "save" {
		t.Fatalf("expected save when shielded unaffordable (no fallback to a cheaper unit), got %v", decision)
	}
}

// TestScriptedProviderDefenderSingleTowerScripts exercises the
// defender_sniper/defender_splash/defender_buffer scripts added for the
// dead-content tower sweep: each must commit exclusively to its named tower
// type rather than falling back to basic.
func TestScriptedProviderDefenderSingleTowerScripts(t *testing.T) {
	cases := []struct {
		model     string
		towerType string
	}{
		{"defender_sniper", "sniper"},
		{"defender_splash", "splash"},
		{"defender_buffer", "buffer"},
	}
	for _, tc := range cases {
		p := NewScriptedProvider(ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: tc.model},
		})
		state := map[string]interface{}{
			"affordable_actions":     []string{"save", "place:" + tc.towerType},
			"valid_tower_candidates": [][]int{{3, 4}},
			"towers":                 []interface{}{},
		}
		decision, err := p.GetTowerDecision(state)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.model, err)
		}
		if decision["action"] != "place" || decision["tower_type"] != tc.towerType {
			t.Fatalf("%s: expected place %s, got %v", tc.model, tc.towerType, decision)
		}
	}
}

func TestScriptedProviderDefenderSingleTowerScriptSavesWhenUnaffordable(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_sniper"},
	})
	state := map[string]interface{}{
		// sniper not in affordable_actions; must not fall back to basic.
		"affordable_actions":     []string{"save", "place:basic"},
		"valid_tower_candidates": [][]int{{3, 4}},
	}
	decision, err := p.GetTowerDecision(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "save" {
		t.Fatalf("expected save when sniper unaffordable, got %v", decision)
	}
}

// TestScriptedProviderAttackerHealerSpawnsHealerWhenAffordable exercises the
// attacker_healer script added to isolate the healer enemy's heal ability
// (keyed on EnemyType == "healer") from its body stats, which a restatted
// "basic" cannot reach. It must spawn a real healer whenever spawn:healer
// appears in affordable_actions.
func TestScriptedProviderAttackerHealerSpawnsHealerWhenAffordable(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_healer"},
	})
	state := map[string]interface{}{
		"resources":          map[string]interface{}{"p1": 100.0, "p2": 100.0},
		"affordable_actions": []string{"save", "spawn:basic", "spawn:healer"},
	}
	decision, err := p.GetEnemyDecision(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "spawn" || decision["enemy_type"] != "healer" {
		t.Fatalf("expected spawn healer, got %v", decision)
	}
}

// TestScriptedProviderAttackerHealerFallsBackToBasicWhenUnaffordable checks
// that, unlike attacker_shielded (which saves), attacker_healer falls back to
// spawning basic when healer isn't affordable yet -- the same fallback
// attacker_baseline's default branch always takes.
func TestScriptedProviderAttackerHealerFallsBackToBasicWhenUnaffordable(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_healer"},
	})
	state := map[string]interface{}{
		"resources":          map[string]interface{}{"p1": 100.0, "p2": 100.0},
		"affordable_actions": []string{"save", "spawn:basic"},
	}
	decision, err := p.GetEnemyDecision(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "spawn" || decision["enemy_type"] != "basic" {
		t.Fatalf("expected fallback spawn basic when healer unaffordable, got %v", decision)
	}
}

// TestScriptedProviderAttackerHealerLaunchesWaveSameConditionAsBaseline
// verifies attacker_healer launches a wave under exactly the condition
// attacker_baseline (the default branch) does, now that both read only the
// player's own balance (gameState["your_resources"]) rather than scanning
// gameState["resources"], the shared map over every player that used to let
// either script fire a wave off the opponent's bank. Each case sets
// "resources" to a whole-game map whose OTHER entries would trip the old,
// buggy >= 260 scan, to prove neither script reads that map for this
// decision any more.
func TestScriptedProviderAttackerHealerLaunchesWaveSameConditionAsBaseline(t *testing.T) {
	baseline := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_baseline"},
	})
	healer := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_healer"},
	})
	states := []map[string]interface{}{
		// The attacker's own balance trips the gate.
		{"your_resources": 260.0, "resources": map[string]interface{}{"p1": 100.0, "p2": 260.0}, "affordable_actions": []string{"save"}},
		// Only the OPPONENT's balance is >= 260 (via the shared "resources"
		// map) -- must NOT trip the gate now that the fix is in.
		{"your_resources": 50.0, "resources": map[string]interface{}{"p1": 260.0, "p2": 50.0}, "affordable_actions": []string{"save"}},
		// Neither trips the gate.
		{"your_resources": 100.0, "resources": map[string]interface{}{"p1": 100.0, "p2": 50.0}, "affordable_actions": []string{"save"}},
	}
	for i, state := range states {
		bd, err := baseline.GetEnemyDecision(state)
		if err != nil {
			t.Fatalf("case %d: baseline unexpected error: %v", i, err)
		}
		hd, err := healer.GetEnemyDecision(state)
		if err != nil {
			t.Fatalf("case %d: healer unexpected error: %v", i, err)
		}
		if (bd["action"] == "wave") != (hd["action"] == "wave") {
			t.Fatalf("case %d: wave-launch condition diverged: baseline=%v healer=%v", i, bd, hd)
		}
	}
}

// TestScriptedProviderAttackerBaselineLaunchesWaveOnOwnResources verifies the
// default/attacker_baseline branch launches a wave once the player's OWN
// balance (your_resources) reaches the 260 threshold.
func TestScriptedProviderAttackerBaselineLaunchesWaveOnOwnResources(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_baseline"},
	})
	state := map[string]interface{}{
		"your_resources":     260.0,
		"affordable_actions": []string{"save"},
	}
	decision, err := p.GetEnemyDecision(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "wave" {
		t.Fatalf("expected wave when own resources hit the threshold, got %v", decision)
	}
}

// TestScriptedProviderAttackerBaselineDoesNotLaunchWaveOnOpponentResources is
// the regression test for the bug this change fixes: gameState["resources"]
// is a map over ALL players (see getGameState in engine/actions.go), so the
// old code launched a wave whenever ANY entry hit 260 -- including the
// defender's bank. Here only the opponent (p1) is at/above the threshold;
// the attacker's own balance (your_resources) is not, so no wave must fire.
// TestScriptedProviderDefenderHoarderSavesBelowThresholdAndPlacesAtOrAbove
// exercises the core contract of defender_hoarder (see hoarderBankThreshold
// and scriptedDefenderHoard in provider_scripted.go): below the threshold it
// must always save, even when a placement is affordable and a candidate
// exists; at or above the threshold it must place.
func TestScriptedProviderDefenderHoarderSavesBelowThresholdAndPlacesAtOrAbove(t *testing.T) {
	newState := func(resources float64) map[string]interface{} {
		return map[string]interface{}{
			"your_resources":         resources,
			"affordable_actions":     []string{"save", "place:basic"},
			"valid_tower_candidates": [][]int{{3, 4}},
			"towers":                 []interface{}{},
		}
	}

	// A fresh provider below threshold must save, despite place:basic being
	// affordable -- the whole point of the script is to refuse to spend
	// early.
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_hoarder"},
	})
	decision, err := p.GetTowerDecision(newState(299))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "save" {
		t.Fatalf("expected save below hoarderBankThreshold, got %v", decision)
	}

	// A fresh provider (independent instance, so no carried-over spend-phase
	// state) at exactly the threshold must place.
	p2 := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_hoarder"},
	})
	decision, err = p2.GetTowerDecision(newState(300))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "place" || decision["tower_type"] != "basic" {
		t.Fatalf("expected place basic at hoarderBankThreshold, got %v", decision)
	}

	// And comfortably above threshold, same thing.
	p3 := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_hoarder"},
	})
	decision, err = p3.GetTowerDecision(newState(500))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "place" || decision["tower_type"] != "basic" {
		t.Fatalf("expected place basic above hoarderBankThreshold, got %v", decision)
	}
}

// TestScriptedProviderDefenderHoarderKeepsSpendingUntilBrokeThenBanksAgain
// exercises the stateful part of the script that a single-call test cannot:
// once it crosses hoarderBankThreshold it must keep placing on every
// subsequent tick -- even as its own balance is reported falling back below
// the threshold -- until it can genuinely no longer afford to place or
// upgrade, and only then fall back to saving and re-arm the threshold gate.
func TestScriptedProviderDefenderHoarderKeepsSpendingUntilBrokeThenBanksAgain(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_hoarder"},
	})
	state := func(resources float64, affordablePlace bool) map[string]interface{} {
		affordable := []string{"save"}
		if affordablePlace {
			affordable = append(affordable, "place:basic")
		}
		return map[string]interface{}{
			"your_resources":         resources,
			"affordable_actions":     affordable,
			"valid_tower_candidates": [][]int{{3, 4}},
			"towers":                 []interface{}{},
		}
	}

	// Tick 1: crosses the threshold, enters spend mode, places.
	d, err := p.GetTowerDecision(state(300, true))
	if err != nil || d["action"] != "place" {
		t.Fatalf("tick1: expected place, got %v (err=%v)", d, err)
	}

	// Tick 2: balance has now dropped to 200 (below hoarderBankThreshold) but
	// place:basic is still affordable. A threshold-only ("if res>=300 place
	// else save") implementation would save here; the spend-phase state must
	// keep it placing instead.
	d, err = p.GetTowerDecision(state(200, true))
	if err != nil || d["action"] != "place" {
		t.Fatalf("tick2: expected place while still in spend phase below threshold, got %v (err=%v)", d, err)
	}

	// Tick 3: now genuinely broke -- place:basic no longer affordable, no
	// towers exist yet to upgrade. Must fall back to save and exit spend
	// mode.
	d, err = p.GetTowerDecision(state(50, false))
	if err != nil || d["action"] != "save" {
		t.Fatalf("tick3: expected save when broke, got %v (err=%v)", d, err)
	}

	// Tick 4: balance recovered to just under the threshold. Spend mode was
	// cleared in tick 3, so this must save again rather than resume spending
	// off stale state.
	d, err = p.GetTowerDecision(state(299, true))
	if err != nil || d["action"] != "save" {
		t.Fatalf("tick4: expected save below threshold after exiting spend phase, got %v (err=%v)", d, err)
	}

	// Tick 5: back at threshold -- resumes spending.
	d, err = p.GetTowerDecision(state(300, true))
	if err != nil || d["action"] != "place" {
		t.Fatalf("tick5: expected place at threshold again, got %v (err=%v)", d, err)
	}
}

// TestScriptedProviderDefenderHoarderPlacesSamePositionsAsBaseline is the
// placement-parity guard the whole comparison depends on: defender_hoarder
// must choose the exact same candidate index defender_baseline would, given
// the same candidate list and tower count, because scriptedDefenderBuild is
// shared between them. If this ever diverges, the two scripts would differ
// in *where* they place as well as *when* they spend, and any measured
// difference between them would be confounded and meaningless (see the
// task's "reuse defender_baseline's placement logic" requirement).
func TestScriptedProviderDefenderHoarderPlacesSamePositionsAsBaseline(t *testing.T) {
	candidates := [][]int{{1, 1}, {2, 2}, {3, 3}, {4, 4}, {5, 5}, {6, 6}, {7, 7}}
	for builtCount := 0; builtCount < len(candidates); builtCount++ {
		towers := make([]interface{}, builtCount)
		for i := range towers {
			towers[i] = map[string]interface{}{"id": i}
		}
		state := map[string]interface{}{
			"your_resources":         500.0,
			"affordable_actions":     []string{"save", "place:basic"},
			"valid_tower_candidates": candidates,
			"towers":                 towers,
		}

		baseline := NewScriptedProvider(ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_baseline"},
		})
		hoarder := NewScriptedProvider(ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_hoarder"},
		})

		bd, err := baseline.GetTowerDecision(state)
		if err != nil {
			t.Fatalf("built=%d: baseline unexpected error: %v", builtCount, err)
		}
		hd, err := hoarder.GetTowerDecision(state)
		if err != nil {
			t.Fatalf("built=%d: hoarder unexpected error: %v", builtCount, err)
		}
		if bd["action"] != "place" || hd["action"] != "place" {
			t.Fatalf("built=%d: expected both to place, got baseline=%v hoarder=%v", builtCount, bd, hd)
		}
		if fmt.Sprint(bd["position"]) != fmt.Sprint(hd["position"]) {
			t.Fatalf("built=%d: placement positions diverged: baseline=%v hoarder=%v", builtCount, bd["position"], hd["position"])
		}
	}
}

func TestScriptedProviderAttackerBaselineDoesNotLaunchWaveOnOpponentResources(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_baseline"},
	})
	state := map[string]interface{}{
		"your_resources":     50.0,
		"resources":          map[string]interface{}{"p1": 300.0, "p2": 50.0},
		"affordable_actions": []string{"save"},
	}
	decision, err := p.GetEnemyDecision(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] == "wave" {
		t.Fatalf("expected no wave when only the opponent's resources are at/above the threshold, got %v", decision)
	}
}

// TestScriptedProviderDefenderResearchScripts exercises the three
// defender_research_* scripts added to isolate each research tech's cost
// and effect (see researchTech in engine/actions.go and
// scriptedDefenderResearch in provider_scripted.go): each must (1) take its
// named research the moment it is affordable, (2) save -- not trickle into
// a tower -- while the tech isn't maxed yet but isn't currently affordable,
// and (3) delegate to build-coverage placement, exactly like
// defender_baseline, once the tech is maxed and no longer offered.
func TestScriptedProviderDefenderResearchScripts(t *testing.T) {
	cases := []struct {
		model string
		tech  string
	}{
		{"defender_research_economy", "economy"},
		{"defender_research_range", "range"},
		{"defender_research_control", "control"},
	}
	for _, tc := range cases {
		// 1. Takes the research action when it's affordable.
		p := NewScriptedProvider(ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: tc.model},
		})
		state := map[string]interface{}{
			"affordable_actions": []string{"save", "research:" + tc.tech, "place:basic"},
			"research":           map[string]interface{}{"economy": 0, "range": 0, "control": 0},
		}
		decision, err := p.GetTowerDecision(state)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.model, err)
		}
		if decision["action"] != "research" || decision["tech"] != tc.tech {
			t.Fatalf("%s: expected research %s while affordable, got %v", tc.model, tc.tech, decision)
		}

		// 2. Saves (rather than trickling into a tower placement) while the
		// tech is not yet maxed but not currently affordable.
		p2 := NewScriptedProvider(ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: tc.model},
		})
		state2 := map[string]interface{}{
			"affordable_actions":     []string{"save", "place:basic"},
			"research":               map[string]interface{}{"economy": 0, "range": 0, "control": 0},
			"valid_tower_candidates": [][]int{{3, 4}},
			"towers":                 []interface{}{},
		}
		decision2, err := p2.GetTowerDecision(state2)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.model, err)
		}
		if decision2["action"] != "save" {
			t.Fatalf("%s: expected save while banking toward %s (place:basic affordable but must not be taken), got %v", tc.model, tc.tech, decision2)
		}

		// 3. Once the tech is maxed (level 2) and no longer offered,
		// delegates to build-coverage placement exactly like
		// defender_baseline.
		p3 := NewScriptedProvider(ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: tc.model},
		})
		maxedResearch := map[string]interface{}{"economy": 0, "range": 0, "control": 0}
		maxedResearch[tc.tech] = 2
		state3 := map[string]interface{}{
			"affordable_actions":     []string{"save", "place:basic"},
			"research":               maxedResearch,
			"valid_tower_candidates": [][]int{{3, 4}},
			"towers":                 []interface{}{},
		}
		decision3, err := p3.GetTowerDecision(state3)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.model, err)
		}
		if decision3["action"] != "place" || decision3["tower_type"] != "basic" {
			t.Fatalf("%s: expected build-coverage placement once %s is maxed, got %v", tc.model, tc.tech, decision3)
		}
	}
}

// TestResearchRangeOrderingIsANoOpWithoutExistingTowers is the focused,
// direct property test for the deliberate consequence of
// defender_research_range's ordering (buy "range" before owning any
// tower): researchTech's "range" branch (engine/actions.go) only bumps
// Range on towers that already exist at the moment of purchase, so buying
// range with zero towers is a guaranteed no-op, while buying it after
// placing a tower raises that tower's range by 1. This exercises
// g.researchTech and g.placeTower directly (not the scripted provider) so
// the property under measurement is pinned regardless of any script logic
// built on top of it.
func TestResearchRangeOrderingIsANoOpWithoutExistingTowers(t *testing.T) {
	// Order A: buy range first, with zero towers on the board. A tower
	// placed afterward must have exactly the un-researched base range --
	// the earlier purchase must have affected nothing.
	g := NewGame("test", "test")
	if len(g.Towers) != 0 {
		t.Fatalf("expected a fresh game to have no towers, got %d", len(g.Towers))
	}
	if !g.researchTech("range") {
		t.Fatalf("expected researchTech(range) to succeed with no towers on the board")
	}
	if !g.placeTower(2, 2, "basic") {
		t.Fatalf("expected placeTower to succeed")
	}
	gotRange := g.Towers[0].Range
	unresearchedRange := g.Balance.Towers["basic"].Range
	if gotRange != unresearchedRange {
		t.Fatalf("tower placed after a no-towers-yet range purchase has range %d, want unresearched base %d (the purchase should have been a no-op)", gotRange, unresearchedRange)
	}

	// Order B: place a tower first, then buy range. That tower's range must
	// go up by exactly 1.
	g2 := NewGame("test", "test")
	if !g2.placeTower(2, 2, "basic") {
		t.Fatalf("expected placeTower to succeed")
	}
	before := g2.Towers[0].Range
	if !g2.researchTech("range") {
		t.Fatalf("expected researchTech(range) to succeed")
	}
	after := g2.Towers[0].Range
	if after != before+1 {
		t.Fatalf("tower range after buying range with an existing tower on the board = %d, want %d (before=%d + 1)", after, before+1, before)
	}
}

// TestScriptedProviderDefenderSlowZoneOnlyProposesLegalPathTiles exercises
// the defender_slowzone script (see scriptedDefenderSlowZone and
// scriptedSlowZoneTarget in provider_scripted.go) against a real Game: it
// must never propose a place_slow_zone position outside g.PathTileSet.
// Plays out several decision points against a live board (real enemies on
// real path tiles), applying each proposed slow zone through the real
// g.placeSlowZone so the engine itself confirms legality, not just a
// PathTileSet lookup.
func TestScriptedProviderDefenderSlowZoneOnlyProposesLegalPathTiles(t *testing.T) {
	g := NewGame("test", "test")
	g.FogOfWar = false
	for i := 0; i < 6; i++ {
		if !g.spawnEnemy("basic", nil) {
			t.Fatalf("expected spawnEnemy to succeed (i=%d)", i)
		}
	}

	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_slowzone"},
	})

	proposals := 0
	for attempt := 0; attempt < 6; attempt++ {
		state := g.getPlayerGameState(g.Defender, "defender")
		decision, err := p.GetTowerDecision(state)
		if err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", attempt, err)
		}
		if decision["action"] != "place_slow_zone" {
			continue
		}
		proposals++
		pos, ok := decision["position"].([]interface{})
		if !ok || len(pos) != 2 {
			t.Fatalf("attempt %d: expected a 2-element position, got %v", attempt, decision["position"])
		}
		y, okY := toIntFromAny(pos[0])
		x, okX := toIntFromAny(pos[1])
		if !okY || !okX {
			t.Fatalf("attempt %d: could not parse proposed position %v", attempt, decision["position"])
		}
		if _, onPath := g.PathTileSet[tileKey(y, x)]; !onPath {
			t.Fatalf("attempt %d: proposed slow zone at [%d,%d] is not a path tile", attempt, y, x)
		}
		if !g.placeSlowZone(y, x) {
			t.Fatalf("attempt %d: engine rejected the proposed slow zone at [%d,%d] as illegal", attempt, y, x)
		}
	}
	if proposals == 0 {
		t.Fatalf("expected at least one place_slow_zone proposal with live enemies on the board")
	}
}

// TestScriptedProviderDefenderSlowZoneFallsBackToBuildingWithNoEnemies
// exercises the other half of defender_slowzone's contract: with
// place_slow_zone affordable but no enemies visible in gameState (the only
// legal-position source it has), it must not guess -- it must fall through
// to exactly defender_baseline's build-coverage placement.
func TestScriptedProviderDefenderSlowZoneFallsBackToBuildingWithNoEnemies(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_slowzone"},
	})
	state := map[string]interface{}{
		"affordable_actions":     []string{"save", "place_slow_zone", "place:basic"},
		"valid_tower_candidates": [][]int{{3, 4}},
		"towers":                 []interface{}{},
		// Deliberately no "enemies" key: the board has no visible enemies.
	}
	decision, err := p.GetTowerDecision(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "place" || decision["tower_type"] != "basic" {
		t.Fatalf("expected fallback to build-coverage placement when no enemies are visible, got %v", decision)
	}
}

// TestScriptedProviderDefenderSlowZonePrefersUncoveredEnemyPosition checks
// scriptedSlowZoneTarget's dedup rule directly: given two enemies, one
// already sitting under an existing slow zone, the script must propose the
// other (uncovered) enemy's position rather than a duplicate.
func TestScriptedProviderDefenderSlowZonePrefersUncoveredEnemyPosition(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_slowzone"},
	})
	state := map[string]interface{}{
		"affordable_actions": []string{"save", "place_slow_zone"},
		"slow_zones":         [][]int{{5, 5}},
		"enemies": []interface{}{
			map[string]interface{}{"position": []int{5, 5}},
			map[string]interface{}{"position": []int{6, 6}},
		},
	}
	decision, err := p.GetTowerDecision(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "place_slow_zone" {
		t.Fatalf("expected place_slow_zone, got %v", decision)
	}
	pos, ok := decision["position"].([]interface{})
	if !ok || len(pos) != 2 {
		t.Fatalf("expected a 2-element position, got %v", decision["position"])
	}
	y, _ := toIntFromAny(pos[0])
	x, _ := toIntFromAny(pos[1])
	if y != 6 || x != 6 {
		t.Fatalf("expected the script to prefer the uncovered enemy position [6,6], got [%d,%d]", y, x)
	}
}

// TestScriptedProviderAttackerAbilityScripts exercises the three
// attacker_* ability scripts added to isolate each attacker ability's
// cost-efficiency (see useAttackerAbility in engine/actions.go and
// scriptedAttackerAbility in provider_scripted.go): each must (1) use its
// named ability whenever "ability:<name>" is legal, and otherwise fall back
// to exactly the attacker_baseline/default decision -- (2) spawning basic
// below the wave threshold, and (3) launching a wave at/above it.
func TestScriptedProviderAttackerAbilityScripts(t *testing.T) {
	cases := []struct {
		model   string
		ability string
	}{
		{"attacker_surge", "surge"},
		{"attacker_shield_burst", "shield_burst"},
		{"attacker_reinforce", "reinforce_wave"},
	}
	for _, tc := range cases {
		// 1. Uses the named ability whenever it is affordable.
		p := NewScriptedProvider(ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: tc.model},
		})
		state := map[string]interface{}{
			"affordable_actions": []string{"save", "ability:" + tc.ability},
			"your_resources":     100.0,
		}
		decision, err := p.GetEnemyDecision(state)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.model, err)
		}
		if decision["action"] != "ability" || decision["ability"] != tc.ability {
			t.Fatalf("%s: expected ability %s while affordable, got %v", tc.model, tc.ability, decision)
		}

		// 2. Falls back to exactly the attacker_baseline/default decision
		// when the ability isn't affordable: below the wave threshold,
		// spawns basic.
		p2 := NewScriptedProvider(ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: tc.model},
		})
		state2 := map[string]interface{}{
			"affordable_actions": []string{"save"},
			"your_resources":     100.0,
		}
		decision2, err := p2.GetEnemyDecision(state2)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.model, err)
		}
		if decision2["action"] != "spawn" || decision2["enemy_type"] != "basic" {
			t.Fatalf("%s: expected fallback spawn basic when ability unaffordable, got %v", tc.model, decision2)
		}

		// 3. And, same as attacker_baseline, launches a wave once own
		// resources reach the threshold.
		p3 := NewScriptedProvider(ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: tc.model},
		})
		state3 := map[string]interface{}{
			"affordable_actions": []string{"save"},
			"your_resources":     260.0,
		}
		decision3, err := p3.GetEnemyDecision(state3)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.model, err)
		}
		if decision3["action"] != "wave" {
			t.Fatalf("%s: expected fallback wave at own-resources threshold, got %v", tc.model, decision3)
		}
	}
}

// TestApplyAdaptivePressureReinforceWaveDoesNotIncrementActionCounters
// documents a confound relevant to reading any sweep result for
// attacker_reinforce: applyAdaptivePressure (engine/actions.go) can call
// g.useAttackerAbility("reinforce_wave") directly, on its own initiative,
// when the attacker has been idle with a quiet board. That call never
// builds a decision map and never goes through applyDecision, and
// g.ActionCounters is only ever incremented inside applyDecision (see
// core.go), so an engine-fired reinforce_wave use is invisible to
// ActionCounters -- it looks, from that counter alone, as if the ability
// was never used at all, even though the game state changed (cooldown set,
// enemies queued). This test pins that current behaviour down directly by
// calling applyAdaptivePressure with the exact preconditions it requires,
// rather than changing the engine to fix it (out of scope here).
func TestApplyAdaptivePressureReinforceWaveDoesNotIncrementActionCounters(t *testing.T) {
	g := NewGame("test", "test")
	g.TickCount = 20 // g.TickCount % 20 == 0 is required to run at all.
	g.NoopStreak[g.Attacker] = 3
	g.Resources[g.Attacker] = 300
	g.AbilityCooldowns["reinforce_wave"] = 0
	g.Enemies = nil
	g.WaveQueue = nil

	counterKey := g.Attacker + ":ability"
	before := g.ActionCounters[counterKey]
	beforeQueueLen := len(g.WaveQueue)

	g.applyAdaptivePressure()

	if g.AbilityCooldowns["reinforce_wave"] == 0 {
		t.Fatalf("expected applyAdaptivePressure to have fired reinforce_wave (cooldown still 0)")
	}
	if len(g.WaveQueue) <= beforeQueueLen {
		t.Fatalf("expected reinforce_wave to have queued extra enemies, queue len still %d", len(g.WaveQueue))
	}
	after := g.ActionCounters[counterKey]
	if after != before {
		t.Fatalf("CONFOUND CHANGED: engine-fired reinforce_wave via applyAdaptivePressure used to bypass g.ActionCounters (before=%d after=%d); if this now increments, applyAdaptivePressure started routing through applyDecision and the ActionCounters-undercounts-reinforce_wave caveat documented on scriptedAttackerAbility no longer holds", before, after)
	}
}

// TestScriptedProviderDefenderBasicBufferNeverProposesBufferWithFewerThanTwoTowers
// exercises defender_basic_buffer's first precondition: even with
// "place:buffer" affordable and a candidate that would sit in range of the
// lone existing tower, fewer than 2 non-buffer towers on the board means it
// must fall through to exactly scriptedDefenderBuild's basic-tower placement,
// never propose a buffer. Checked at both 0 and 1 existing non-buffer towers.
func TestScriptedProviderDefenderBasicBufferNeverProposesBufferWithFewerThanTwoTowers(t *testing.T) {
	baseState := func(towers []interface{}) map[string]interface{} {
		return map[string]interface{}{
			"affordable_actions":     []string{"save", "place:basic", "place:buffer"},
			"valid_tower_candidates": [][]int{{5, 5}, {5, 6}},
			"towers":                 towers,
		}
	}

	cases := []struct {
		name   string
		towers []interface{}
	}{
		{"zero existing towers", []interface{}{}},
		{"one existing non-buffer tower", []interface{}{
			map[string]interface{}{"type": "basic", "position": []int{5, 4}, "range": 5},
		}},
	}

	for _, tc := range cases {
		p := NewScriptedProvider(ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_basic_buffer"},
		})
		decision, err := p.GetTowerDecision(baseState(tc.towers))
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if decision["action"] == "place" && decision["tower_type"] == "buffer" {
			t.Fatalf("%s: expected no buffer proposal with fewer than 2 non-buffer towers, got %v", tc.name, decision)
		}
		if decision["action"] != "place" || decision["tower_type"] != "basic" {
			t.Fatalf("%s: expected fallback to basic build-coverage placement, got %v", tc.name, decision)
		}
	}
}

// TestScriptedProviderDefenderBasicBufferCoversAtLeastTwoExistingTowers
// checks the core placement rule: with 2 existing non-buffer towers and
// place:buffer affordable, the proposed position must actually sit within
// the buffer's range (bufferDefaultRange, since no buffer exists yet to read
// a range from) of at least 2 of them. The candidate list includes two decoy
// positions that each cover only 1 tower, so a script that ignored coverage
// entirely (e.g. always picking the first or last candidate) would fail this.
func TestScriptedProviderDefenderBasicBufferCoversAtLeastTwoExistingTowers(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_basic_buffer"},
	})
	state := map[string]interface{}{
		"affordable_actions": []string{"save", "place:basic", "place:buffer"},
		"towers": []interface{}{
			map[string]interface{}{"type": "basic", "position": []int{5, 5}},
			map[string]interface{}{"type": "sniper", "position": []int{5, 7}},
		},
		// [1,1] covers neither tower; [9,9] covers neither; [5,6] sits
		// distance 1 from both (5,5) and (5,7), within bufferDefaultRange
		// (2), so it is the only candidate that qualifies.
		"valid_tower_candidates": [][]int{{1, 1}, {9, 9}, {5, 6}},
	}
	decision, err := p.GetTowerDecision(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision["action"] != "place" || decision["tower_type"] != "buffer" {
		t.Fatalf("expected a buffer placement, got %v", decision)
	}
	pos, ok := decision["position"].([]interface{})
	if !ok || len(pos) != 2 {
		t.Fatalf("expected a 2-element position, got %v", decision["position"])
	}
	y, _ := toIntFromAny(pos[0])
	x, _ := toIntFromAny(pos[1])
	if y != 5 || x != 6 {
		t.Fatalf("expected the only 2-tower-covering candidate [5,6], got [%d,%d]", y, x)
	}

	// Independently verify the coverage property itself (not just the
	// specific coordinates above), the same way runTowerPhase (engine/
	// actions.go) measures it: Euclidean distance <= range.
	towerPositions := [][2]int{{5, 5}, {5, 7}}
	covered := 0
	for _, tp := range towerPositions {
		dy := float64(y - tp[0])
		dx := float64(x - tp[1])
		if math.Sqrt(dy*dy+dx*dx) <= float64(bufferDefaultRange) {
			covered++
		}
	}
	if covered < 2 {
		t.Fatalf("proposed buffer position [%d,%d] covers only %d of the 2 existing towers", y, x, covered)
	}
}

// TestScriptedProviderDefenderBasicBufferIsDeterministic feeds
// defender_basic_buffer the exact same game state repeatedly and requires
// the exact same proposed position every time -- this script feeds a 40-seed
// balance sweep whose output must be reproducible run to run, and Go
// randomizes map iteration order, so any map-range-driven tie-break would
// show up here as flakiness.
func TestScriptedProviderDefenderBasicBufferIsDeterministic(t *testing.T) {
	state := map[string]interface{}{
		"affordable_actions": []string{"save", "place:basic", "place:buffer"},
		"towers": []interface{}{
			map[string]interface{}{"type": "basic", "position": []int{2, 2}},
			map[string]interface{}{"type": "sniper", "position": []int{2, 4}},
			map[string]interface{}{"type": "splash", "position": []int{8, 8}},
		},
		// [2,3] is the only candidate that covers 2 towers (the basic+sniper
		// pair); [0,0] and [8,9] cover at most 1 each. The specific pick
		// isn't what this test is checking -- repeatability across calls is.
		"valid_tower_candidates": [][]int{{2, 3}, {0, 0}, {8, 9}},
	}

	var first map[string]interface{}
	for i := 0; i < 20; i++ {
		p := NewScriptedProvider(ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_basic_buffer"},
		})
		decision, err := p.GetTowerDecision(state)
		if err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", i, err)
		}
		if first == nil {
			first = decision
			continue
		}
		if decision["action"] != first["action"] ||
			decision["tower_type"] != first["tower_type"] ||
			fmt.Sprint(decision["position"]) != fmt.Sprint(first["position"]) {
			t.Fatalf("attempt %d: nondeterministic decision, first=%v got=%v", i, first, decision)
		}
	}
}

// TestScriptedProviderDefenderBasicBufferOnlyProposesValidCandidates checks
// that every buffer position defender_basic_buffer proposes, across several
// distinct game states, is one of gameState["valid_tower_candidates"] --
// never an invented cell.
func TestScriptedProviderDefenderBasicBufferOnlyProposesValidCandidates(t *testing.T) {
	states := []map[string]interface{}{
		{
			"affordable_actions": []string{"save", "place:buffer"},
			"towers": []interface{}{
				map[string]interface{}{"type": "basic", "position": []int{3, 3}},
				map[string]interface{}{"type": "basic", "position": []int{3, 5}},
			},
			"valid_tower_candidates": [][]int{{10, 10}, {3, 4}, {0, 0}},
		},
		{
			"affordable_actions": []string{"save", "place:buffer"},
			"towers": []interface{}{
				map[string]interface{}{"type": "sniper", "position": []int{1, 1}},
				map[string]interface{}{"type": "splash", "position": []int{1, 2}},
				map[string]interface{}{"type": "basic", "position": []int{20, 20}},
			},
			"valid_tower_candidates": [][]int{{1, 3}, {1, 0}, {6, 6}},
		},
	}

	for i, state := range states {
		p := NewScriptedProvider(ResolvedPlayerModelConfig{
			PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_basic_buffer"},
		})
		decision, err := p.GetTowerDecision(state)
		if err != nil {
			t.Fatalf("state %d: unexpected error: %v", i, err)
		}
		// Not every state here is guaranteed to produce a qualifying buffer
		// candidate; when it doesn't, the fallback build-coverage placement
		// must still only ever use a valid candidate -- checked below either
		// way, regardless of which branch (buffer vs. basic) fired.
		pos, ok := decision["position"].([]interface{})
		if !ok || len(pos) != 2 {
			t.Fatalf("state %d: expected a 2-element position, got %v", i, decision["position"])
		}
		y, _ := toIntFromAny(pos[0])
		x, _ := toIntFromAny(pos[1])
		candidates, _ := state["valid_tower_candidates"].([][]int)
		found := false
		for _, c := range candidates {
			if len(c) == 2 && c[0] == y && c[1] == x {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("state %d: proposed position [%d,%d] is not among valid_tower_candidates %v", i, y, x, candidates)
		}
	}
}

// TestScriptedAttackerLiveLikeAbilityFiresThenFallsThrough pins the two
// properties the live-like ability arms depend on: the ability is taken
// whenever it is legal, and when it is not legal the script plays the
// live-like schedule unchanged (never a substitute action, never a stall).
func TestScriptedAttackerLiveLikeAbilityFiresThenFallsThrough(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{PlayerModelConfig: PlayerModelConfig{
		Provider: ProviderScripted, Model: "attacker_live_like_surge", APIKeyEnv: "NONE",
	}})

	withAbility := map[string]interface{}{
		"affordable_actions": []string{"save", "spawn:basic", "spawn:shielded", "ability:surge"},
		"your_resources":     500,
	}
	got, err := p.GetEnemyDecision(withAbility)
	if err != nil {
		t.Fatalf("GetEnemyDecision: %v", err)
	}
	if got["action"] != "ability" || got["ability"] != "surge" {
		t.Fatalf("with ability:surge affordable, got %v, want the surge ability", got)
	}

	// Not legal -> must fall through to the live-like schedule, which on a
	// fully affordable state emits a spawn, not a save and not a substitute
	// ability.
	withoutAbility := map[string]interface{}{
		"affordable_actions": []string{"save", "spawn:basic", "spawn:shielded", "spawn:fast", "spawn:tank"},
		"your_resources":     500,
	}
	got, err = p.GetEnemyDecision(withoutAbility)
	if err != nil {
		t.Fatalf("GetEnemyDecision: %v", err)
	}
	if got["action"] != "spawn" {
		t.Fatalf("with ability:surge unaffordable, got %v, want a live-like spawn", got)
	}
	if _, isAbility := got["ability"]; isAbility {
		t.Fatalf("fell through to an ability anyway: %v", got)
	}
}
