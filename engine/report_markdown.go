package engine

import (
	"fmt"
	"strings"
	"time"
)

// MarkdownReport renders a human-readable markdown summary of a match result.
// It is used by headless runs (-report-md) to produce shareable match reports.
func (r MatchResult) MarkdownReport() string {
	var b strings.Builder

	title := "Match Report"
	if r.WinnerModel != "" {
		title = fmt.Sprintf("Match Report: %s wins", r.WinnerModel)
	}
	fmt.Fprintf(&b, "# %s\n\n", title)

	fmt.Fprintf(&b, "- **Winner:** %s\n", roleLabel(r, r.Winner))
	fmt.Fprintf(&b, "- **Win reason:** %s\n", humanizeReason(r.WinReason))
	fmt.Fprintf(&b, "- **Waves:** %d / %d\n", r.Waves, r.MaxWaves)
	fmt.Fprintf(&b, "- **Ticks:** %d\n", r.Ticks)
	fmt.Fprintf(&b, "- **Duration:** %s\n", formatMillis(r.DurationMillis))
	fmt.Fprintf(&b, "- **Replay events:** %d\n\n", r.ReplayEvents)

	fmt.Fprintf(&b, "## Players\n\n")
	fmt.Fprintf(&b, "| Player | Role | Model | Lives | Score | Norm | Authored | Calls | Avg latency | Tokens | Est. cost | Rejected | Errors |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, p := range orderedPlayers(r) {
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %.3f | %s | %d | %s | %d | %s | %d | %d |\n",
			p,
			roleForPlayer(r, p),
			r.Models[p],
			r.Lives[p],
			r.Score[p],
			r.NormalizedScore[p],
			formatModelAuthored(r, p),
			r.ProviderCalls[p],
			formatLatency(r.ProviderLatency[p]),
			r.TokenUsage[p],
			formatCost(r.CostMicros[p]),
			sumCounters(r.RejectedActions, p),
			sumCounters(r.ProviderErrors, p),
		)
	}
	b.WriteString("\n")

	return b.String()
}

// formatModelAuthored renders MatchResult.ModelAuthored for the report
// table. "not measured" -- never "0%" -- is what a pre-provenance result (or
// a player with no recorded decisions at all) renders as; see ModelAuthored.
func formatModelAuthored(r MatchResult, playerID string) string {
	share, ok := r.ModelAuthored(playerID)
	if !ok {
		return "not measured"
	}
	return fmt.Sprintf("%.0f%%", share*100)
}

// orderedPlayers returns player IDs in a stable order: defender then attacker,
// falling back to any other players present.
func orderedPlayers(r MatchResult) []string {
	seen := map[string]bool{}
	order := []string{}
	for _, p := range []string{r.Defender, r.Attacker} {
		if p != "" && !seen[p] {
			seen[p] = true
			order = append(order, p)
		}
	}
	for p := range r.Models {
		if !seen[p] {
			seen[p] = true
			order = append(order, p)
		}
	}
	return order
}

func roleForPlayer(r MatchResult, playerID string) string {
	switch playerID {
	case r.Defender:
		return "defender"
	case r.Attacker:
		return "attacker"
	default:
		return "-"
	}
}

func roleLabel(r MatchResult, playerID string) string {
	if playerID == "" {
		return "none"
	}
	model := r.Models[playerID]
	role := roleForPlayer(r, playerID)
	if model == "" {
		return fmt.Sprintf("%s (%s)", playerID, role)
	}
	return fmt.Sprintf("%s (%s)", model, role)
}

func humanizeReason(reason string) string {
	if reason == "" {
		return "unknown"
	}
	return strings.ReplaceAll(reason, "_", " ")
}

// sumCounters totals any counters keyed by "<playerID>:<label>" plus a bare
// "<playerID>" entry, so per-action breakdowns collapse into a per-player total.
func sumCounters(counters map[string]int, playerID string) int {
	total := 0
	prefix := playerID + ":"
	for key, val := range counters {
		if key == playerID || strings.HasPrefix(key, prefix) {
			total += val
		}
	}
	return total
}

func formatMillis(ms int64) string {
	if ms <= 0 {
		return "0s"
	}
	return (time.Duration(ms) * time.Millisecond).Round(time.Millisecond).String()
}

func formatLatency(avgMS float64) string {
	if avgMS <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f ms", avgMS)
}

func formatCost(micros int64) string {
	if micros <= 0 {
		return "-"
	}
	return fmt.Sprintf("$%.4f", float64(micros)/1_000_000)
}
