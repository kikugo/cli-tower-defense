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

func TestResolveTimeoutCreditsDefenderSurvival(t *testing.T) {
	g := NewGame("test", "test")
	g.ResolveTimeout()
	if !g.GameOver || g.Winner != g.Defender {
		t.Fatalf("expected defender survival win, over=%v winner=%q", g.GameOver, g.Winner)
	}
	result := g.BuildMatchResult()
	if result.WinReason != "defender_outlasted" {
		t.Fatalf("expected defender_outlasted reason, got %q", result.WinReason)
	}
	ends := 0
	for _, ev := range g.ReplayEvents {
		if ev.Type == ReplayGameEnd {
			ends++
		}
	}
	if ends != 1 {
		t.Fatalf("expected one game_end event, got %d", ends)
	}
}

func TestResolveTimeoutNoopWhenAlreadyOver(t *testing.T) {
	g := NewGame("test", "test")
	g.GameOver = true
	g.Winner = g.Attacker
	g.ResolveTimeout()
	if g.Winner != g.Attacker {
		t.Fatalf("expected existing winner preserved, got %q", g.Winner)
	}
	if g.BuildMatchResult().WinReason == "defender_outlasted" {
		t.Fatalf("timeout reason must not override a decided match")
	}
}

func TestRunScriptedDuelResolvesTimeouts(t *testing.T) {
	// A duel that cannot finish (save-only scripts, tiny tick budget) must
	// still end with a decisive survival verdict, not winner "".
	result := RunScriptedDuel(ScriptedDuelConfig{
		Seed: 3, MaxTicks: 10,
		Ruleset: BaselineDuelRuleset(), Balance: DefaultBalanceConfig(),
		DefenderScript: "defender_invest", AttackerScript: "attacker_spawn",
	})
	if result.Winner == "" {
		t.Fatalf("expected timeout to resolve to a winner, got none (%s)", result.WinReason)
	}
}

func TestDefenderHeld(t *testing.T) {
	win := MatchResult{Winner: "p1", Defender: "p1", Lives: map[string]int{"p1": 3}}
	if !win.DefenderHeld() {
		t.Fatalf("outright win must count as held")
	}
	survived := MatchResult{Winner: "", Defender: "p1", Lives: map[string]int{"p1": 2}}
	if !survived.DefenderHeld() {
		t.Fatalf("surviving to max ticks with lives must count as held")
	}
	lost := MatchResult{Winner: "p2", Defender: "p1", Lives: map[string]int{"p1": 0}}
	if lost.DefenderHeld() {
		t.Fatalf("attacker win must not count as held")
	}
	depleted := MatchResult{Winner: "", Defender: "p1", Lives: map[string]int{"p1": 0}}
	if depleted.DefenderHeld() {
		t.Fatalf("zero lives at timeout must not count as held")
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
