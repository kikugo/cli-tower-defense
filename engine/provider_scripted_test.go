package engine

import (
	"fmt"
	"testing"
)

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
