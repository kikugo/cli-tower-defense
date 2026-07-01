package engine

import (
	"strings"
	"testing"
)

func TestTournamentMarkdownReport(t *testing.T) {
	report := TournamentReport{
		Name: "smoke",
		Results: []TournamentMatchResult{
			{Matchup: "A vs B", Seed: 1, Swapped: false, Result: MatchResult{
				Winner: "p2", WinnerModel: "beta", WinReason: "defender_lives_depleted", Waves: 3, MaxWaves: 5,
				Models: map[string]string{"p1": "alpha", "p2": "beta"},
			}},
			{Matchup: "A vs B", Seed: 1, Swapped: true, Result: MatchResult{
				Winner: "p1", WinnerModel: "alpha", WinReason: "max_waves_cleared", Waves: 5, MaxWaves: 5,
				Models: map[string]string{"p1": "alpha", "p2": "beta"},
			}},
		},
		Standings: []TournamentStanding{
			{Model: "alpha", Matches: 2, Wins: 1, WinRate: 0.5, AverageNormalized: 0.6, AverageScore: 200, AverageWaveReached: 4},
			{Model: "beta", Matches: 2, Wins: 1, WinRate: 0.5, AverageNormalized: 0.4, AverageScore: 150, AverageWaveReached: 4},
		},
		Ratings: map[string]float64{"alpha": 1512, "beta": 1488},
	}

	md := report.MarkdownReport()

	wants := []string{
		"# Tournament: smoke",
		"## Standings",
		"## Matches",
		"alpha",
		"beta",
		"defender lives depleted",
		"max waves cleared",
		"1512", // alpha rating
	}
	for _, w := range wants {
		if !strings.Contains(md, w) {
			t.Fatalf("expected report to contain %q\n---\n%s", w, md)
		}
	}

	// alpha has higher normalized score, so it should rank first in standings.
	standingsIdx := strings.Index(md, "## Standings")
	matchesIdx := strings.Index(md, "## Matches")
	standingsSection := md[standingsIdx:matchesIdx]
	if strings.Index(standingsSection, "alpha") > strings.Index(standingsSection, "beta") {
		t.Fatalf("expected alpha ranked before beta in standings:\n%s", standingsSection)
	}
}

func TestTournamentMarkdownReportEmpty(t *testing.T) {
	md := TournamentReport{}.MarkdownReport()
	if !strings.Contains(md, "# Tournament: Tournament") {
		t.Fatalf("expected default title, got:\n%s", md)
	}
}
