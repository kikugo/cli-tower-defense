package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	eng "tower-defense/engine"
)

type balanceOverride struct {
	Towers               map[string]eng.TowerStat `json:"towers"`
	Enemies              map[string]eng.EnemyStat `json:"enemies"`
	BreachResourceBounty *int                     `json:"breach_resource_bounty"`
	BreachScore          *int                     `json:"breach_score"`
}

type balanceSweepCandidate struct {
	Name    string          `json:"name"`
	Balance balanceOverride `json:"balance"`
}

type balanceSweepConfig struct {
	Seeds          []int64                 `json:"seeds"`
	MaxTicks       int                     `json:"max_ticks"`
	Ruleset        *eng.ArenaRuleset       `json:"ruleset"`
	DefenderScript string                  `json:"defender_script"`
	AttackerScript string                  `json:"attacker_script"`
	Candidates     []balanceSweepCandidate `json:"candidates"`
}

// applyBalanceOverride overlays a candidate's overrides on a copy of base.
// Tower/enemy overrides replace the WHOLE stat for that type (no field merge).
func applyBalanceOverride(base eng.BalanceConfig, o balanceOverride) eng.BalanceConfig {
	out := base
	out.Towers = make(map[string]eng.TowerStat, len(base.Towers))
	for k, v := range base.Towers {
		out.Towers[k] = v
	}
	out.Enemies = make(map[string]eng.EnemyStat, len(base.Enemies))
	for k, v := range base.Enemies {
		out.Enemies[k] = v
	}
	for k, v := range o.Towers {
		out.Towers[k] = v
	}
	for k, v := range o.Enemies {
		out.Enemies[k] = v
	}
	if o.BreachResourceBounty != nil {
		out.BreachResourceBounty = *o.BreachResourceBounty
	}
	if o.BreachScore != nil {
		out.BreachScore = *o.BreachScore
	}
	return out
}

// minStratumSample is the smallest per-stratum match count this report will
// render as a percentage. Below this, a "win rate" is noise pretending to be
// a finding -- the whole reason this file exists is a false conclusion drawn
// from a 16-match stratum. See formatRate.
const minStratumSample = 10

// unbalancedShareThreshold is the point at which a single stratum so
// dominates a candidate's matches that an aggregate over all strata would
// just be restating that stratum's number under a misleading label. See
// dominantStratumShare and runBalanceSweep.
const unbalancedShareThreshold = 0.90

// stratumStats accumulates duel outcomes for one (candidate, stratum)
// bucket, where "stratum" is the realised lanes count a match generated
// (MatchResult.Strata["lanes"]), not anything a ruleset requested.
//
// rejectedDefender/rejectedAttacker/providerCalls/authored* are the
// recorded-measurement columns: what the arena actually recorded during each
// duel, as opposed to the four simulation numbers above (n, wins, ticks,
// score). A change to the engine's decision plumbing can leave every
// simulation number bit-identical while silently changing these -- see
// runBalanceSweep and skip_forced_save_turns in readme.md.
type stratumStats struct {
	key        string
	n          int
	wins       int
	totalTicks int64
	totalScore int

	rejectedDefender int
	rejectedAttacker int
	providerCalls    int

	// authoredSum/authoredMeasured/authoredTotal implement an unweighted mean
	// of eng.MatchResult.ModelAuthored's per-player, per-match share. Each
	// match contributes up to two samples (defender, attacker); a sample
	// counts toward authoredSum/authoredMeasured only when ModelAuthored's
	// bool says that player's provenance was actually recorded for that
	// match -- an unmeasured sample is dropped from the average entirely,
	// never treated as 0. authoredTotal is the number of samples attempted
	// (2 * match count), so the printed aggregate can show partial coverage
	// ("measured X/Y") instead of silently implying full coverage. See
	// formatAuthoredAggregate.
	authoredSum      float64
	authoredMeasured int
	authoredTotal    int
}

func (s stratumStats) avgTicks() float64  { return float64(s.totalTicks) / float64(s.n) }
func (s stratumStats) avgScore() float64  { return float64(s.totalScore) / float64(s.n) }
func (s stratumStats) rejectedTotal() int { return s.rejectedDefender + s.rejectedAttacker }

// stratumKeyFor derives the grouping key for one match result from its
// realised lane count. A result with no Strata map, or no "lanes" entry,
// groups under "lanes=?" rather than crashing or being silently dropped --
// requirement is that every duel ends up counted in exactly one bucket.
func stratumKeyFor(result eng.MatchResult) string {
	if result.Strata == nil {
		return "lanes=?"
	}
	lanes, ok := result.Strata["lanes"]
	if !ok || lanes == "" {
		return "lanes=?"
	}
	return "lanes=" + lanes
}

// laneNumber extracts the numeric lane count from a "lanes=N" stratum key,
// used only to sort strata numerically (so "lanes=10" would sort after
// "lanes=2", unlike a plain lexicographic sort).
func laneNumber(key string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimPrefix(key, "lanes="))
	if err != nil {
		return 0, false
	}
	return n, true
}

// sumByPlayerPrefix sums a counters map keyed either exactly playerID
// (eng.MatchResult.ProviderCalls' convention) or "playerID:suffix"
// (eng.MatchResult.RejectedActions' and DecisionSources' convention). This
// mirrors sumCounters (engine/report_markdown.go) and totalByPlayerPrefix
// (engine/scoring.go); both are unexported in package engine, so this is a
// small local re-implementation rather than a shared import.
func sumByPlayerPrefix(counters map[string]int, playerID string) int {
	if playerID == "" {
		return 0
	}
	total := 0
	prefix := playerID + ":"
	for key, val := range counters {
		if key == playerID || strings.HasPrefix(key, prefix) {
			total += val
		}
	}
	return total
}

// groupByStratum buckets a candidate's duel results by realised lane count
// and returns the buckets alongside their keys in stable, deterministic
// order: ascending lane count, with the unparseable "lanes=?" bucket last.
// The stable order is what makes sweep output diffable across runs.
func groupByStratum(results []eng.MatchResult) (map[string]*stratumStats, []string) {
	buckets := make(map[string]*stratumStats)
	for _, r := range results {
		key := stratumKeyFor(r)
		b, ok := buckets[key]
		if !ok {
			b = &stratumStats{key: key}
			buckets[key] = b
		}
		b.n++
		if r.DefenderHeld() {
			b.wins++
		}
		b.totalTicks += r.Ticks
		b.totalScore += r.Score[r.Defender]

		b.rejectedDefender += sumByPlayerPrefix(r.RejectedActions, r.Defender)
		b.rejectedAttacker += sumByPlayerPrefix(r.RejectedActions, r.Attacker)
		b.providerCalls += r.ProviderCalls[r.Defender] + r.ProviderCalls[r.Attacker]
		for _, p := range []string{r.Defender, r.Attacker} {
			if p == "" {
				// A result with no attacker/defender recorded (malformed or
				// hand-built) contributes no attempted sample -- counting an
				// empty player would inflate authoredTotal without ever being
				// able to measure it, understating coverage for no reason.
				continue
			}
			b.authoredTotal++
			if share, ok := r.ModelAuthored(p); ok {
				b.authoredSum += share
				b.authoredMeasured++
			}
		}
	}
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ni, oki := laneNumber(keys[i])
		nj, okj := laneNumber(keys[j])
		if oki && okj {
			return ni < nj
		}
		if oki != okj {
			return oki // parseable "lanes=N" keys sort before "lanes=?"
		}
		return keys[i] < keys[j]
	})
	return buckets, keys
}

// aggregateStrata sums every bucket into a single MIXTURE-labelled total.
// Callers must not print this without also calling composeMixtureNote (or
// deciding, via dominantStratumShare, not to print a mixture row at all) --
// see formatMixtureRow, whose signature makes that pairing structural.
func aggregateStrata(buckets map[string]*stratumStats, keys []string) stratumStats {
	agg := stratumStats{key: "MIXTURE"}
	for _, k := range keys {
		b := buckets[k]
		agg.n += b.n
		agg.wins += b.wins
		agg.totalTicks += b.totalTicks
		agg.totalScore += b.totalScore
		agg.rejectedDefender += b.rejectedDefender
		agg.rejectedAttacker += b.rejectedAttacker
		agg.providerCalls += b.providerCalls
		agg.authoredSum += b.authoredSum
		agg.authoredMeasured += b.authoredMeasured
		agg.authoredTotal += b.authoredTotal
	}
	return agg
}

// dominantStratumShare reports the largest single stratum's share of a
// candidate's total matches, and which stratum that was. When that share
// exceeds unbalancedShareThreshold, an aggregate across strata would not be
// a meaningful number -- see runBalanceSweep.
func dominantStratumShare(buckets map[string]*stratumStats, keys []string, total int) (dominantKey string, share float64) {
	for _, k := range keys {
		s := float64(buckets[k].n) / float64(total)
		if s > share {
			share = s
			dominantKey = k
		}
	}
	return dominantKey, share
}

// composeMixtureNote renders the "[lanes=1:60% lanes=2:40%]" composition
// string a MIXTURE row must carry. It is the only supported way to build
// that string, and formatMixtureRow requires one of these as an argument.
func composeMixtureNote(buckets map[string]*stratumStats, keys []string, total int) string {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		pct := float64(buckets[k].n) / float64(total) * 100
		parts = append(parts, fmt.Sprintf("%s:%.0f%%", k, pct))
	}
	return strings.Join(parts, " ")
}

// formatRate renders a stratum's defender win rate, refusing to print a
// percentage for an underpowered sample (n < minStratumSample). Small-n
// strata are exactly where the false "range is a smooth tuning axis"
// finding came from.
func formatRate(n, wins int) string {
	if n < minStratumSample {
		return "(underpowered)"
	}
	return fmt.Sprintf("%.0f%%", float64(wins)/float64(n)*100)
}

// formatAuthoredAggregate renders the model-authored share aggregated across
// every (player, match) sample in a stratum or mixture -- an unweighted mean
// of eng.MatchResult.ModelAuthored's per-sample share, over samples where
// ModelAuthored's bool said provenance was actually recorded (see
// stratumStats.authoredSum). An unmeasured sample is dropped from the
// average entirely; it is never treated as 0, which is the same "0% authored"
// vs "not measured" distinction formatModelAuthoredShare (main.go) and
// formatModelAuthored (engine/report_markdown.go) already enforce per-match.
// When some but not all samples in the aggregate are measured, the coverage
// is printed alongside the percentage ("62% (58/80 measured)") rather than
// silently presenting a partial average as if it covered everything.
func formatAuthoredAggregate(sum float64, measured, total int) string {
	if measured == 0 {
		return "not measured"
	}
	pct := fmt.Sprintf("%.0f%%", sum/float64(measured)*100)
	if measured < total {
		return fmt.Sprintf("%s (%d/%d measured)", pct, measured, total)
	}
	return pct
}

// formatRecordedLine renders the recorded-measurement second line that
// accompanies every stratum/MIXTURE row: total rejected actions (split
// defender/attacker), total provider calls, and the model-authored share --
// all read off what the arena actually recorded, not the simulation. This is
// the column set that can change while the four simulation numbers on the
// first line stay bit-identical; see skip_forced_save_turns in readme.md.
func formatRecordedLine(s stratumStats) string {
	return fmt.Sprintf("    recorded: rejected def=%d att=%d total=%d | provider calls=%d | model-authored=%s",
		s.rejectedDefender, s.rejectedAttacker, s.rejectedTotal(), s.providerCalls,
		formatAuthoredAggregate(s.authoredSum, s.authoredMeasured, s.authoredTotal))
}

// formatStratumRow renders one (candidate, stratum) row across two lines:
// the simulation numbers (n, wins, rate, avg ticks, avg score), then the
// recorded-measurement numbers from formatRecordedLine.
func formatStratumRow(candidate string, s stratumStats) string {
	first := fmt.Sprintf("%-24s | %-9s | %3d | %2d/%-5d | %-13s | %9.1f | %.1f",
		candidate, s.key, s.n, s.wins, s.n, formatRate(s.n, s.wins), s.avgTicks(), s.avgScore())
	return first + "\n" + formatRecordedLine(s)
}

// formatMixtureRow renders the aggregate MIXTURE row, across two lines like
// formatStratumRow. composition is a required parameter -- not optional, not
// defaulted -- specifically so a future edit cannot print a MIXTURE row
// without stating what it is a mixture of. An empty composition is refused
// outright rather than silently printing a bare "MIXTURE" row.
func formatMixtureRow(candidate string, agg stratumStats, composition string) string {
	if composition == "" {
		panic("formatMixtureRow: composition must not be empty -- a MIXTURE row must always state its composition")
	}
	first := fmt.Sprintf("%-24s | %-9s | %3d | %2d/%-5d | %-13s | %9.1f | %.1f   [%s]",
		candidate, "MIXTURE", agg.n, agg.wins, agg.n, formatRate(agg.n, agg.wins), agg.avgTicks(), agg.avgScore(), composition)
	return first + "\n" + formatRecordedLine(agg)
}

// formatUnbalancedNote replaces the MIXTURE row when one stratum accounts
// for more than unbalancedShareThreshold of a candidate's matches. An
// aggregate over an unbalanced design is not a meaningful number, so no row
// is printed in its place -- only an explanation.
func formatUnbalancedNote(candidate, dominantKey string, sharePct float64) string {
	return fmt.Sprintf("%-24s | %-9s | aggregate suppressed: %s is %.0f%% of matches -- an aggregate over an unbalanced design is not a meaningful number",
		candidate, "MIXTURE", dominantKey, sharePct)
}

func runBalanceSweep(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg balanceSweepConfig
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return fmt.Errorf("parse balance sweep config %s: %w", path, err)
	}
	if len(cfg.Seeds) == 0 {
		cfg.Seeds = []int64{1, 2, 3, 4, 5, 6, 7, 8}
	}
	if cfg.MaxTicks <= 0 {
		cfg.MaxTicks = 400
	}
	if cfg.DefenderScript == "" {
		cfg.DefenderScript = "defender_baseline"
	}
	if cfg.AttackerScript == "" {
		// The default scripted attacker launches waves when resources allow,
		// giving the defender a real wave-clear victory path.
		cfg.AttackerScript = "attacker_baseline"
	}
	ruleset := eng.BaselineDuelRuleset()
	if cfg.Ruleset != nil {
		ruleset = *cfg.Ruleset
	}

	fmt.Printf("%-24s | %-9s | %-3s | %-8s | %-13s | %9s | %s\n",
		"candidate", "stratum", "n", "def wins", "rate", "avg ticks", "avg def score")
	fmt.Println("  (each row's second line: recorded rejections / provider calls / model-authored share -- what the arena recorded, not the simulation)")
	for _, cand := range cfg.Candidates {
		balance := applyBalanceOverride(eng.DefaultBalanceConfig(), cand.Balance)
		results := make([]eng.MatchResult, 0, len(cfg.Seeds))
		for _, seed := range cfg.Seeds {
			result := eng.RunScriptedDuel(eng.ScriptedDuelConfig{
				Seed: seed, MaxTicks: cfg.MaxTicks, Ruleset: ruleset, Balance: balance,
				DefenderScript: cfg.DefenderScript, AttackerScript: cfg.AttackerScript,
			})
			results = append(results, result)
		}

		buckets, keys := groupByStratum(results)
		for _, k := range keys {
			fmt.Println(formatStratumRow(cand.Name, *buckets[k]))
		}

		total := len(results)
		dominantKey, share := dominantStratumShare(buckets, keys, total)
		if share > unbalancedShareThreshold {
			fmt.Println(formatUnbalancedNote(cand.Name, dominantKey, share*100))
			continue
		}
		agg := aggregateStrata(buckets, keys)
		composition := composeMixtureNote(buckets, keys, total)
		fmt.Println(formatMixtureRow(cand.Name, agg, composition))
	}
	return nil
}
