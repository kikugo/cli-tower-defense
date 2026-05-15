package engine

import "testing"

func TestBuildTournamentStandings(t *testing.T) {
	results := []TournamentMatchResult{
		{
			Result: MatchResult{
				Winner: "p1",
				Models: map[string]string{"p1": "model-a", "p2": "model-b"},
				Score:  map[string]int{"p1": 100, "p2": 50},
				Waves:  3,
			},
		},
		{
			Result: MatchResult{
				Winner: "p2",
				Models: map[string]string{"p1": "model-a", "p2": "model-b"},
				Score:  map[string]int{"p1": 20, "p2": 80},
				Waves:  5,
			},
		},
	}

	standings := BuildTournamentStandings(results)
	if len(standings) != 2 {
		t.Fatalf("expected two standings, got %d", len(standings))
	}
	for _, standing := range standings {
		if standing.Matches != 2 {
			t.Fatalf("expected two matches per model, got %d", standing.Matches)
		}
		if standing.Wins != 1 {
			t.Fatalf("expected one win per model, got %d", standing.Wins)
		}
	}
}
