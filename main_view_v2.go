package main

// Phase 4 of the redesign: the composition layer. This is where
// computeLayoutV2's pane rects meet the v2 renderers, and where live engine
// state is turned into the flat, pre-decided structs those renderers take.
//
// --- Composition is a blit, not a stack ------------------------------------
//
// The old view composed panes with vstack/hjoin -- concatenate row lists,
// then merge two column blocks side by side. That works for the old design,
// where every mode is either a vertical stack or one two-column split.
//
// It does not work for the new wide mode, which has two columns that are
// each themselves a stack of differently-sized panes, with a one-column rule
// between them. Expressing that as nested hjoin/vstack calls means the
// nesting structure has to mirror the layout's structure, so a layout change
// becomes a composition change in a different file.
//
// So instead: computeLayoutV2 already returns absolute rects, and this file
// blits each pane into a frame buffer at its own (x, y). Composition stops
// knowing anything about which panes are beside which. The tiling guarantee
// that makes this safe -- every cell covered exactly once, nothing off-frame
// -- is already tested exhaustively in main_layout_v2_test.go.
//
// blitV2 cuts the destination row with padCells (left) and dropCellsV2
// (right), the same ANSI-safe pair OverlayCenteredV2 uses, because since
// Phase 3 pane content carries colour.
//
// --- Extraction is a boundary, not a convenience ---------------------------
//
// Every buildXV2 function below exists to make one decision in one place:
// what "not measured" means for each figure. The renderers take three-state
// types (AuthoredShare, SavesStat, TrustState) precisely so they CANNOT see
// a raw counter and print a confident zero for a match that never recorded
// one. This file is where the engine's (value, ok) accessors are turned into
// those states, and it is the only place allowed to know the rule.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	eng "tower-defense/engine"
)

// defaultTickDurV2 is the tick duration a fresh model starts at (main.go's
// initialModel), and therefore the denominator of the header's "1.0x" speed
// figure. It is named here rather than repeated as a literal so the speed
// readout and the model's own starting value cannot drift apart.
const defaultTickDurV2 = 100 * time.Millisecond

// --- frame buffer ---------------------------------------------------------

// newFrameV2 returns h rows of w blank columns.
func newFrameV2(w, h int) []string {
	return blankRows(h, w)
}

// blitV2 writes rows into frame at rc's position. Rows longer or shorter
// than rc.w are padded/truncated to it, and rows past rc.h are dropped, so a
// renderer that broke its exact-size contract can corrupt its own pane but
// never the ones beside it.
func blitV2(frame []string, rc paneRectV2, rows []string, frameW int) {
	if rc.w <= 0 || rc.h <= 0 {
		return
	}
	for i := 0; i < rc.h; i++ {
		y := rc.y + i
		if y < 0 || y >= len(frame) {
			continue
		}
		content := ""
		if i < len(rows) {
			content = rows[i]
		}
		mid := padCells(content, rc.w)

		if rc.x == 0 && rc.w == frameW {
			frame[y] = mid
			continue
		}
		left := padCells(frame[y], rc.x)
		right := dropCellsV2(frame[y], rc.x+rc.w)
		frame[y] = padCells(left+mid+right, frameW)
	}
}

// verticalRuleV2 renders the one-column divider between wide mode's two
// columns.
func verticalRuleV2(h int) []string {
	rows := make([]string, h)
	for i := range rows {
		rows[i] = styleDimV2.Render("│")
	}
	return rows
}

// --- extraction: shared pieces --------------------------------------------

// authoredShareV2 converts engine.MatchResult's three-state authorship
// accessor into the renderers' AuthoredShare. This function is the entire
// reason ModelAuthoredState exists: without it the only way to tell
// "provenance was never recorded" from "no decisions yet" outside the engine
// would be to read ProvenanceVersion here, duplicating a rule match_result.go
// owns.
func authoredShareV2(r eng.MatchResult, playerID string) AuthoredShare {
	share, state := r.ModelAuthoredState(playerID)
	switch state {
	case eng.AuthorshipUntracked:
		return AuthoredShare{}
	case eng.AuthorshipNoDecisions:
		return AuthoredShare{Known: true}
	default:
		return AuthoredShare{Known: true, HasData: true, Share: share}
	}
}

func savesStatV2(r eng.MatchResult, playerID string) SavesStat {
	authored, total, ok := r.AuthoredSaves(playerID)
	if !ok {
		return SavesStat{}
	}
	return SavesStat{Known: true, Authored: authored, Total: total}
}

// trustStateV2 builds the trust band's input. AssistsEnabled is the
// NEGATION of the ruleset's DisableAssists, and it is read from the ruleset
// rather than inferred from a zero assist count -- "assists are off" and
// "assists are on and nothing has fired yet" are different claims and the UI
// words them differently.
func trustStateV2(g *eng.Game, r eng.MatchResult, ruleset eng.ArenaRuleset) TrustState {
	defAssists, ok := r.EngineAssistTotal(g.Defender)
	attAssists, _ := r.EngineAssistTotal(g.Attacker)

	_, provState := r.ModelAuthoredState(g.Defender)

	t := TrustState{
		AssistKnown:     ok,
		AssistsEnabled:  !ruleset.DisableAssists,
		AssistCount:     defAssists + attAssists,
		ProvenanceKnown: provState != eng.AuthorshipUntracked,
	}
	if t.AssistKnown && t.AssistsEnabled && t.AssistCount > 0 {
		t.AssistDetail = assistDetailV2(g.ReplayEvents)
	}
	return t
}

// assistDetailV2 summarises WHAT the engine did, not just how often -- the
// distinction the whole assist-telemetry effort exists to surface. Branch
// names come from engine/assist.go's AssistBranch constants.
func assistDetailV2(events []eng.ReplayEvent) string {
	counts := map[eng.AssistBranch]int{}
	for _, ev := range events {
		if ev.Type != eng.ReplayEngineAssist {
			continue
		}
		branch, _ := ev.Details["branch"].(string)
		counts[eng.AssistBranch(branch)]++
	}

	var parts []string
	if n := counts[eng.AssistQueueEnemy]; n > 0 {
		parts = append(parts, fmt.Sprintf("queued %d enemies", n))
	}
	if n := counts[eng.AssistAutoWave]; n > 0 {
		parts = append(parts, fmt.Sprintf("started %d waves", n))
	}
	if n := counts[eng.AssistReinforceWave]; n > 0 {
		parts = append(parts, fmt.Sprintf("fired %d abilities", n))
	}
	return strings.Join(parts, ", ")
}

// builtTallyV2 renders the defender's tower count by type as "^3  !2  *1",
// in the vocabulary's own order so the tally reads the same way every frame
// regardless of map iteration order.
func builtTallyV2(g *eng.Game) string {
	counts := map[string]int{}
	for _, t := range g.Towers {
		counts[t.TowerType]++
	}
	return tallyV2(counts, towerGlyph, []string{"basic", "sniper", "splash", "buffer"})
}

// liveTallyV2 renders the attacker's live enemies by type as "o4  f2  t1".
func liveTallyV2(g *eng.Game) string {
	counts := map[string]int{}
	for _, e := range g.Enemies {
		counts[e.EnemyType]++
	}
	return tallyV2(counts, enemyGlyph, []string{"basic", "fast", "tank", "shielded", "healer"})
}

// tallyV2 formats a type->count map in a FIXED type order, with any type not
// in that order appended alphabetically afterwards. The fixed order is what
// keeps the tally from reordering itself between frames (Go map iteration is
// deliberately randomised); the alphabetical tail is so a type added to the
// engine without being added to the order list still appears, rather than
// silently vanishing from the UI.
func tallyV2(counts map[string]int, glyph func(string) rune, order []string) string {
	seen := map[string]bool{}
	var parts []string
	for _, typ := range order {
		seen[typ] = true
		if n := counts[typ]; n > 0 {
			parts = append(parts, fmt.Sprintf("%c%d", glyph(typ), n))
		}
	}
	var extra []string
	for typ := range counts {
		if !seen[typ] {
			extra = append(extra, typ)
		}
	}
	sort.Strings(extra)
	for _, typ := range extra {
		parts = append(parts, fmt.Sprintf("%c%d", glyph(typ), counts[typ]))
	}
	return strings.Join(parts, "  ")
}

// researchTallyV2 renders the defender's research as "economy 2, range 1",
// omitting untaken lines entirely rather than printing them at 0.
func researchTallyV2(g *eng.Game) string {
	var parts []string
	for _, tech := range []string{"economy", "range", "control"} {
		if lvl := g.ResearchLevels[tech]; lvl > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", tech, lvl))
		}
	}
	return strings.Join(parts, ", ")
}

// abilityTallyV2 renders the attacker's APPLIED ability uses as
// "surge 1, shield_burst 1".
//
// It counts applied uses, not attempts: g.ActionCounters bumps on every
// "ability" decision including rejected ones (an ability still on cooldown),
// and it does not record WHICH ability, since applyDecision leaves
// entityType empty on that branch. So this pairs each ReplayOutcome whose
// action is "ability" and whose recorded outcome is a clean apply with the
// ReplayDecision at the same tick, which is where the ability's name lives.
func abilityTallyV2(events []eng.ReplayEvent) string {
	type key struct {
		tick   int64
		player string
	}
	names := map[key]string{}
	for _, ev := range events {
		if ev.Type != eng.ReplayDecision || ev.Action != "ability" {
			continue
		}
		decision, _ := ev.Details["decision"].(map[string]interface{})
		name, _ := decision["ability"].(string)
		if name != "" {
			names[key{ev.Tick, ev.PlayerID}] = name
		}
	}

	counts := map[string]int{}
	for _, ev := range events {
		if ev.Type != eng.ReplayOutcome || ev.Action != "ability" {
			continue
		}
		if ev.Reason != "applied_primary" {
			continue
		}
		if name, ok := names[key{ev.Tick, ev.PlayerID}]; ok {
			counts[name]++
		}
	}

	var parts []string
	for _, name := range []string{"surge", "shield_burst", "reinforce_wave"} {
		if n := counts[name]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", name, n))
		}
	}
	return strings.Join(parts, ", ")
}

// sentCountV2 is how many enemies this match has put on the board, summed
// from the engine's own per-wave WaveSummary.Sent counters.
//
// Two earlier sources were wrong, each in its own way, and both produced the
// same visible nonsense -- a header reading "sent 0  breached 10":
//
//   - ActionCounters[attacker+":spawn"] only counts spawn ACTIONS the
//     attacker chose. An attacker that plays by launching waves never
//     touches it.
//   - Counting ReplaySpawn events misses the same enemies for the same
//     reason: a wave launch queues its enemies without emitting one per
//     enemy.
//
// WaveSummary.Sent is incremented at the single site where an enemy is
// actually added to the board, so it counts every enemy however it got
// there. It is also the number the timeline's own "sent" column shows, so
// the header and the table can no longer disagree.
func sentCountV2(r eng.MatchResult) int {
	n := 0
	for _, ws := range r.WaveSummaries {
		n += ws.Sent
	}
	return n
}

// rulesetStampV2 is the timeline's one-line provenance footer. "pricing
// unset" is printed verbatim when no token pricing was configured -- this
// codebase does not fabricate a cost.
func rulesetStampV2(g *eng.Game, ruleset eng.ArenaRuleset) string {
	assists := "assists on"
	if ruleset.DisableAssists {
		assists = "assists off"
	}
	return fmt.Sprintf("balance %s %s   %d waves   %s   pricing unset",
		g.Balance.Version, eng.ComputeBalanceHash(g.Balance)[:6], g.MaxWaves, assists)
}

// tickHorizon is the tick at which THIS run will stop, or 0 for none.
//
// It is deliberately not m.maxTicks. That flag is documented as "maximum
// ticks to run in headless mode" and is only read by runHeadless and the
// tournament/sweep paths; the interactive Update loop calls UpdateGameState
// on every tickMsg with no upper bound at all. Rendering m.maxTicks as the
// horizon is what produced "tick 445/400" with a full progress bar during a
// live recording -- a finished-looking match that was still running.
//
// A ruleset's own max_ticks is not used either, and for the same reason:
// declared is not enforced. Interactively the match ends when the core
// falls, when the waves run out, or when the viewer quits, so the header
// and the timeline say there is no tick cap, which is true.
func (m model) tickHorizon() int64 {
	return 0
}

// --- extraction: per-pane -------------------------------------------------

func (m model) headerDataV2(r eng.MatchResult) MatchHeaderData {
	g := m.game
	leaked, window, _ := r.RecentLeaks()

	turn := "DEF"
	if g.CurrentTurn == g.Attacker {
		turn = "ATT"
	}
	run := "RUN"
	if m.paused {
		run = "PAUSED"
	}

	return MatchHeaderData{
		Defender: PlayerHeaderData{
			ModelName: g.ModelNames[g.Defender],
			Resources: g.Resources[g.Defender],
			Income:    g.Income[g.Defender],
			Lives:     g.Lives[g.Defender],
			MaxLives:  g.StartingLives,
			Built:     builtTallyV2(g),
			Saves:     savesStatV2(r, g.Defender),
			Authored:  authoredShareV2(r, g.Defender),
		},
		Attacker: PlayerHeaderData{
			ModelName: g.ModelNames[g.Attacker],
			Resources: g.Resources[g.Attacker],
			Income:    g.Income[g.Attacker],
			Live:      liveTallyV2(g),
			Sent:      sentCountV2(r),
			Saves:     savesStatV2(r, g.Attacker),
			Authored:  authoredShareV2(r, g.Attacker),
		},
		Wave: g.Wave, MaxWave: g.MaxWaves,
		Tick: int64(g.TickCount), MaxTick: m.tickHorizon(),
		TurnSide: turn,
		Speed:    speedMultiplierV2(m.tickDur),
		RunState: run,
		Breached: g.BreachCount,
		Leak:     LeakStat{Leaked: leaked, Window: window},
	}
}

// speedMultiplierV2 turns the model's tick duration back into the "1.0x"
// figure the header shows, relative to the default tick rate.
func speedMultiplierV2(tickDur time.Duration) float64 {
	if tickDur <= 0 {
		return 1
	}
	return float64(defaultTickDurV2) / float64(tickDur)
}

func (m model) cardsDataV2(r eng.MatchResult) MatchCardsData {
	g := m.game
	defAssist, assistKnown := r.EngineAssistTotal(g.Defender)
	attAssist, _ := r.EngineAssistTotal(g.Attacker)

	return MatchCardsData{
		Defender: CardsPlayerData{
			ModelName: g.ModelNames[g.Defender],
			Lives:     g.Lives[g.Defender], MaxLives: g.StartingLives,
			Resources: g.Resources[g.Defender], Income: g.Income[g.Defender],
			Built:       builtTallyV2(g),
			Research:    researchTallyV2(g),
			Authored:    authoredShareV2(r, g.Defender),
			Saves:       savesStatV2(r, g.Defender),
			Calls:       g.ProviderCalls[g.Defender],
			Tokens:      g.ProviderTokenUsage[g.Defender],
			AssistKnown: assistKnown, AssistCount: defAssist,
			Streak: g.NoopStreak[g.Defender], StreakMax: g.AutoDefendMinStreak,
			Reasoning: g.LastReasoning[g.Defender],
		},
		Attacker: CardsPlayerData{
			ModelName: g.ModelNames[g.Attacker],
			Breaches:  g.BreachCount,
			Resources: g.Resources[g.Attacker], Income: g.Income[g.Attacker],
			Sent: sentCountV2(r), Live: len(g.Enemies),
			Abilities:   abilityTallyV2(g.ReplayEvents),
			Authored:    authoredShareV2(r, g.Attacker),
			Saves:       savesStatV2(r, g.Attacker),
			Calls:       g.ProviderCalls[g.Attacker],
			Tokens:      g.ProviderTokenUsage[g.Attacker],
			AssistKnown: assistKnown, AssistCount: attAssist,
			Streak: g.NoopStreak[g.Attacker], StreakMax: g.AutoDefendMinStreak,
			Reasoning: g.LastReasoning[g.Attacker],
		},
	}
}

func (m model) timelineDataV2(r eng.MatchResult, trust TrustState) TimelineData {
	g := m.game
	substituted := 0
	for key, n := range r.DecisionSources {
		if strings.Contains(key, ":") && !strings.HasSuffix(key, ":"+string(eng.SourceModel)) &&
			!strings.HasSuffix(key, ":"+string(eng.SourceSkippedForcedSave)) {
			substituted += n
		}
	}

	return TimelineData{
		Waves:         r.WaveSummaries,
		Wave:          g.Wave,
		MaxWave:       g.MaxWaves,
		Tick:          int64(g.TickCount),
		MaxTick:       m.tickHorizon(),
		StartingLives: g.StartingLives,
		Lives:         g.Lives[g.Defender],
		DefAuthored:   authoredShareV2(r, g.Defender),
		AttAuthored:   authoredShareV2(r, g.Attacker),
		Substituted:   substituted,
		// Substitutions are counted from DecisionSources, which is gated by
		// the same ProvenanceVersion as the authorship share -- so the two
		// are known together or unknown together.
		SubstitutedKnown:  trust.ProvenanceKnown,
		Assist:            trust,
		RejectedDef:       rejectedCountV2(r, g.Defender),
		RejectedAtt:       rejectedCountV2(r, g.Attacker),
		RejectedDefReason: dominantRejectionV2(r, g.Defender),
		Ruleset:           rulesetStampV2(g, m.ruleset),
	}
}

// rejectedCountV2 sums every rejected-action counter belonging to playerID.
func rejectedCountV2(r eng.MatchResult, playerID string) int {
	total := 0
	for key, n := range r.RejectedActions {
		if key == playerID || strings.HasPrefix(key, playerID+":") {
			total += n
		}
	}
	return total
}

// dominantRejectionV2 names the most common rejection cause for playerID as
// prose, or "" when there is nothing to report. It reports the single
// largest bucket rather than a breakdown because the row it feeds has space
// for one clause, and the largest bucket is the one worth acting on.
func dominantRejectionV2(r eng.MatchResult, playerID string) string {
	best, bestN := "", 0
	for key, n := range r.RejectedActions {
		if !strings.HasPrefix(key, playerID+":") || n <= bestN {
			continue
		}
		best, bestN = key, n
	}
	if best == "" {
		return ""
	}
	reason := best[strings.LastIndex(best, ":")+1:]
	return strings.ReplaceAll(reason, "_", " ")
}

func (m model) gameOverDataV2(r eng.MatchResult, trust TrustState) GameOverData {
	g := m.game

	winnerName, winnerRole := "", ""
	if g.Winner != "" {
		winnerName = g.ModelNames[g.Winner]
		winnerRole = "ATTACKER"
		if g.Winner == g.Defender {
			winnerRole = "DEFENDER"
		}
	}

	endedBy, endedDetail := endReasonV2(g, int(m.tickHorizon()), r.WinReason)

	return GameOverData{
		WinnerName: winnerName, WinnerRole: winnerRole,
		EndedBy: endedBy, EndedDetail: endedDetail,
		Wave: g.Wave, MaxWave: g.MaxWaves,
		Lives: g.Lives[g.Defender], MaxLives: g.StartingLives,
		DefName: g.ModelNames[g.Defender], AttName: g.ModelNames[g.Attacker],
		DefScore: g.Score[g.Defender], AttScore: g.Score[g.Attacker],
		DefAuthored:       authoredShareV2(r, g.Defender),
		AttAuthored:       authoredShareV2(r, g.Attacker),
		DefSaves:          savesStatV2(r, g.Defender),
		AttSaves:          savesStatV2(r, g.Attacker),
		Assist:            trust,
		RejectedDef:       rejectedCountV2(r, g.Defender),
		RejectedDefReason: dominantRejectionV2(r, g.Defender),
		Cost:              "pricing unset -- unknown",
		Verdict:           verdictV2(endedBy, trust),
	}
}

// endReasonV2 names how the match ended, in the card's two-part form: a
// shouted headline this file derives from board state, and a detail clause
// that is the ENGINE's own recorded WinReason, verbatim.
//
// Carrying the engine's string through rather than paraphrasing it is the
// point. WinReason is what BuildMatchResult writes into the replay, the
// manifest and the markdown report, and it is the token anyone
// cross-referencing a screenshot against a result file will search for. A UI
// that displayed a prettier synonym would make the final card and the
// artifact for the same match disagree about why it ended.
func endReasonV2(g *eng.Game, maxTicks int, winReason string) (string, string) {
	detail := winReason
	if detail == "" {
		detail = "reason not recorded"
	}

	switch {
	case g.Lives[g.Defender] <= 0:
		return "CORE LOST", detail
	case g.Wave >= g.MaxWaves:
		return "WAVES CLEARED", detail
	case maxTicks > 0 && g.TickCount >= int64(maxTicks):
		return fmt.Sprintf("TICK CAP %d", maxTicks), detail
	default:
		return "ENDED", detail
	}
}

// verdictV2 is the card's trust judgement: how the match ended, plus whether
// the engine had a hand in it. An engine-assisted result is labelled as such
// on the final card because that is the panel people screenshot, and a
// result that the engine helped produce should not be quotable without the
// caveat attached.
func verdictV2(endedBy string, trust TrustState) string {
	switch {
	case !trust.AssistKnown:
		return endedBy + ", ASSISTS UNKNOWN"
	case trust.AssistsEnabled && trust.AssistCount > 0:
		return endedBy + ", ENGINE-ASSISTED"
	case trust.AssistsEnabled:
		return endedBy + ", assists armed but idle"
	default:
		return endedBy + ", no engine assistance"
	}
}

// --- the view -------------------------------------------------------------

// ViewV2 renders one frame of the live match through the redesigned layout.
// It returns exactly m.height rows of exactly m.width columns.
func (m model) ViewV2() string {
	// A zero terminal size means Bubble Tea has not delivered a WindowSizeMsg
	// yet. computeLayout normalised that to 80x24 rather than showing the
	// too-small notice, and this keeps doing so: a first frame that says
	// "terminal too small" before the real size arrives is a worse answer
	// than a frame drawn at the standard default.
	w, h := m.width, m.height
	if w == 0 || h == 0 {
		w, h = 80, 24
	}
	l := computeLayoutV2(w, h)
	if l.mode == modeNotice {
		return tooSmallNotice(w, h)
	}

	g := m.game
	result := g.BuildMatchResult()
	trust := trustStateV2(g, result, m.ruleset)

	frame := newFrameV2(l.w, l.h)

	blitV2(frame, l.header, RenderHeaderV2(l, m.headerDataV2(result), trust), l.w)

	viewportW := boardViewportWidth(g.MapWidth, l.board.w)
	panX := autoFollowPanX(g, viewportW)

	// The game-over card overlays whichever map pane this mode drew -- the
	// framed board in narrow/mid/wide, the borderless map in
	// minimum/compact. Both paths go through OverlayCenteredV2 so the card
	// appears in EVERY mode, not just the one the mockup happens to show it
	// in. At the narrow end the card is wider than the pane, and
	// OverlayCenteredV2 clips it rather than refusing: a cramped card is
	// still readable, a frame that grew a column is not.
	switch l.mode {
	case modeMinimum, modeCompact:
		blitV2(frame, l.label, renderLabelRowV2(g, rect{w: l.label.w, h: l.label.h}, trust, m.tickHorizon()), l.w)
		mapRows := renderMapPaneV2(g, rect{w: l.mapPane.w, h: l.mapPane.h}, panX)
		if g.GameOver {
			card := m.gameOverDataV2(result, trust)
			mapRows = OverlayCenteredV2(mapRows,
				RenderGameOverCardV2(card, gameOverCardWidth(card, l.mapPane.w)), l.mapPane.w)
		}
		blitV2(frame, l.mapPane, mapRows, l.w)

	default:
		var bl, br string
		if l.mode == modeWide {
			bl, br = boardBottomBorderKeyHintsV2()
		} else {
			bl, br = boardBottomBorderTrustBandV2(trust)
		}
		board := renderFramedBoardV2(g, rect{w: l.board.w, h: l.board.h}, panX, bl, br)
		if g.GameOver {
			card := m.gameOverDataV2(result, trust)
			board = OverlayCenteredV2(board,
				RenderGameOverCardV2(card, gameOverCardWidth(card, l.board.w)), l.board.w)
		}
		blitV2(frame, l.board, board, l.w)
	}

	if l.legend.area() > 0 {
		blitV2(frame, l.legend, renderLegendV2(g, rect{w: l.legend.w, h: l.legend.h}), l.w)
	}
	if l.rule.area() > 0 {
		blitV2(frame, l.rule, verticalRuleV2(l.rule.h), l.w)
	}
	if l.cards.area() > 0 {
		blitV2(frame, l.cards, RenderCardsV2(rect{w: l.cards.w, h: l.cards.h}, m.cardsDataV2(result)), l.w)
	}
	if l.timeline.area() > 0 {
		blitV2(frame, l.timeline,
			RenderTimelineV2(rect{w: l.timeline.w, h: l.timeline.h}, m.timelineDataV2(result, trust)), l.w)
	}

	blitV2(frame, l.feed, m.feedPaneV2(l), l.w)
	blitV2(frame, l.keys, []string{keyBarV2(m)}, l.w)

	if m.asciiMode {
		frame = asciiFoldRows(frame)
	}
	return strings.Join(frame, "\n")
}

// feedPaneV2 selects the feed pane's content: the move feed by default, or
// the raw log window when 'L' has toggled it on -- the same choice
// buildSideRows makes for the old view, kept so the key does the same thing
// in both.
func (m model) feedPaneV2(l layoutV2) []string {
	if m.showLogs {
		return fitLines(selectLogWindow(m.game.Logs, l.feed.h, m.logScroll), l.feed.w, l.feed.h)
	}
	return RenderFeedV2(m.game.ReplayEvents, l.feed.w, l.feed.h)
}

// keyBarV2 renders the single key-hint row at the bottom of every non-notice
// mode.
func keyBarV2(m model) string {
	if m.game.GameOver {
		return renderGameOverKeyText(m.showLogs)
	}
	return renderKeyText(m.paused, m.game.AIEnabled, m.showLogs)
}
