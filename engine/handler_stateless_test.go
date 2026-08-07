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
//
// "The four the providers call" is the whole list -- there is no cross-type
// delegate to also cover. Both providers reach across types directly, e.g.
// provider_gemini_native.go builds its enemy prompt with
// (&GeminiHandler{}).createEnemyPrompt even though it is the Gemini provider.
// An earlier version of this test also exercised thin GeminiHandler.createTowerPrompt
// / OpenAIHandler.createEnemyPrompt delegates that forwarded to these; they
// had no callers outside this file, so being tested was the only thing keeping
// them alive, and they were deleted.
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
