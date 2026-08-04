package main

import (
	"reflect"
	"testing"

	eng "tower-defense/engine"
)

// newOrderTestGame builds a *eng.Game the same way runTournamentMatch does
// (via eng.ResolveMatchConfig + eng.NewGameFromResolvedConfig), using
// offline scripted providers so the test needs no API keys, no .env, and no
// network access.
func newOrderTestGame(t *testing.T) *eng.Game {
	t.Helper()
	matchConfig := eng.MatchConfig{
		Player1: eng.PlayerModelConfig{Provider: eng.ProviderScripted, Model: "defender_baseline", APIKeyEnv: "NONE"},
		Player2: eng.PlayerModelConfig{Provider: eng.ProviderScripted, Model: "attacker_baseline", APIKeyEnv: "NONE"},
	}
	resolved, err := eng.ResolveMatchConfig(matchConfig)
	if err != nil {
		t.Fatalf("ResolveMatchConfig: %v", err)
	}
	return eng.NewGameFromResolvedConfig(resolved)
}

func assertSameMap(t *testing.T, label string, a, b *eng.Game) {
	t.Helper()
	if !reflect.DeepEqual(a.Paths, b.Paths) {
		t.Fatalf("%s: paths differ\nA: %v\nB: %v", label, a.Paths, b.Paths)
	}
	if !reflect.DeepEqual(a.Obstacles, b.Obstacles) {
		t.Fatalf("%s: obstacles differ (counts %d vs %d)\nA: %v\nB: %v", label, len(a.Obstacles), len(b.Obstacles), a.Obstacles, b.Obstacles)
	}
}

// TestGameConfigurationSeedAppliesLast guards against a bug where the
// generated map depended on the order flags were applied, not only on the
// seed: initialModel() (TUI/headless entry point) used to call
// SetRandomSeed(seed) first and SetMapType/ApplyRuleset afterward, while
// runTournamentMatch() applied ruleset/map-type configuration first and
// seeded last. Since SetMapType and ApplyRuleset (when a ruleset sets a map
// type) both regenerate the map from whatever RNG state exists at call
// time, and SetRandomSeed replaces the RNG and regenerates again, seeding
// at a different point in the sequence produced a different map for an
// identical (seed, map type) pair -- while writing byte-identical manifests,
// since manifests don't record the map itself.
//
// Both entry points now build games through applyGameConfiguration, which
// always applies the seed last, after every map-type/ruleset call. These
// tests build games the way each entry point does and assert identical
// Paths and identical Obstacles -- not just matching counts, since two maps
// can share an obstacle count and differ entirely in position.
func TestGameConfigurationSeedAppliesLast(t *testing.T) {
	const seed = int64(11)

	t.Run("seed and map-type", func(t *testing.T) {
		// TUI/headless path: -seed 11 -map-type choke, no ruleset flags.
		// initialModel passes mapType through the mapType parameter.
		tui := newOrderTestGame(t)
		applyGameConfiguration(tui, "choke", 0, nil, seed)

		// Tournament path: seed 11, map type supplied via config.Ruleset --
		// the tournament runner has no -map-type flag of its own, only a
		// ruleset JSON that can set map_type. runTournamentMatch passes
		// this through the rulesets parameter, not mapType.
		tournament := newOrderTestGame(t)
		applyGameConfiguration(tournament, "", 0, []eng.ArenaRuleset{{MapType: "choke"}}, seed)

		assertSameMap(t, "seed+map-type (TUI vs tournament)", tui, tournament)

		// Sanity control: a different seed with the same map type must NOT
		// produce the same map, or this test would be vacuous.
		other := newOrderTestGame(t)
		applyGameConfiguration(other, "choke", 0, nil, seed+1)
		if reflect.DeepEqual(tui.Obstacles, other.Obstacles) {
			t.Fatalf("sanity check failed: different seeds produced identical obstacles %v", tui.Obstacles)
		}
	})

	t.Run("seed, ruleset-preset, and map-type", func(t *testing.T) {
		preset, err := eng.PresetArenaRuleset("fast")
		if err != nil {
			t.Fatalf("PresetArenaRuleset: %v", err)
		}

		// Reference: -seed 11 -map-type choke -ruleset-preset fast, applied
		// through applyGameConfiguration exactly as initialModel does.
		reference := newOrderTestGame(t)
		applyGameConfiguration(reference, "choke", 0, []eng.ArenaRuleset{preset}, seed)

		// Same effective configuration, but reached after extra, redundant
		// map-type/ruleset calls happened first (as if a different code
		// path -- or a future caller supplying -ruleset-preset and
		// -ruleset together -- had already consumed additional RNG draws
		// via its own SetMapType/ApplyRuleset calls before ever reaching
		// applyGameConfiguration). The number and order of these prior
		// calls must not affect the final map, because the seed is applied
		// last, always, and SetRandomSeed regenerates from a fresh RNG.
		variant := newOrderTestGame(t)
		variant.ApplyRuleset(preset)
		variant.SetMapType("choke")
		variant.ApplyRuleset(preset)
		applyGameConfiguration(variant, "choke", 0, []eng.ArenaRuleset{preset}, seed)

		assertSameMap(t, "seed+ruleset-preset+map-type (extra prior draws)", reference, variant)
	})

	t.Run("seed alone", func(t *testing.T) {
		// No map-type, no ruleset -- this already worked before the fix
		// and must keep working.
		a := newOrderTestGame(t)
		applyGameConfiguration(a, "", 0, nil, seed)

		b := newOrderTestGame(t)
		applyGameConfiguration(b, "", 0, nil, seed)

		assertSameMap(t, "seed alone", a, b)
	})
}
