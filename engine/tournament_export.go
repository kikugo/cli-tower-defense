package engine

import (
	"encoding/csv"
	"sort"
	"strconv"
	"strings"
)

// SortStandings orders standings for reporting: highest win rate first, then
// highest average normalized score, then model name for stable ties.
func SortStandings(standings []TournamentStanding) []TournamentStanding {
	sorted := make([]TournamentStanding, len(standings))
	copy(sorted, standings)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.WinRate != b.WinRate {
			return a.WinRate > b.WinRate
		}
		if a.AverageNormalized != b.AverageNormalized {
			return a.AverageNormalized > b.AverageNormalized
		}
		if a.AverageScore != b.AverageScore {
			return a.AverageScore > b.AverageScore
		}
		return a.Model < b.Model
	})
	return sorted
}

// StandingsCSV renders tournament standings as CSV, ranked best-first.
func StandingsCSV(standings []TournamentStanding) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write([]string{
		"rank", "model", "matches", "wins", "win_rate",
		"average_score", "average_normalized_score", "average_wave_reached",
		"rejected_actions", "provider_errors",
	})
	for i, s := range SortStandings(standings) {
		_ = w.Write([]string{
			strconv.Itoa(i + 1),
			s.Model,
			strconv.Itoa(s.Matches),
			strconv.Itoa(s.Wins),
			strconv.FormatFloat(s.WinRate, 'f', 4, 64),
			strconv.FormatFloat(s.AverageScore, 'f', 2, 64),
			strconv.FormatFloat(s.AverageNormalized, 'f', 4, 64),
			strconv.FormatFloat(s.AverageWaveReached, 'f', 2, 64),
			strconv.Itoa(s.RejectedActions),
			strconv.Itoa(s.ProviderErrors),
		})
	}
	w.Flush()
	return b.String()
}
