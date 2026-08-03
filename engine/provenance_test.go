package engine

import (
	"errors"
	"testing"
)

// newProvenanceGame returns a fresh two-player game configured for
// deterministic, assist-free provenance testing: engine assists
// (auto-defend / auto-wave, already correctly tagged applied_auto_*) are
// disabled so they cannot add noise to the substitution/primary distinction
// under test, and there is no between-turn pause.
func newProvenanceGame() *Game {
	g := NewGame("k1", "k2")
	g.AssistsDisabled = true
	g.PauseBetweenTurns = false
	return g
}

// --- markDecisionSource / takeDecisionSource --------------------------------

func TestMarkDecisionSourceFirstWriterWins(t *testing.T) {
	decision := map[string]interface{}{"action": "save"}
	markDecisionSource(decision, SourceProviderFailure)
	markDecisionSource(decision, SourceNormalizerDefault) // must be a no-op

	if got := takeDecisionSource(decision); got != SourceProviderFailure {
		t.Fatalf("expected first-writer-wins to keep %q, got %q", SourceProviderFailure, got)
	}
}

func TestTakeDecisionSourceDefaultsToModel(t *testing.T) {
	if got := takeDecisionSource(map[string]interface{}{"action": "save"}); got != SourceModel {
		t.Fatalf("expected an untagged decision to default to SourceModel, got %q", got)
	}
	if got := takeDecisionSource(nil); got != SourceModel {
		t.Fatalf("expected a nil decision to default to SourceModel, got %q", got)
	}
}

func TestTakeDecisionSourceStripsTheReservedKey(t *testing.T) {
	decision := map[string]interface{}{"action": "save"}
	markDecisionSource(decision, SourceParserEmpty)
	takeDecisionSource(decision)
	if _, exists := decision[decisionSourceKey]; exists {
		t.Fatalf("expected takeDecisionSource to remove the reserved key, still present: %v", decision)
	}
}

// --- parser tagging (points 1-3 of ARENA-AUDIT.md's nine substitution points) --

func TestParseTowerResponseTagsUnparseableFallback(t *testing.T) {
	got, err := (&OpenAIHandler{}).parseTowerResponse("I think I should build something")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src := takeDecisionSource(got); src != SourceParserUnparseable {
		t.Fatalf("expected SourceParserUnparseable, got %q", src)
	}
}

func TestParseTowerResponseTagsMissingPosition(t *testing.T) {
	got, err := (&OpenAIHandler{}).parseTowerResponse(`{"action":"place","tower_type":"basic"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src := takeDecisionSource(got); src != SourceParserUnparseable {
		t.Fatalf("expected a missing position to be tagged SourceParserUnparseable, got %q", src)
	}
}

func TestParseTowerResponseCleanDecisionIsUntagged(t *testing.T) {
	got, err := (&OpenAIHandler{}).parseTowerResponse(`{"action":"place","tower_type":"basic","position":[5,5]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src := takeDecisionSource(got); src != SourceModel {
		t.Fatalf("expected a fully-specified decision to be untagged (SourceModel), got %q", src)
	}
}

func TestParseEnemyResponseTagsEmpty(t *testing.T) {
	got, err := (&GeminiHandler{}).parseEnemyResponse("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src := takeDecisionSource(got); src != SourceParserEmpty {
		t.Fatalf("expected SourceParserEmpty, got %q", src)
	}
}

func TestParseEnemyResponseTagsUnparseable(t *testing.T) {
	got, err := (&GeminiHandler{}).parseEnemyResponse("```\nno json here\n```")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src := takeDecisionSource(got); src != SourceParserUnparseable {
		t.Fatalf("expected SourceParserUnparseable, got %q", src)
	}
}

// --- normalizer tagging (point 8) -------------------------------------------

func TestNormalizeDecisionTagsUnknownAction(t *testing.T) {
	n := normalizeDecision("defender", map[string]interface{}{"action": "fortify"})
	if src := takeDecisionSource(n); src != SourceNormalizerDefault {
		t.Fatalf("expected SourceNormalizerDefault for an unknown action, got %q", src)
	}
}

func TestNormalizeDecisionTagsBadTowerType(t *testing.T) {
	n := normalizeDecision("defender", map[string]interface{}{
		"action": "place", "tower_type": "laser", "position": []interface{}{float64(5), float64(5)},
	})
	if src := takeDecisionSource(n); src != SourceNormalizerDefault {
		t.Fatalf("expected SourceNormalizerDefault for an invalid tower type, got %q", src)
	}
}

func TestNormalizeDecisionTagsBadEnemyType(t *testing.T) {
	n := normalizeDecision("attacker", map[string]interface{}{"action": "spawn", "enemy_type": "dragon"})
	if src := takeDecisionSource(n); src != SourceNormalizerDefault {
		t.Fatalf("expected SourceNormalizerDefault for an invalid enemy type, got %q", src)
	}
}

func TestNormalizeDecisionCleanDecisionIsUntagged(t *testing.T) {
	n := normalizeDecision("defender", map[string]interface{}{
		"action": "place", "tower_type": "basic", "position": []interface{}{float64(5), float64(5)},
	})
	if src := takeDecisionSource(n); src != SourceModel {
		t.Fatalf("expected a fully-valid decision to be untagged (SourceModel), got %q", src)
	}
}

// TestNormalizeDecisionPreservesUpstreamTag is the first-writer-wins case
// that matters most: a provider failure normalized from {"action":"none"}
// to "save" must keep its SourceProviderFailure tag, not be silently
// downgraded to SourceNormalizerDefault by the normalizer reacting to
// "none" being an invalid action.
func TestNormalizeDecisionPreservesUpstreamTag(t *testing.T) {
	raw := map[string]interface{}{"action": "none"}
	markDecisionSource(raw, SourceProviderFailure)

	source := takeDecisionSource(raw)
	normalized := normalizeDecision("defender", raw)
	if s := takeDecisionSource(normalized); source == SourceModel {
		source = s
	}
	if source != SourceProviderFailure {
		t.Fatalf("expected the upstream SourceProviderFailure tag to win over the normalizer's own tag, got %q", source)
	}
}

// --- provider failure: errors are returned and counted (P1.1) --------------

func TestProviderFailureIsCounted(t *testing.T) {
	g := newProvenanceGame()
	g.pendingTurnResults <- turnResult{
		playerID: g.Player1,
		role:     "defender",
		err:      errors.New("stub http failure"),
	}

	g.processPendingTurnResults()

	if g.TotalProviderErrorsForPlayer(g.Player1) == 0 {
		t.Fatalf("expected ProviderErrors to increment on a provider failure")
	}
	foundEvent := false
	for _, ev := range g.ReplayEvents {
		if ev.Type == ReplayProviderErr && ev.PlayerID == g.Player1 {
			foundEvent = true
		}
		if ev.Type == ReplayOutcome && ev.PlayerID == g.Player1 {
			t.Fatalf("a provider failure must skip the turn, not apply a decision, but found an outcome event: %#v", ev)
		}
	}
	if !foundEvent {
		t.Fatalf("expected a provider_error replay event")
	}
	if g.DecisionSources[g.Player1+":"+string(SourceProviderFailure)] == 0 {
		t.Fatalf("expected the failed decision attempt to be counted in DecisionSources, got %v", g.DecisionSources)
	}
}

// --- the invariant: none of the nine forced failure modes serialize as primary --

// TestSubstitutedDecisionsNeverSerializeAsPrimary is the negative invariant
// from AUDIT-FOLLOWUP.md Task 1.5: force each of the nine substitution
// points the audit found, and confirm that not one of them can be
// classified "primary" (indistinguishable from a genuine model decision),
// and that MatchResult.ModelAuthored reads (0, true) -- "0% authored",
// measured -- for the affected player. This test fails on pre-provenance
// `main` for all nine cases; that is its acceptance criterion.
func TestSubstitutedDecisionsNeverSerializeAsPrimary(t *testing.T) {
	cases := []struct {
		name     string
		role     string
		playerID func(g *Game) string
		run      func(g *Game, playerID string)
	}{
		{
			name:     "http_failure_tower",
			role:     "defender",
			playerID: func(g *Game) string { return g.Player1 },
			run: func(g *Game, playerID string) {
				decision := map[string]interface{}{"action": "save", "reason": "provider request failed"}
				markDecisionSource(decision, SourceProviderFailure)
				g.pendingTurnResults <- turnResult{playerID: playerID, role: "defender", decision: decision, err: errors.New("stub http failure")}
				g.processPendingTurnResults()
			},
		},
		{
			name:     "http_failure_enemy",
			role:     "attacker",
			playerID: func(g *Game) string { return g.Player2 },
			run: func(g *Game, playerID string) {
				decision := map[string]interface{}{"action": "save", "reason": "provider request failed"}
				markDecisionSource(decision, SourceProviderFailure)
				g.pendingTurnResults <- turnResult{playerID: playerID, role: "attacker", decision: decision, err: errors.New("stub http failure")}
				g.processPendingTurnResults()
			},
		},
		{
			name:     "unparseable_tower",
			role:     "defender",
			playerID: func(g *Game) string { return g.Player1 },
			run: func(g *Game, playerID string) {
				decision, _ := (&OpenAIHandler{}).parseTowerResponse("I think I should build something")
				g.applyDecision(playerID, "defender", decision)
			},
		},
		{
			name:     "unparseable_enemy",
			role:     "attacker",
			playerID: func(g *Game) string { return g.Player2 },
			run: func(g *Game, playerID string) {
				decision, _ := (&GeminiHandler{}).parseEnemyResponse("```\nno json here\n```")
				g.applyDecision(playerID, "attacker", decision)
			},
		},
		{
			name:     "empty_enemy",
			role:     "attacker",
			playerID: func(g *Game) string { return g.Player2 },
			run: func(g *Game, playerID string) {
				decision, _ := (&GeminiHandler{}).parseEnemyResponse("")
				g.applyDecision(playerID, "attacker", decision)
			},
		},
		{
			name:     "unknown_action",
			role:     "defender",
			playerID: func(g *Game) string { return g.Player1 },
			run: func(g *Game, playerID string) {
				decision, _ := (&OpenAIHandler{}).parseTowerResponse(`{"action":"fortify"}`)
				g.applyDecision(playerID, "defender", decision)
			},
		},
		{
			name:     "bad_tower_type",
			role:     "defender",
			playerID: func(g *Game) string { return g.Player1 },
			run: func(g *Game, playerID string) {
				decision, _ := (&OpenAIHandler{}).parseTowerResponse(`{"action":"place","tower_type":"laser","position":[5,5]}`)
				g.applyDecision(playerID, "defender", decision)
			},
		},
		{
			name:     "bad_enemy_type",
			role:     "attacker",
			playerID: func(g *Game) string { return g.Player2 },
			run: func(g *Game, playerID string) {
				decision, _ := (&GeminiHandler{}).parseEnemyResponse(`{"action":"spawn","enemy_type":"dragon"}`)
				g.applyDecision(playerID, "attacker", decision)
			},
		},
		{
			name:     "missing_position",
			role:     "defender",
			playerID: func(g *Game) string { return g.Player1 },
			run: func(g *Game, playerID string) {
				decision, _ := (&OpenAIHandler{}).parseTowerResponse(`{"action":"place","tower_type":"basic"}`)
				g.applyDecision(playerID, "defender", decision)
			},
		},
	}

	if len(cases) != 9 {
		t.Fatalf("expected exactly nine forced failure modes, got %d", len(cases))
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := newProvenanceGame()
			playerID := c.playerID(g)
			for i := 0; i < 3; i++ {
				c.run(g, playerID)
			}

			for _, ev := range g.ReplayEvents {
				if ev.Type != ReplayOutcome || ev.PlayerID != playerID {
					continue
				}
				if q, _ := ev.Details["quality"].(string); q == "primary" {
					t.Errorf("%s: a substituted decision serialized as primary (reason=%q)", c.name, ev.Reason)
				}
			}

			res := g.BuildMatchResult()
			share, ok := res.ModelAuthored(playerID)
			if !ok || share != 0 {
				t.Errorf("%s: model_authored_share = %v (ok=%v), want (0, true)", c.name, share, ok)
			}
		})
	}
}

// TestModelAuthoredShareIsOneForCleanRun guards against over-tagging: a run
// where every decision is well-formed must report 100% authored and no
// "substituted" outcomes.
func TestModelAuthoredShareIsOneForCleanRun(t *testing.T) {
	g := newProvenanceGame()
	for i := 0; i < 5; i++ {
		g.applyDecision(g.Player1, "defender", map[string]interface{}{"action": "save"})
		g.applyDecision(g.Player2, "attacker", map[string]interface{}{"action": "save"})
	}

	res := g.BuildMatchResult()
	for _, p := range []string{g.Player1, g.Player2} {
		share, ok := res.ModelAuthored(p)
		if !ok || share != 1 {
			t.Fatalf("expected a clean run to be 100%% authored for %s, got share=%v ok=%v (sources=%v)", p, share, ok, res.DecisionSources)
		}
	}
	for _, ev := range g.ReplayEvents {
		if ev.Type != ReplayOutcome {
			continue
		}
		if q, _ := ev.Details["quality"].(string); q == "substituted" {
			t.Fatalf("expected no substituted outcomes on a clean run, got one: %#v", ev)
		}
	}
}

// TestScriptedDuelIsFullyModelSourced protects the balance-sweep determinism
// gate: scripted providers never touch the parser or HTTP path (they hand
// back a hand-built decision map directly), so every decision in a scripted
// duel must be attributed to the model, never substituted.
func TestScriptedDuelIsFullyModelSourced(t *testing.T) {
	result := RunScriptedDuel(ScriptedDuelConfig{
		Seed:           1,
		MaxTicks:       400,
		Ruleset:        BaselineDuelRuleset(),
		Balance:        DefaultBalanceConfig(),
		DefenderScript: "defender_baseline",
		AttackerScript: "attacker_baseline",
	})
	for _, p := range []string{result.Defender, result.Attacker} {
		share, ok := result.ModelAuthored(p)
		if !ok || share != 1 {
			t.Fatalf("expected a scripted duel to be 100%% model-sourced for %s, got share=%v ok=%v (sources=%v)", p, share, ok, result.DecisionSources)
		}
	}
}

// --- MatchResult.ModelAuthored: absence of provenance is not evidence ------

// TestModelAuthoredNotMeasuredWithoutProvenance simulates a MatchResult
// unmarshaled from a pre-provenance JSON file (or any MatchResult built
// before this code existed): no decision_sources, no provenance_version.
// It must read as "not measured", never as "0% authored".
func TestModelAuthoredNotMeasuredWithoutProvenance(t *testing.T) {
	legacy := MatchResult{Defender: "p1", Attacker: "p2"}

	share, ok := legacy.ModelAuthored("p1")
	if ok {
		t.Fatalf("expected a provenance-less result to report unmeasured (ok=false), got share=%v ok=true", share)
	}
	if share != 0 {
		t.Fatalf("expected the zero value 0 alongside ok=false, got %v", share)
	}
}

func TestModelAuthoredZeroDecisionsIsAlsoNotMeasured(t *testing.T) {
	// ProvenanceVersion 1 but no decisions recorded for this player at all --
	// still cannot claim a percentage.
	r := MatchResult{ProvenanceVersion: 1, DecisionSources: map[string]int{}}
	if _, ok := r.ModelAuthored("p1"); ok {
		t.Fatalf("expected zero recorded decisions to report unmeasured")
	}
}
