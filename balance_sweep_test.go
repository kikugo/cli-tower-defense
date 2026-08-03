package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	eng "tower-defense/engine"
)

// TestFormatModelAuthoredShareNotMeasured covers the critical semantics from
// engine.ModelAuthored: a MatchResult with ProvenanceVersion == 0 (the Go
// zero value, as on every pre-provenance result) must never render as "0%".
func TestFormatModelAuthoredShareNotMeasured(t *testing.T) {
	r := eng.MatchResult{
		ProvenanceVersion: 0,
		DecisionSources:   map[string]int{},
	}
	got := formatModelAuthoredShare(r, "p1")
	if got == "0%" {
		t.Fatalf("not-measured result rendered as 0%%, want a non-numeric marker like 'not measured'")
	}
	if strings.Contains(got, "%") {
		t.Fatalf("not-measured result rendered with a %% sign: %q", got)
	}
}

// TestFormatModelAuthoredShareMeasured sanity-checks the measured path still
// renders a percentage once provenance is recorded.
func TestFormatModelAuthoredShareMeasured(t *testing.T) {
	r := eng.MatchResult{
		ProvenanceVersion: 1,
		DecisionSources: map[string]int{
			"p1:" + string(eng.SourceModel): 3,
			"p1:fallback":                   1,
		},
	}
	got := formatModelAuthoredShare(r, "p1")
	if got != "75%" {
		t.Fatalf("formatModelAuthoredShare = %q, want 75%%", got)
	}
}

// TestRunBalanceSweepRejectsUnknownField exercises the exact failure mode
// this project already hit once: a misplaced/unknown key (here, "ruleset"
// nested inside a candidate instead of at the top level) must be a loud
// error, not a silently-discarded field.
func TestRunBalanceSweepRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "malformed_sweep.json")
	malformed := `{
		"seeds": [1, 2],
		"max_ticks": 50,
		"defender_script": "defender_baseline",
		"attacker_script": "attacker_baseline",
		"candidates": [
			{
				"name": "bad",
				"balance": {},
				"ruleset": {"map_type": "straight"}
			}
		]
	}`
	if err := os.WriteFile(path, []byte(malformed), 0600); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}

	err := runBalanceSweep(path)
	if err == nil {
		t.Fatalf("expected an error for a config with an unknown/misplaced key, got nil")
	}
	if !strings.Contains(err.Error(), "ruleset") {
		t.Fatalf("expected error to name the offending key %q, got: %v", "ruleset", err)
	}
}

// TestRunBalanceSweepAcceptsValidConfig confirms a well-formed sweep config
// (top-level ruleset omitted, so the correct default applies) still parses
// and runs without error after switching to DisallowUnknownFields.
func TestRunBalanceSweepAcceptsValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "valid_sweep.json")
	valid := `{
		"seeds": [1, 2],
		"max_ticks": 50,
		"defender_script": "defender_baseline",
		"attacker_script": "attacker_baseline",
		"candidates": [
			{"name": "baseline", "balance": {}}
		]
	}`
	if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
		t.Fatalf("write valid config: %v", err)
	}

	if err := runBalanceSweep(path); err != nil {
		t.Fatalf("expected valid config to parse and run without error, got: %v", err)
	}
}
