package engine

import (
	"reflect"
	"strings"
	"testing"
)

// The engine holds simulation state. It does not hold presentation state.
//
// It used to hold both, and the cost was concrete: Entity.Char gave the
// engine a second opinion about what a sniper looks like, which is how the
// codebase ended up with three glyph tables that disagreed; and the particle
// system spawned and aged hit-effect objects on every tick for a renderer
// that had already been deleted.
//
// The tests below are the guard on that boundary. They are deliberately
// written against the type system rather than against a rendered frame,
// because the failure they exist to catch is someone re-adding a display
// field to engine state -- which no output test would notice until a second
// vocabulary had grown around it.

// presentationFieldNames are field names that mean "how this is drawn",
// which is a UI concern. The UI's own vocabulary lives in glyphs_v2.go and
// render_theme_v2.go in package main.
var presentationFieldNames = []string{"char", "glyph", "color", "colour", "style", "icon", "sprite"}

// TestEngineTypesCarryNoPresentationFields walks the simulation's own board
// types and fails on any field whose name says it exists to be looked at.
func TestEngineTypesCarryNoPresentationFields(t *testing.T) {
	types := map[string]reflect.Type{
		"Entity":   reflect.TypeOf(Entity{}),
		"Tower":    reflect.TypeOf(Tower{}),
		"Enemy":    reflect.TypeOf(Enemy{}),
		"SlowZone": reflect.TypeOf(SlowZone{}),
		"Position": reflect.TypeOf(Position{}),
	}

	for name, typ := range types {
		for i := 0; i < typ.NumField(); i++ {
			field := strings.ToLower(typ.Field(i).Name)
			for _, banned := range presentationFieldNames {
				if field == banned {
					t.Fatalf("%s has a presentation field %q -- glyphs and colours belong to the UI (glyphs_v2.go), not to simulation state",
						name, typ.Field(i).Name)
				}
			}
		}
	}
}

// TestGameCarriesNoVisualEffectState is the particle system's headstone.
//
// Particles were written on every heal and every kill and aged out on every
// tick, for a renderer deleted in the UI cutover -- pure work in the hot
// loop with no consumer and, since the redesign draws terrain plus units and
// nothing else, no planned one. This checks Game grew no replacement.
func TestGameCarriesNoVisualEffectState(t *testing.T) {
	typ := reflect.TypeOf(Game{})
	for i := 0; i < typ.NumField(); i++ {
		field := strings.ToLower(typ.Field(i).Name)
		for _, banned := range []string{"particles", "effects", "animations", "flashes"} {
			if field == banned {
				t.Fatalf("Game has a visual-effect field %q -- the engine simulates, the UI draws", typ.Field(i).Name)
			}
		}
	}
}

// TestRemovingParticlesLeftTheSimulationIntact checks the two sites the
// particle appends were removed from still do their actual jobs: a healer
// adjacent to a damaged ally heals it and goes on cooldown, and a tower kill
// still records its ReplayDamage event.
//
// This is the part worth testing about a deletion. "The field is gone" is
// checked above by reflection; what a reflection test cannot catch is an
// edit that took a line of real behaviour out with the dead line beside it.
func TestRemovingParticlesLeftTheSimulationIntact(t *testing.T) {
	g := NewGame("p1", "p2")
	g.SetRandomSeed(1)

	// A healer with a wounded ally in range.
	wounded := &Enemy{
		Entity:    Entity{Pos: Position{Y: 5, X: 10}, Health: 50, MaxHealth: 100},
		EnemyType: "basic",
	}
	healer := &Enemy{
		Entity:    Entity{Pos: Position{Y: 5, X: 11}, Health: 100, MaxHealth: 100},
		EnemyType: "healer",
	}
	g.Enemies = []*Enemy{wounded, healer}

	g.UpdateGameState()

	if wounded.Health <= 50 {
		t.Fatalf("healer did not heal: ally health %d, want > 50", wounded.Health)
	}
	if healer.Cooldown <= 0 {
		t.Fatalf("healer did not go on cooldown after healing: %d", healer.Cooldown)
	}
}

// TestTowerKillStillRecordsADamageEvent covers the other removal site: the
// ReplayDamage record sat inside the same loop as the kill particle.
func TestTowerKillStillRecordsADamageEvent(t *testing.T) {
	g := NewGame("p1", "p2")
	g.SetRandomSeed(1)

	spots := g.validTowerCandidates(1)
	if len(spots) == 0 {
		t.Fatal("setup: no valid tower placement on the seeded map")
	}
	if !g.placeTower(spots[0][0], spots[0][1], "basic") {
		t.Fatalf("setup: could not place a tower at %v", spots[0])
	}
	tower := g.Towers[len(g.Towers)-1]

	// A one-hit-from-death enemy standing on the tower.
	g.Enemies = []*Enemy{{
		Entity:    Entity{Pos: tower.Pos, Health: 1, MaxHealth: 100},
		EnemyType: "basic",
	}}

	before := len(g.ReplayEvents)
	g.UpdateGameState()

	found := false
	for _, ev := range g.ReplayEvents[before:] {
		if ev.Type == ReplayDamage {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("a tower kill recorded no ReplayDamage event -- the removal took a live line with it")
	}
}
