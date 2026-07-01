package engine

import (
	"fmt"
	"strings"
)

// MarkdownReport renders a tournament report as markdown: ranked standings
// followed by a per-match results table. It is used by headless tournament
// runs (-tournament-md) to produce a shareable comparison document.
func (r TournamentReport) MarkdownReport() string {
	var b strings.Builder

	name := r.Name
	if name == "" {
		name = "Tournament"
	}
	fmt.Fprintf(&b, "# Tournament: %s\n\n", name)
	fmt.Fprintf(&b, "- **Matches played:** %d\n", len(r.Results))
	fmt.Fprintf(&b, "- **Models:** %d\n\n", len(r.Standings))

	fmt.Fprintf(&b, "## Standings\n\n")
	fmt.Fprintf(&b, "| Rank | Model | Matches | Wins | Win rate | Avg score | Avg norm | Avg wave | Rejected | Errors | Rating |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|---|---|---|\n")
	for i, s := range SortStandings(r.Standings) {
		fmt.Fprintf(&b, "| %d | %s | %d | %d | %.3f | %.1f | %.3f | %.2f | %d | %d | %s |\n",
			i+1,
			s.Model,
			s.Matches,
			s.Wins,
			s.WinRate,
			s.AverageScore,
			s.AverageNormalized,
			s.AverageWaveReached,
			s.RejectedActions,
			s.ProviderErrors,
			formatRating(r.Ratings, s.Model),
		)
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## Matches\n\n")
	fmt.Fprintf(&b, "| Matchup | Seed | Swapped | Winner | Waves | Win reason |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|\n")
	for _, m := range r.Results {
		winner := m.Result.WinnerModel
		if winner == "" {
			winner = m.Result.Winner
		}
		fmt.Fprintf(&b, "| %s | %d | %t | %s | %d/%d | %s |\n",
			m.Matchup,
			m.Seed,
			m.Swapped,
			winner,
			m.Result.Waves,
			m.Result.MaxWaves,
			humanizeReason(m.Result.WinReason),
		)
	}
	b.WriteString("\n")

	return b.String()
}

func formatRating(ratings map[string]float64, model string) string {
	if ratings == nil {
		return "-"
	}
	if v, ok := ratings[model]; ok {
		return fmt.Sprintf("%.0f", v)
	}
	return "-"
}
