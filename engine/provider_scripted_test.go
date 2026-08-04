package engine

import "testing"

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
// attacker_baseline (the default branch) does -- including the known bug
// where a wave triggers if ANY player's resources entry is >= 260, not just
// the attacker's own. This is deliberately reproduced, not fixed, so a sweep
// comparing the two scripts isolates the spawned unit and nothing else.
func TestScriptedProviderAttackerHealerLaunchesWaveSameConditionAsBaseline(t *testing.T) {
	baseline := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_baseline"},
	})
	healer := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "attacker_healer"},
	})
	states := []map[string]interface{}{
		// Attacker's own resources trip the gate.
		{"resources": map[string]interface{}{"p1": 100.0, "p2": 260.0}, "affordable_actions": []string{"save"}},
		// Only the OPPONENT's resources trip the gate -- the latent bug,
		// reproduced faithfully in both scripts.
		{"resources": map[string]interface{}{"p1": 260.0, "p2": 50.0}, "affordable_actions": []string{"save"}},
		// Neither trips the gate.
		{"resources": map[string]interface{}{"p1": 100.0, "p2": 50.0}, "affordable_actions": []string{"save"}},
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
