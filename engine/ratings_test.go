package engine

import "testing"

func TestApplyTournamentResultsUpdatesRatings(t *testing.T) {
	r := DefaultModelRatings()
	results := []TournamentMatchResult{
		{
			Result: MatchResult{
				Winner: "p1",
				Models: map[string]string{"p1": "model-a", "p2": "model-b"},
			},
		},
	}
	r.ApplyTournamentResults(results)
	if r.Ratings["model-a"] <= 1200 {
		t.Fatalf("expected winner rating to increase")
	}
	if r.Ratings["model-b"] >= 1200 {
		t.Fatalf("expected loser rating to decrease")
	}
}
