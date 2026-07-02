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

func TestRejectionPenaltyIsCapped(t *testing.T) {
	// A shutout defender (all lives, decent score, won) with a huge rejection
	// count must not be crushed to zero: penalties cap so outcomes dominate.
	result := MatchResult{
		Winner:          "p1",
		Waves:           0,
		MaxWaves:        5,
		Score:           map[string]int{"p1": 480, "p2": 0},
		Lives:           map[string]int{"p1": 20, "p2": 20},
		RejectedActions: map[string]int{"p1:place": 185},
	}
	got := BuildScoreBreakdown(result, "p1")
	if got.RejectPenalty > maxRejectPenalty {
		t.Fatalf("expected reject penalty capped at %v, got %v", maxRejectPenalty, got.RejectPenalty)
	}
	if got.Normalized <= 0.3 {
		t.Fatalf("shutout winner should keep a meaningful score, got %v", got.Normalized)
	}
}

func TestNormalizedScoreStaysInUnitRange(t *testing.T) {
	// A dominant player with a huge raw score must not exceed 1.0.
	dominant := MatchResult{
		Winner:   "p1",
		Waves:    30,
		MaxWaves: 30,
		Score:    map[string]int{"p1": 100000, "p2": 0},
		Lives:    map[string]int{"p1": 100, "p2": 0},
	}
	// A player drowning in penalties must not drop below 0.0.
	punished := MatchResult{
		Winner:          "p1",
		Waves:           0,
		MaxWaves:        30,
		Score:           map[string]int{"p1": 0, "p2": 0},
		Lives:           map[string]int{"p2": 0},
		RejectedActions: map[string]int{"p2:spawn": 500},
		ProviderErrors:  map[string]int{"p2:timeout": 500},
	}

	for _, tc := range []struct {
		name   string
		result MatchResult
		player string
	}{
		{"dominant", dominant, "p1"},
		{"punished", punished, "p2"},
	} {
		got := BuildScoreBreakdown(tc.result, tc.player).Normalized
		if got < 0 || got > 1 {
			t.Fatalf("%s: normalized score %f out of [0,1]", tc.name, got)
		}
	}
}
