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

// --- recorded-measurement columns ------------------------------------------
//
// These cover the whole reason this file exists: a change that leaves the
// four simulation numbers above (n, wins, ticks, score) bit-identical can
// still silently alter what the arena recorded -- rejections, provider
// calls, model-authored share. See readme.md and
// engine/forced_save_skip_test.go's TestSkipForcedSaveTurnsEliminatesRejectionsWhenEnabled
// for the real change that motivated adding these columns.

// TestGroupByStratumAggregatesRecordedMeasurements hand-builds two
// eng.MatchResult values with known RejectedActions, ProviderCalls, and
// DecisionSources and checks stratumStats sums them correctly: rejections
// split by defender/attacker and totalled, provider calls summed across both
// players, and the model-authored aggregate correctly excluding the match
// whose provenance was never recorded (ProvenanceVersion == 0).
func TestGroupByStratumAggregatesRecordedMeasurements(t *testing.T) {
	measured := eng.MatchResult{
		Defender: "p1", Attacker: "p2",
		Winner: "p1", Ticks: 400, Score: map[string]int{"p1": 760},
		Strata:          map[string]string{"lanes": "1"},
		RejectedActions: map[string]int{"p1:place_tower": 3, "p2:launch_wave": 2},
		ProviderCalls:   map[string]int{"p1": 10, "p2": 8},
		ProvenanceVersion: 1,
		DecisionSources: map[string]int{
			"p1:" + string(eng.SourceModel): 8,
			"p1:fallback":                   2,
			"p2:" + string(eng.SourceModel): 4,
			"p2:fallback":                   4,
		},
	}
	// unmeasured has real rejections/calls (like every match does) but its
	// decisions were never given provenance -- ProvenanceVersion left at the
	// Go zero value, exactly like every pre-provenance result on disk. Its
	// rejections/calls must still count; its authored share must not.
	unmeasured := eng.MatchResult{
		Defender: "p1", Attacker: "p2",
		Winner: "p1", Ticks: 400, Score: map[string]int{"p1": 700},
		Strata:          map[string]string{"lanes": "1"},
		RejectedActions: map[string]int{"p1:place_tower": 1},
		ProviderCalls:   map[string]int{"p1": 5, "p2": 5},
	}

	buckets, keys := groupByStratum([]eng.MatchResult{measured, unmeasured})
	if len(keys) != 1 || keys[0] != "lanes=1" {
		t.Fatalf("expected a single lanes=1 bucket, got %v", keys)
	}
	b := buckets["lanes=1"]

	if b.rejectedDefender != 4 { // 3 (measured) + 1 (unmeasured)
		t.Fatalf("rejectedDefender = %d, want 4", b.rejectedDefender)
	}
	if b.rejectedAttacker != 2 {
		t.Fatalf("rejectedAttacker = %d, want 2", b.rejectedAttacker)
	}
	if b.rejectedTotal() != 6 {
		t.Fatalf("rejectedTotal() = %d, want 6", b.rejectedTotal())
	}
	if b.providerCalls != 28 { // 10+8 (measured) + 5+5 (unmeasured)
		t.Fatalf("providerCalls = %d, want 28", b.providerCalls)
	}
	if b.authoredTotal != 4 { // 2 matches * 2 players each
		t.Fatalf("authoredTotal = %d, want 4", b.authoredTotal)
	}
	if b.authoredMeasured != 2 { // only `measured`'s p1 and p2 have provenance
		t.Fatalf("authoredMeasured = %d, want 2 (unmeasured match must not count)", b.authoredMeasured)
	}
	wantSum := 0.8 + 0.5 // p1: 8/10 model-authored, p2: 4/8 model-authored
	if diff := b.authoredSum - wantSum; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("authoredSum = %v, want %v", b.authoredSum, wantSum)
	}
}

// TestFormatAuthoredAggregateNotMeasuredWhenNoSamplesMeasured is the
// aggregate-level version of TestFormatModelAuthoredShareNotMeasured: zero
// measured samples (e.g. every match in a stratum has ProvenanceVersion ==
// 0) must render as "not measured", never as "0%" -- a bare 0% would read as
// "the model authored nothing," which is a claim about authorship, not about
// missing data.
func TestFormatAuthoredAggregateNotMeasuredWhenNoSamplesMeasured(t *testing.T) {
	got := formatAuthoredAggregate(0, 0, 4)
	if got == "0%" {
		t.Fatalf("zero measured samples rendered as 0%%, want a non-numeric marker like 'not measured'")
	}
	if strings.Contains(got, "%") {
		t.Fatalf("not-measured aggregate rendered with a %% sign: %q", got)
	}
}

// TestFormatAuthoredAggregatePartialMeasurementShowsCoverage confirms that
// when only some samples in a stratum were measured, the rendered aggregate
// states the coverage (measured/total) alongside the percentage, rather than
// presenting a partial average as if it were complete.
func TestFormatAuthoredAggregatePartialMeasurementShowsCoverage(t *testing.T) {
	got := formatAuthoredAggregate(1.3, 2, 4) // 65% average from 2 of 4 samples
	if !strings.Contains(got, "65%") {
		t.Fatalf("expected 65%% in %q", got)
	}
	if !strings.Contains(got, "2/4") {
		t.Fatalf("expected measured coverage \"2/4\" in %q", got)
	}
}

// TestFormatAuthoredAggregateFullCoverageOmitsCoverageNote confirms that
// when every sample was measured, the aggregate prints a bare percentage
// with no "(x/y measured)" qualifier -- that qualifier exists to flag
// incomplete coverage, and would be noise on a fully measured stratum.
func TestFormatAuthoredAggregateFullCoverageOmitsCoverageNote(t *testing.T) {
	got := formatAuthoredAggregate(3, 4, 4) // 75%, fully covered
	if got != "75%" {
		t.Fatalf("formatAuthoredAggregate(3, 4, 4) = %q, want \"75%%\"", got)
	}
}

// TestFormatStratumRowProvenanceZeroNeverRendersZeroPercent is the
// end-to-end version: a stratum built entirely from ProvenanceVersion == 0
// matches (mkResult never sets it, matching every pre-provenance result on
// disk) must never show "0%" authored in the printed row -- only
// "not measured".
func TestFormatStratumRowProvenanceZeroNeverRendersZeroPercent(t *testing.T) {
	r := mkResult(true, "1", 400, 760)
	buckets, keys := groupByStratum([]eng.MatchResult{r})
	row := formatStratumRow("cand", *buckets[keys[0]])
	if strings.Contains(row, "0%") {
		t.Fatalf("expected no bare 0%% authored figure for an unmeasured match, got:\n%s", row)
	}
	if !strings.Contains(row, "not measured") {
		t.Fatalf("expected \"not measured\" in row for a zero-provenance match, got:\n%s", row)
	}
}

// TestFormatRecordedLineReportsRejectionsAndCalls confirms formatRecordedLine
// surfaces the raw rejection/call totals a formatStratumRow/formatMixtureRow
// caller cannot get from the simulation columns alone.
func TestFormatRecordedLineReportsRejectionsAndCalls(t *testing.T) {
	s := stratumStats{
		key: "lanes=1", n: 2,
		rejectedDefender: 4, rejectedAttacker: 2, providerCalls: 28,
		authoredSum: 1.3, authoredMeasured: 2, authoredTotal: 4,
	}
	line := formatRecordedLine(s)
	for _, want := range []string{"def=4", "att=2", "total=6", "calls=28", "65%", "2/4"} {
		if !strings.Contains(line, want) {
			t.Fatalf("expected formatRecordedLine output to contain %q, got: %s", want, line)
		}
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
