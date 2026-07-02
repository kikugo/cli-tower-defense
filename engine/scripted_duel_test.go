package engine

import (
	"strings"
	"testing"
)

func TestDefenderBaselineScriptEscalates(t *testing.T) {
	p := NewScriptedProvider(ResolvedPlayerModelConfig{
		PlayerModelConfig: PlayerModelConfig{Provider: ProviderScripted, Model: "defender_baseline"},
	})
	place, _ := p.GetTowerDecision(map[string]interface{}{
		"affordable_actions":     []string{"save", "place:basic"},
		"valid_tower_candidates": [][]int{{4, 9}},
	})
	if place["action"] != "place" {
		t.Fatalf("expected place, got %v", place["action"])
	}
	up, _ := p.GetTowerDecision(map[string]interface{}{
		"affordable_actions": []string{"save", "upgrade:2", "invest"},
	})
	if up["action"] != "upgrade" {
		t.Fatalf("expected upgrade when placement unavailable, got %v", up["action"])
	}
	if id, ok := toIntFromAny(up["tower_id"]); !ok || id != 2 {
		t.Fatalf("expected tower_id 2, got %v", up["tower_id"])
	}
	save, _ := p.GetTowerDecision(map[string]interface{}{
		"affordable_actions": []string{"save"},
	})
	if save["action"] != "save" {
		t.Fatalf("expected save when broke, got %v", save["action"])
	}
}

func TestRunScriptedDuelIsDeterministicAndFast(t *testing.T) {
	cfg := ScriptedDuelConfig{
		Seed: 11, MaxTicks: 400,
		Ruleset: BaselineDuelRuleset(), Balance: DefaultBalanceConfig(),
		DefenderScript: "defender_baseline", AttackerScript: "attacker_spawn",
	}
	r1 := RunScriptedDuel(cfg)
	r2 := RunScriptedDuel(cfg)
	if r1.Winner != r2.Winner || r1.Ticks != r2.Ticks {
		t.Fatalf("expected deterministic duel, got %s/%d vs %s/%d", r1.Winner, r1.Ticks, r2.Winner, r2.Ticks)
	}
	if r1.Winner == "" && !strings.Contains(r1.WinReason, "incomplete") {
		t.Fatalf("expected a resolved or explicitly incomplete duel, got %+v", r1.WinReason)
	}
}
