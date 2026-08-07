package engine

import (
	"reflect"
	"testing"
)

// OpenAIHandler and GeminiHandler used to embed an *AIHandler carrying an
// http.Client and an rng, from when the handlers made their own HTTP calls.
// The provider split moved the transport to the providers, and every call site
// since has been a zero value -- `(&OpenAIHandler{}).parseTowerResponse(...)`.
// The embedded pointer was therefore always nil and never dereferenced, and
// NewAIHandler had no callers at all.
//
// These tests pin the property that made removing that state safe: the handlers
// are pure receivers. If someone re-adds a field, the first test fails and this
// comment explains why the field cannot be relied on -- nothing constructs
// these types with anything but a zero value.

func TestDecisionHandlersHoldNoState(t *testing.T) {
	for _, tc := range []struct {
		name string
		typ  reflect.Type
	}{
		{"OpenAIHandler", reflect.TypeOf(OpenAIHandler{})},
		{"GeminiHandler", reflect.TypeOf(GeminiHandler{})},
	} {
		if n := tc.typ.NumField(); n != 0 {
			t.Errorf("%s has %d field(s); it must stay stateless because every "+
				"call site constructs it as a zero value and would never "+
				"populate them", tc.name, n)
		}
	}
}

// TestZeroValueHandlersParseAndPrompt is the behavioural half: the four methods
// the live providers actually call must work on a zero value, with no setup.
func TestZeroValueHandlersParseAndPrompt(t *testing.T) {
	// Build the state the way the live path does rather than by hand: the
	// prompt builders type-assert on keys getPlayerGameState populates.
	// Seeded, because NewGame otherwise seeds its map from the clock.
	g := NewGame("test", "test")
	g.SetRandomSeed(1)
	defState := g.getPlayerGameState(g.Defender, "defender")
	attState := g.getPlayerGameState(g.Attacker, "attacker")

	if got := (&OpenAIHandler{}).createTowerPrompt(defState); got == "" {
		t.Error("createTowerPrompt returned empty on a zero-value OpenAIHandler")
	}
	if got := (&GeminiHandler{}).createEnemyPrompt(attState); got == "" {
		t.Error("createEnemyPrompt returned empty on a zero-value GeminiHandler")
	}

	// The cross-type delegates in gemini_tower.go / openai_enemy.go build the
	// opposite handler internally; they used to thread the shared *AIHandler
	// through and now construct a zero value directly.
	if got := (&GeminiHandler{}).createTowerPrompt(defState); got == "" {
		t.Error("GeminiHandler.createTowerPrompt delegate returned empty")
	}
	if got := (&OpenAIHandler{}).createEnemyPrompt(attState); got == "" {
		t.Error("OpenAIHandler.createEnemyPrompt delegate returned empty")
	}

	decision, err := (&OpenAIHandler{}).parseTowerResponse(
		`{"action":"place","tower_type":"basic","position":[5,5],"reason":"t"}`)
	if err != nil {
		t.Fatalf("parseTowerResponse on a zero value: %v", err)
	}
	if decision["action"] != "place" {
		t.Errorf("expected action place, got %v", decision["action"])
	}

	decision, err = (&GeminiHandler{}).parseEnemyResponse(
		`{"action":"spawn","enemy_type":"basic","reason":"t"}`)
	if err != nil {
		t.Fatalf("parseEnemyResponse on a zero value: %v", err)
	}
	if decision["action"] != "spawn" {
		t.Errorf("expected action spawn, got %v", decision["action"])
	}
}
