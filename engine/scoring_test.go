package engine

import "testing"

func TestBuildScoreBreakdownRewardsWinningPlayer(t *testing.T) {
	result := MatchResult{
		Winner:          "p1",
		Waves:           10,
		MaxWaves:        20,
		Score:           map[string]int{"p1": 200, "p2": 200},
		Lives:           map[string]int{"p1": 15, "p2": 3},
		RejectedActions: map[string]int{"p1:save": 1, "p2:save": 6},
		ProviderErrors:  map[string]int{"p1:timeout": 0, "p2:timeout": 2},
	}
	p1 := BuildScoreBreakdown(result, "p1")
	p2 := BuildScoreBreakdown(result, "p2")
	if p1.Normalized <= p2.Normalized {
		t.Fatalf("expected winner score to be higher, p1=%f p2=%f", p1.Normalized, p2.Normalized)
	}
}
