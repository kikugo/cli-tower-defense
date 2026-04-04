package engine

import "testing"

func TestNormalizeDecisionDefenderPlaceFallbacks(t *testing.T) {
	n := normalizeDecision("defender", map[string]interface{}{
		"action":     "place",
		"tower_type": "invalid",
		"position":   []interface{}{"bad", "data"},
	})

	if n["action"] != "place" {
		t.Fatalf("expected place action, got %v", n["action"])
	}
	if n["tower_type"] != "basic" {
		t.Fatalf("expected tower fallback basic, got %v", n["tower_type"])
	}
}

func TestNormalizeDecisionAttackerSpawnFallbacks(t *testing.T) {
	n := normalizeDecision("attacker", map[string]interface{}{
		"action":     "spawn",
		"enemy_type": "unknown",
	})
	if n["enemy_type"] != "basic" {
		t.Fatalf("expected enemy fallback basic, got %v", n["enemy_type"])
	}
}

func TestNormalizeDecisionUnknownActionBecomesSave(t *testing.T) {
	n := normalizeDecision("attacker", map[string]interface{}{"action": "do-anything"})
	if n["action"] != "save" {
		t.Fatalf("expected save fallback, got %v", n["action"])
	}
}

