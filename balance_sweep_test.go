package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
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

// --- stratified reporting -------------------------------------------------

// mkResult builds a minimal eng.MatchResult for stratification tests: a
// defender win/loss, a lanes stratum tag (or none), and fixed ticks/score so
// averages are easy to hand-check.
func mkResult(defenderWon bool, lanes string, ticks int64, score int) eng.MatchResult {
	r := eng.MatchResult{
		Defender: "p1",
		Ticks:    ticks,
		Score:    map[string]int{"p1": score},
	}
	if defenderWon {
		r.Winner = "p1"
	} else {
		r.Winner = "p2"
	}
	if lanes != "" {
		r.Strata = map[string]string{"lanes": lanes}
	}
	return r
}

// TestGroupByStratumSeparatesLaneCounts confirms a lanes=1 result and a
// lanes=2 result land in different buckets, and a result with no Strata at
// all lands in the "lanes=?" bucket rather than crashing or being dropped.
func TestGroupByStratumSeparatesLaneCounts(t *testing.T) {
	results := []eng.MatchResult{
		mkResult(true, "1", 400, 760),
		mkResult(true, "1", 400, 760),
		mkResult(false, "2", 150, 400),
		func() eng.MatchResult {
			r := mkResult(true, "", 999, 999)
			r.Strata = nil
			return r
		}(),
	}
	buckets, keys := groupByStratum(results)

	if len(keys) != 3 {
		t.Fatalf("expected 3 buckets (lanes=1, lanes=2, lanes=?), got %d: %v", len(keys), keys)
	}
	wantOrder := []string{"lanes=1", "lanes=2", "lanes=?"}
	for i, k := range wantOrder {
		if keys[i] != k {
			t.Fatalf("expected sorted key[%d] = %q, got %q (full order: %v)", i, k, keys[i], keys)
		}
	}
	if buckets["lanes=1"].n != 2 {
		t.Fatalf("expected 2 matches in lanes=1, got %d", buckets["lanes=1"].n)
	}
	if buckets["lanes=2"].n != 1 {
		t.Fatalf("expected 1 match in lanes=2, got %d", buckets["lanes=2"].n)
	}
	if buckets["lanes=?"].n != 1 {
		t.Fatalf("expected 1 match in lanes=? for a result with no Strata, got %d", buckets["lanes=?"].n)
	}
}

// TestFormatRateUnderpoweredBelowTen confirms a stratum with n < 10 renders
// "(underpowered)" rather than a percentage that implies statistical weight
// it doesn't have.
func TestFormatRateUnderpoweredBelowTen(t *testing.T) {
	got := formatRate(9, 9)
	if got != "(underpowered)" {
		t.Fatalf("formatRate(9, 9) = %q, want \"(underpowered)\"", got)
	}
	if strings.Contains(got, "%") {
		t.Fatalf("underpowered rate must not contain a %%, got %q", got)
	}
}

// TestFormatRateAtTenIsPowered confirms n == 10 is the boundary: 10 is
// powered enough to render a percentage.
func TestFormatRateAtTenIsPowered(t *testing.T) {
	got := formatRate(10, 5)
	if got != "50%" {
		t.Fatalf("formatRate(10, 5) = %q, want \"50%%\"", got)
	}
}

// TestFormatMixtureRowRequiresComposition proves, at the signature level,
// that a MIXTURE row cannot be produced without stating its composition:
// formatMixtureRow takes composition as a required positional argument, and
// this test additionally verifies the empty-string case panics rather than
// silently printing a bare row, and that a real composition string actually
// appears in the rendered output.
func TestFormatMixtureRowRequiresComposition(t *testing.T) {
	agg := stratumStats{key: "MIXTURE", n: 40, wins: 25, totalTicks: 40 * 315, totalScore: 40 * 655}

	defer func() {
		if recover() == nil {
			t.Fatalf("expected formatMixtureRow to panic on an empty composition, it did not")
		}
	}()
	_ = formatMixtureRow("defaults", agg, "")
}

// TestFormatMixtureRowIncludesComposition checks the non-panicking path: the
// rendered MIXTURE row must contain the composition string verbatim.
func TestFormatMixtureRowIncludesComposition(t *testing.T) {
	agg := stratumStats{key: "MIXTURE", n: 40, wins: 25, totalTicks: 40 * 315, totalScore: 40 * 655}
	composition := "lanes=1:60% lanes=2:40%"
	row := formatMixtureRow("defaults", agg, composition)
	if !strings.Contains(row, composition) {
		t.Fatalf("expected MIXTURE row to contain composition %q, got: %s", composition, row)
	}
	if !strings.Contains(row, "MIXTURE") {
		t.Fatalf("expected MIXTURE row to be labelled MIXTURE, got: %s", row)
	}
}

// TestDominantStratumShareSuppressesUnbalancedMixture confirms a candidate
// whose matches are more than 90%% one stratum is flagged as unbalanced, so
// runBalanceSweep will print no MIXTURE row for it.
func TestDominantStratumShareSuppressesUnbalancedMixture(t *testing.T) {
	results := make([]eng.MatchResult, 0, 40)
	for i := 0; i < 37; i++ {
		results = append(results, mkResult(true, "1", 400, 760))
	}
	for i := 0; i < 3; i++ {
		results = append(results, mkResult(false, "2", 150, 400))
	}
	buckets, keys := groupByStratum(results)
	dominantKey, share := dominantStratumShare(buckets, keys, len(results))
	if dominantKey != "lanes=1" {
		t.Fatalf("expected lanes=1 to dominate, got %q", dominantKey)
	}
	if share <= unbalancedShareThreshold {
		t.Fatalf("expected dominant share > %.2f, got %.4f", unbalancedShareThreshold, share)
	}
}

// TestDominantStratumShareBalancedDesignAllowsMixture confirms a roughly
// balanced 60/40 split (like the real defaults sweep) does NOT trip the
// unbalanced-design suppression.
func TestDominantStratumShareBalancedDesignAllowsMixture(t *testing.T) {
	results := make([]eng.MatchResult, 0, 40)
	for i := 0; i < 24; i++ {
		results = append(results, mkResult(true, "1", 400, 760))
	}
	for i := 0; i < 16; i++ {
		results = append(results, mkResult(false, "2", 150, 400))
	}
	buckets, keys := groupByStratum(results)
	_, share := dominantStratumShare(buckets, keys, len(results))
	if share > unbalancedShareThreshold {
		t.Fatalf("expected a 60/40 split to stay under the %.2f unbalanced threshold, got %.4f", unbalancedShareThreshold, share)
	}
}

// TestRunBalanceSweepDefaultConfigPrintsMixtureCompositionAndUnderpowered
// runs the real sweep entry point end-to-end against a 40-seed default
// config -- the same shape as the published balance sweep -- and checks the
// printed table contains a MIXTURE row carrying its composition, and that
// the 16-match lanes=2 stratum (below minStratumSample) is not rendered as
// a bare percentage.
func TestRunBalanceSweepDefaultConfigPrintsMixtureCompositionAndUnderpowered(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "default_sweep.json")
	// Build the JSON seeds array directly to avoid pulling in encoding/json
	// for a fixed 1..40 list.
	seedsJSON := "["
	for i := 1; i <= 40; i++ {
		if i > 1 {
			seedsJSON += ","
		}
		seedsJSON += strconv.Itoa(i)
	}
	seedsJSON += "]"

	cfg := `{
		"seeds": ` + seedsJSON + `,
		"max_ticks": 400,
		"defender_script": "defender_baseline",
		"attacker_script": "attacker_baseline",
		"candidates": [
			{"name": "defaults", "balance": {}}
		]
	}`
	if err := os.WriteFile(path, []byte(cfg), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout := captureStdout(t, func() {
		if err := runBalanceSweep(path); err != nil {
			t.Fatalf("runBalanceSweep: %v", err)
		}
	})

	if !strings.Contains(stdout, "MIXTURE") {
		t.Fatalf("expected a MIXTURE row in output, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "[lanes=") {
		t.Fatalf("expected the MIXTURE row to carry its composition (a \"[lanes=...\" fragment), got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "lanes=1") || !strings.Contains(stdout, "lanes=2") {
		t.Fatalf("expected per-stratum lanes=1 and lanes=2 rows, got:\n%s", stdout)
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns what
// was written, so tests can assert on runBalanceSweep's printed table.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return buf.String()
}
