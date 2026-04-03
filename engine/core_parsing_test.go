package engine

import "testing"

func TestParseTowerResponseMalformedFallsBack(t *testing.T) {
	h := &OpenAIHandler{}

	got, err := h.parseTowerResponse("not-json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["action"] != "place" {
		t.Fatalf("expected fallback place action, got %v", got["action"])
	}
	if got["tower_type"] != "basic" {
		t.Fatalf("expected fallback basic tower, got %v", got["tower_type"])
	}
}

func TestParseTowerResponseMissingPositionGetsDefault(t *testing.T) {
	h := &OpenAIHandler{}

	got, err := h.parseTowerResponse(`{"action":"place","tower_type":"sniper"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pos, ok := got["position"].([]interface{})
	if !ok || len(pos) != 2 {
		t.Fatalf("expected default 2d position, got %#v", got["position"])
	}
}

func TestParseEnemyResponseEmptyFallsBackToBasicSpawn(t *testing.T) {
	h := &GeminiHandler{}

	got, err := h.parseEnemyResponse("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["action"] != "spawn" {
		t.Fatalf("expected spawn fallback, got %v", got["action"])
	}
	if got["enemy_type"] != "basic" {
		t.Fatalf("expected basic enemy fallback, got %v", got["enemy_type"])
	}
}

func TestParseEnemyResponseMissingTypeDefaultsToBasic(t *testing.T) {
	h := &GeminiHandler{}

	got, err := h.parseEnemyResponse(`{"action":"spawn"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["enemy_type"] != "basic" {
		t.Fatalf("expected basic enemy fallback, got %v", got["enemy_type"])
	}
}

