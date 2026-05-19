package engine

import "math"

type ModelRatings struct {
	Ratings map[string]float64 `json:"ratings"`
	KFactor float64            `json:"k_factor"`
}

func DefaultModelRatings() ModelRatings {
	return ModelRatings{
		Ratings: map[string]float64{},
		KFactor: 24.0,
	}
}

func (r *ModelRatings) ensure(model string) {
	if r.Ratings == nil {
		r.Ratings = map[string]float64{}
	}
	if _, ok := r.Ratings[model]; !ok {
		r.Ratings[model] = 1200
	}
	if r.KFactor <= 0 {
		r.KFactor = 24.0
	}
}

func (r *ModelRatings) ApplyTournamentResults(results []TournamentMatchResult) {
	for _, match := range results {
		p1 := match.Result.Models[match.Result.Player1()]
		p2 := match.Result.Models[match.Result.Player2()]
		if p1 == "" || p2 == "" {
			continue
		}
		r.ensure(p1)
		r.ensure(p2)
		s1, s2 := scoresForMatch(match.Result)
		r1, r2 := r.Ratings[p1], r.Ratings[p2]
		e1 := 1.0 / (1.0 + math.Pow(10, (r2-r1)/400.0))
		e2 := 1.0 / (1.0 + math.Pow(10, (r1-r2)/400.0))
		r.Ratings[p1] = r1 + r.KFactor*(s1-e1)
		r.Ratings[p2] = r2 + r.KFactor*(s2-e2)
	}
}

func scoresForMatch(result MatchResult) (float64, float64) {
	p1 := result.Player1()
	p2 := result.Player2()
	switch result.Winner {
	case p1:
		return 1.0, 0.0
	case p2:
		return 0.0, 1.0
	default:
		return 0.5, 0.5
	}
}
