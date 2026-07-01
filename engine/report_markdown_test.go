package engine

import (
	"strings"
	"testing"
)

func sampleMatchResult() MatchResult {
	return MatchResult{
		Winner:      "p1",
		WinnerModel: "gpt-4.1-mini",
		WinReason:   "max_waves_cleared",
		Ticks:       1200,
		Waves:       10,
		MaxWaves:    10,
		Defender:    "p1",
		Attacker:    "p2",
		Models:      map[string]string{"p1": "gpt-4.1-mini", "p2": "gemini-2.5-pro"},
		Lives:       map[string]int{"p1": 12, "p2": 20},
		Score:       map[string]int{"p1": 340, "p2": 180},
		NormalizedScore: map[string]float64{"p1": 0.65, "p2": 0.35},
		ProviderCalls:   map[string]int{"p1": 40, "p2": 38},
		ProviderLatency: map[string]float64{"p1": 820, "p2": 910},
		TokenUsage:      map[string]int{"p1": 5000, "p2": 5200},
		CostMicros:      map[string]int64{"p1": 8000, "p2": 0},
		RejectedActions: map[string]int{"p1:place": 2, "p2:spawn": 1},
		ProviderErrors:  map[string]int{"p2:status": 3},
		DurationMillis:  65000,
		ReplayEvents:    412,
	}
}

func TestMarkdownReportContainsKeyFields(t *testing.T) {
	md := sampleMatchResult().MarkdownReport()

	wants := []string{
		"# Match Report: gpt-4.1-mini wins",
		"**Win reason:** max waves cleared",
		"**Waves:** 10 / 10",
		"gpt-4.1-mini",
		"gemini-2.5-pro",
		"defender",
		"attacker",
		"$0.0080", // p1 cost from 8000 micros
	}
	for _, w := range wants {
		if !strings.Contains(md, w) {
			t.Fatalf("expected report to contain %q\n---\n%s", w, md)
		}
	}
}

func TestMarkdownReportRejectedAndErrorTotals(t *testing.T) {
	md := sampleMatchResult().MarkdownReport()
	// p1 defender row should show 2 rejected, 0 errors; p2 attacker 1 rejected, 3 errors.
	lines := strings.Split(md, "\n")
	var p1Row, p2Row string
	for _, ln := range lines {
		if strings.Contains(ln, "| p1 |") {
			p1Row = ln
		}
		if strings.Contains(ln, "| p2 |") {
			p2Row = ln
		}
	}
	if p1Row == "" || p2Row == "" {
		t.Fatalf("expected player rows, got:\n%s", md)
	}
	if !strings.HasSuffix(strings.TrimSpace(p1Row), "| 2 | 0 |") {
		t.Fatalf("p1 row rejected/errors wrong: %q", p1Row)
	}
	if !strings.HasSuffix(strings.TrimSpace(p2Row), "| 1 | 3 |") {
		t.Fatalf("p2 row rejected/errors wrong: %q", p2Row)
	}
}

func TestMarkdownReportEmptyResult(t *testing.T) {
	md := MatchResult{}.MarkdownReport()
	if !strings.Contains(md, "# Match Report") {
		t.Fatalf("expected default title, got:\n%s", md)
	}
}
