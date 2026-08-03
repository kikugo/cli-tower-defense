package engine

import "testing"

// TestMatchStrataReportsRealisedLaneCount is the whole point of this
// feature: a ruleset's map_type tells you nothing about how many lanes a
// seeded-random match actually generated, but Strata["lanes"] must. Seeds
// are chosen deterministically by first checking what len(g.Paths) the
// engine's default generator actually produces at that seed -- never
// hardcoded blind.
func TestMatchStrataReportsRealisedLaneCount(t *testing.T) {
	oneLaneGame := NewGame("test", "test")
	oneLaneGame.SetRandomSeed(2)
	if got := len(oneLaneGame.Paths); got != 1 {
		t.Fatalf("seed 2 expected to generate 1 lane (test assumption stale), got %d", got)
	}
	result := oneLaneGame.BuildMatchResult()
	if result.Strata["lanes"] != "1" {
		t.Fatalf("expected lanes=1 for a 1-lane game, got %q", result.Strata["lanes"])
	}

	twoLaneGame := NewGame("test", "test")
	twoLaneGame.SetRandomSeed(1)
	if got := len(twoLaneGame.Paths); got != 2 {
		t.Fatalf("seed 1 expected to generate 2 lanes (test assumption stale), got %d", got)
	}
	result = twoLaneGame.BuildMatchResult()
	if result.Strata["lanes"] != "2" {
		t.Fatalf("expected lanes=2 for a 2-lane game, got %q", result.Strata["lanes"])
	}
}

// TestMatchStrataMapTypeRendersRandomForEmpty verifies the empty-string
// map_type (the seeded-random generator) is never reported as a blank key --
// it renders as "random" -- while an explicit map type is reported verbatim
// alongside whatever lane count it actually realised.
func TestMatchStrataMapTypeRendersRandomForEmpty(t *testing.T) {
	randomGame := NewGame("test", "test")
	randomGame.SetRandomSeed(2)
	result := randomGame.BuildMatchResult()
	if result.Strata["map_type"] != "random" {
		t.Fatalf("expected map_type=random for empty MapType, got %q", result.Strata["map_type"])
	}

	forkedGame := NewGame("test", "test")
	forkedGame.SetMapType("forked")
	if got := len(forkedGame.Paths); got != 2 {
		t.Fatalf("forked map type expected to generate 2 lanes (test assumption stale), got %d", got)
	}
	result = forkedGame.BuildMatchResult()
	if result.Strata["map_type"] != "forked" {
		t.Fatalf("expected map_type=forked, got %q", result.Strata["map_type"])
	}
	if result.Strata["lanes"] != "2" {
		t.Fatalf("expected lanes=2 for forked map, got %q", result.Strata["lanes"])
	}
}

// TestMatchStrataIncludesBalanceVersion checks the third documented key.
func TestMatchStrataIncludesBalanceVersion(t *testing.T) {
	g := NewGame("test", "test")
	result := g.BuildMatchResult()
	if result.Strata["balance"] != g.Balance.Version {
		t.Fatalf("expected balance=%q, got %q", g.Balance.Version, result.Strata["balance"])
	}
	if result.Strata["balance"] == "" {
		t.Fatalf("expected non-empty balance version")
	}
}

// TestMatchStrataIsACopy proves Strata does not alias live game state: it
// must follow the same copy semantics as copyIntMap/copyStringMap so a
// caller mutating a previously-built MatchResult can't corrupt subsequent
// BuildMatchResult() calls on the same Game.
func TestMatchStrataIsACopy(t *testing.T) {
	g := NewGame("test", "test")

	first := g.BuildMatchResult()
	first.Strata["lanes"] = "corrupted"
	first.Strata["map_type"] = "corrupted"
	first.Strata["balance"] = "corrupted"

	second := g.BuildMatchResult()
	if second.Strata["lanes"] == "corrupted" || second.Strata["map_type"] == "corrupted" || second.Strata["balance"] == "corrupted" {
		t.Fatalf("expected Strata to be an independent copy, got %#v", second.Strata)
	}
}
