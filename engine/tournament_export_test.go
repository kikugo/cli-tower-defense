package engine

import (
	"strings"
	"testing"
)

func TestSortStandingsRanksByWinRate(t *testing.T) {
	standings := []TournamentStanding{
		{Model: "b", WinRate: 0.4, AverageNormalized: 0.5},
		{Model: "a", WinRate: 0.8, AverageNormalized: 0.6},
		{Model: "c", WinRate: 0.8, AverageNormalized: 0.9},
	}
	sorted := SortStandings(standings)
	if sorted[0].Model != "c" {
		t.Fatalf("expected c first (win rate tie broken by normalized), got %s", sorted[0].Model)
	}
	if sorted[1].Model != "a" {
		t.Fatalf("expected a second, got %s", sorted[1].Model)
	}
	if sorted[2].Model != "b" {
		t.Fatalf("expected b last, got %s", sorted[2].Model)
	}
	// original slice must be untouched
	if standings[0].Model != "b" {
		t.Fatalf("SortStandings mutated input")
	}
}

func TestStandingsCSVHeaderAndRows(t *testing.T) {
	csv := StandingsCSV([]TournamentStanding{
		{Model: "alpha", Matches: 4, Wins: 3, WinRate: 0.75, AverageScore: 210.5, AverageNormalized: 0.6, AverageWaveReached: 8.5, RejectedActions: 2, ProviderErrors: 1},
		{Model: "beta", Matches: 4, Wins: 1, WinRate: 0.25, AverageScore: 90, AverageNormalized: 0.4, AverageWaveReached: 5, RejectedActions: 0, ProviderErrors: 5},
	})

	lines := strings.Split(strings.TrimSpace(csv), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 rows, got %d lines:\n%s", len(lines), csv)
	}
	if !strings.HasPrefix(lines[0], "rank,model,matches,wins,win_rate") {
		t.Fatalf("unexpected header: %s", lines[0])
	}
	// alpha has higher win rate, so it ranks first
	if !strings.HasPrefix(lines[1], "1,alpha,4,3,0.7500") {
		t.Fatalf("unexpected first row: %s", lines[1])
	}
	if !strings.HasPrefix(lines[2], "2,beta,") {
		t.Fatalf("unexpected second row: %s", lines[2])
	}
}

func TestStandingsCSVEmpty(t *testing.T) {
	csv := StandingsCSV(nil)
	if !strings.HasPrefix(csv, "rank,model") {
		t.Fatalf("expected header even when empty, got: %q", csv)
	}
}
