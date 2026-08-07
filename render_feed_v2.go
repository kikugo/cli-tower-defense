package main

// The move-feed pane: layout.feed's renderer.
//
// It replaced view_render.go's buildMoveFeed/formatMoveRow/renderMoveFeed,
// which are now deleted along with the rest of the pre-redesign view. The
// replay inspector renders through this too, on the events up to its
// playhead.
//
// The old feed (view_render.go) is the single most-criticised part of the
// current UI, for reasons a real captured frame makes concrete: roughly 90%
// of its visible rows were "REJECT plac | rejected:" repeated -- a model
// retrying an illegal action over and over, drowning out everything else,
// including the one or two rows (a breach, a provider error) that actually
// mattered. This file's design is a direct response to that measurement:
//
//  1. Collapse consecutive identical rows into one, with a count.
//  2. An outcome column (ok/x/!!/!/~) instead of burying the outcome in a
//     free-text "REJECT " prefix.
//  3. The engine is rendered as a third actor (">>>"/"ENGINE"), not folded
//     into DEF/ATT -- see engine/assist.go's ReplayEngineAssist.
//  4. Wave-boundary separator rows.
//  5. Truncate, never wrap -- every row costs exactly one terminal row.
//  6. Newest at the bottom.
//
// And the part the task brief cares about most: when the pane is only a
// few rows tall (it can be 0 rows at the extreme low end of a valid
// terminal size -- see main_layout_v2.go's height-allocation doc comment),
// simply keeping the newest N events is wrong. A breach or a provider error
// six rows back matters more than the routine save that happened one tick
// ago. See priorityTier below for the explicit ordering this enforces.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	eng "tower-defense/engine"
)

// --- Row model --------------------------------------------------------

// feedRowKind buckets a feedRow for priorityTier's ordering. It is a
// separate axis from the marker string (which is what's actually drawn) --
// e.g. both "rejected" and "substituted" rows share tier 5 but carry
// different markers ("x" vs "~").
type feedRowKind int

const (
	feedApplied feedRowKind = iota
	feedRejected
	feedSubstituted
	feedBreach
	feedProviderError
	feedGameEnd
	feedEngine
	feedWaveSeparator
)

// feedRow is one rendered line of the move feed: either a real event (or a
// collapsed run of identical ones) or a synthetic wave-boundary separator.
// origIndex is the row's position in the ORIGINAL chronological build order
// -- kept as an explicit field (not implied by slice position) because
// collapseFeedRows and selectFeedRowsV2 both reorder/filter the slice this
// row lives in, and priority selection's recency tie-break needs to survive
// that.
type feedRow struct {
	origIndex int
	tick      int64
	kind      feedRowKind
	side      string // "DEF" / "ATT" / "***" (game end) / ">>>" (engine)
	action    string
	target    string
	marker    string // "ok" / "x" / "!!" / "!" / "~" / ">>" -- see requirement 2
	detail    string
	count     int // 1 normally; >1 once collapseFeedRows has merged a run
}

// --- Priority ordering --------------------------------------------------

// priorityTier is the explicit ordering the task brief requires: when the
// pane has fewer rows than there are candidate events, rows are kept by
// tier first (lower number = kept first), and only within a tier does
// recency (origIndex) decide. This is why a breach six rows back survives
// over a save from one tick ago -- the save's tier (6) never even gets
// compared against the breach's tier (1) on anything but raw row count.
//
// Ordering, and why:
//  0. game_end     -- the match ending is the single most important thing
//     the feed can ever show; it can never be starved out.
//  1. breach        -- the core got hit. The measured defect this whole
//     redesign responds to is a breach buried under forty rejection rows.
//  2. provider_error -- a turn where a model produced nothing at all is a
//     data-quality signal (see AUDIT-FOLLOWUP.md), not routine play.
//  3. engine assist  -- the engine acting as a third actor is exactly the
//     kind of thing a UI that only shows DEF/ATT would hide (requirement
//     3's whole justification); it outranks routine play but not the
//     things above it.
//  4. wave separator -- structural context (which wave a row belongs to)
//     helps read everything below it, so it's worth a slot before plain
//     decisions, but it is not itself gameplay signal.
//  5. rejected / substituted -- a rejection or a substitution is a model
//     failing to act as intended. Less urgent than a breach or an error,
//     but still more informative than a routine successful action --
//     "the model tried and failed" beats "the model saved again" for
//     understanding a match.
//  6. applied        -- ordinary successful play (place, spawn, wave,
//     research, upgrade, invest, save, ...). This is the tier that gets
//     starved first when rows are scarce, deliberately: it's the
//     "fourth consecutive save" the task brief names as the thing that
//     should NOT survive over a breach or a provider error.
func priorityTier(r feedRow) int {
	switch r.kind {
	case feedGameEnd:
		return 0
	case feedBreach:
		return 1
	case feedProviderError:
		return 2
	case feedEngine:
		return 3
	case feedWaveSeparator:
		return 4
	case feedRejected, feedSubstituted:
		return 5
	default: // feedApplied, and any unrecognized kind
		return 6
	}
}

// --- Building rows from the replay stream -------------------------------

// siblingKey indexes the one-shot "applied" events (ReplayPlacement,
// ReplaySpawn, and a non-engine-assisted ReplayWave) that always fire at
// the same tick, for the same player, with the same action string, as the
// ReplayOutcome event that represents the turn -- see feedSiblingIndex's
// doc comment for why this join exists at all instead of rendering those
// events directly.
type siblingKey struct {
	tick   int64
	player string
	action string
}

// feedSiblingIndex maps a siblingKey to the enrichment event recorded at
// that tick, built by buildFeedRowsV2 in a first pass over the stream.
type feedSiblingIndex map[siblingKey]eng.ReplayEvent

// buildFeedRowsV2 turns a game's full replay stream into feed rows, oldest
// first, before collapsing or priority selection.
//
// Design choice worth spelling out: this does NOT render eng.ReplayDecision
// or eng.ReplayRejected as their own rows, even though both are in
// isMoveFeedEvent's old (view_render.go) list. ReplayOutcome fires exactly
// once per applyDecision call and already carries the true result (applied
// / rejected / substituted, via Details["quality"]) -- rendering
// ReplayDecision too would print two rows for one turn (the old feed does
// exactly this for research/invest/upgrade/save/ability, which have no
// dedicated "applied" event type of their own), and ReplayRejected carries
// nothing ReplayOutcome's rejected case doesn't already have. Similarly,
// eng.ReplayPlacement / eng.ReplaySpawn / a non-engine-assisted
// eng.ReplayWave are never rendered as their own rows: every one of them is
// only ever recorded from inside the same applyDecision call that also
// produces the ReplayOutcome row for that turn (see engine/actions.go and
// engine/core.go), so they are folded into that row as position/detail
// enrichment via feedSiblingIndex instead of appearing twice.
func buildFeedRowsV2(events []eng.ReplayEvent) []feedRow {
	siblings := make(feedSiblingIndex)
	for _, ev := range events {
		switch ev.Type {
		case eng.ReplayPlacement, eng.ReplaySpawn:
			siblings[siblingKey{ev.Tick, ev.PlayerID, ev.Action}] = ev
		case eng.ReplayWave:
			if !ev.EngineAssisted {
				siblings[siblingKey{ev.Tick, ev.PlayerID, ev.Action}] = ev
			}
		}
	}

	var rows []feedRow
	nextIdx := 0
	emit := func(r feedRow) {
		r.origIndex = nextIdx
		nextIdx++
		if r.count == 0 {
			r.count = 1
		}
		rows = append(rows, r)
	}

	// Wave tracking: derived purely from the events actually on the stream
	// (ReplayWave for wave boundaries + queue size, ReplayBreach for leaks),
	// not from engine.WaveSummary/MatchResult -- those are a different data
	// structure the task brief's Data section doesn't authorize reading
	// from here ("Events come from engine.ReplayEvent"). This means the
	// "CLEARED" separator's leaked count is this pane's own derived
	// approximation (breaches seen since the last wave boundary), not the
	// engine's authoritative per-wave ledger -- see the report for this
	// gap called out explicitly.
	currentWave := 0
	leakedSinceWaveStart := 0

	for _, ev := range events {
		switch ev.Type {
		case eng.ReplayBreach:
			leakedSinceWaveStart++
			enemyType, _ := ev.Details["enemy_type"].(string)
			lives, _ := intFromDetails(ev.Details, "defender_lives")
			// The GLYPH, not the type name: this row has to be scannable
			// against the board it describes, and the board draws 'o', not
			// "basic". See glyphs_v2.go.
			target := strings.TrimSpace(string(enemyGlyph(enemyType)) + " " + posStr(ev.Position))
			emit(feedRow{
				tick:   ev.Tick,
				kind:   feedBreach,
				side:   "ATT",
				action: "BREACH",
				target: target,
				marker: "!!",
				detail: fmt.Sprintf("core hit   lives %d -> %d", lives+1, lives),
			})

		case eng.ReplayWave:
			newWave, _ := intFromDetails(ev.Details, "wave")
			queue, _ := intFromDetails(ev.Details, "queue")
			if currentWave > 0 && newWave > currentWave {
				emit(feedRow{
					tick:   ev.Tick,
					kind:   feedWaveSeparator,
					detail: fmt.Sprintf("WAVE %d CLEARED   %d leaked", currentWave, leakedSinceWaveStart),
				})
			}
			if newWave > 0 {
				emit(feedRow{
					tick:   ev.Tick,
					kind:   feedWaveSeparator,
					detail: fmt.Sprintf("WAVE %d OPENS   %d queued", newWave, queue),
				})
				currentWave = newWave
				leakedSinceWaveStart = 0
			}
			// The row for the turn that caused this (model-chosen or
			// applied_auto_wave) is emitted from the paired ReplayOutcome
			// below via siblings[]; an engine-assisted wave (applyAdaptivePressure's
			// AssistAutoWave branch, ev.EngineAssisted true) has no
			// ReplayOutcome pairing at all -- it bypasses applyDecision
			// entirely -- but IS already covered by its own
			// eng.ReplayEngineAssist row, so nothing more is emitted here
			// either way.

		case eng.ReplayProviderErr:
			emit(feedRow{
				tick:   ev.Tick,
				kind:   feedProviderError,
				side:   sideForRole(ev.Role),
				action: "ERROR",
				marker: "!",
				detail: humanizeFeedText(ev.Reason),
			})

		case eng.ReplayGameEnd:
			emit(feedRow{
				tick:   ev.Tick,
				kind:   feedGameEnd,
				side:   "***",
				action: "GAME END",
				marker: "!!",
				detail: humanizeFeedText(ev.Reason),
			})

		case eng.ReplayEngineAssist:
			emit(feedRow{
				tick:   ev.Tick,
				kind:   feedEngine,
				side:   ">>>",
				action: "ENGINE",
				marker: ">>",
				detail: engineAssistMessage(ev),
			})

		case eng.ReplayOutcome:
			emit(outcomeRow(ev, siblings))
		}
	}

	return rows
}

// outcomeRow renders a single ReplayOutcome event -- the authoritative
// per-turn record of what actually happened (applied / rejected /
// substituted, via Details["quality"], set by engine's
// classifyActionOutcome) -- into a feedRow, enriching applied
// place/spawn/wave turns with their sibling event's position and detail.
func outcomeRow(ev eng.ReplayEvent, siblings feedSiblingIndex) feedRow {
	side := sideForRole(ev.Role)
	action := ev.Action
	if action == "" {
		action = "save"
	}
	quality, _ := ev.Details["quality"].(string)

	row := feedRow{tick: ev.Tick, side: side, action: action}
	switch quality {
	case "primary", "fallback", "auto_corrected":
		row.kind = feedApplied
		row.marker = "ok"
		row.target, row.detail = enrichApplied(ev.Tick, ev.PlayerID, action, siblings)
	case "rejected":
		row.kind = feedRejected
		row.marker = "x"
		row.detail = humanizeFeedText(strings.TrimPrefix(ev.Reason, "rejected:"))
	case "substituted":
		row.kind = feedSubstituted
		row.marker = "~"
		row.detail = humanizeSubstitutedReason(ev.Reason)
		row.target, _ = enrichApplied(ev.Tick, ev.PlayerID, action, siblings)
	default:
		// Defensive only: classifyActionOutcome (engine/core.go) always
		// returns one of the four cases above, or "unknown" for an outcome
		// string matching none of its prefixes -- not reachable from any
		// current code path, kept so a future outcome shape degrades
		// gracefully instead of rendering a blank marker.
		row.kind = feedApplied
		row.marker = "?"
		row.detail = humanizeFeedText(ev.Reason)
	}
	return row
}

// enrichApplied looks up the sibling ReplayPlacement/ReplaySpawn/ReplayWave
// event recorded at the same tick/player/action (see siblingKey) and
// returns the position/detail text it contributes to the feed row. Returns
// ("", "") if no sibling was recorded -- e.g. every non-place/spawn/wave
// action (research, upgrade, invest, ability, save), which never had a
// position to begin with.
func enrichApplied(tick int64, player, action string, siblings feedSiblingIndex) (target, detail string) {
	sib, ok := siblings[siblingKey{tick, player, action}]
	if !ok {
		return "", ""
	}
	switch sib.Type {
	case eng.ReplayPlacement:
		// Glyph in the target column so the row scans against the board
		// ("place  ! 9,40"), prose name in the detail so the row is still
		// self-explanatory to someone who has not learned the glyphs yet
		// ("ok sniper   cost 100"). Both come from glyphs_v2.go, so this
		// column and the board can never disagree.
		towerType, _ := sib.Details["tower_type"].(string)
		cost, _ := intFromDetails(sib.Details, "cost")
		target = strings.TrimSpace(string(towerGlyph(towerType)) + " " + posStr(sib.Position))
		return target, strings.TrimSpace(fmt.Sprintf("%s   cost %d", towerType, cost))
	case eng.ReplaySpawn:
		enemyType, _ := sib.Details["enemy_type"].(string)
		cost, _ := intFromDetails(sib.Details, "cost")
		target = strings.TrimSpace(string(enemyGlyph(enemyType)) + " " + posStr(sib.Position))
		return target, strings.TrimSpace(fmt.Sprintf("%s   cost %d", enemyDisplayName(enemyType), cost))
	case eng.ReplayWave:
		newWave, _ := intFromDetails(sib.Details, "wave")
		queue, _ := intFromDetails(sib.Details, "queue")
		return "", fmt.Sprintf("wave %d   %d queued", newWave, queue)
	}
	return "", ""
}

// engineAssistMessage builds the detail text for a ReplayEngineAssist row
// (requirement 3) from Details["branch"] (see engine/assist.go's
// recordEngineAssist, which always sets it to one of the three AssistBranch
// constants) plus whatever branch-specific fields that call recorded.
//
// "branch not recorded" is rendered verbatim, per the task brief, whenever
// Details["branch"] is missing or empty. As of this writing that path is
// not reachable from any current engine code -- recordEngineAssist always
// sets it -- so this is a defensive fallback (e.g. for older serialized
// replay JSON predating this field, or a hand-built ReplayEvent in a test)
// rather than something a live match can currently produce. See the report
// for this gap.
func engineAssistMessage(ev eng.ReplayEvent) string {
	branch, ok := ev.Details["branch"].(string)
	if !ok || branch == "" {
		return "branch not recorded"
	}
	switch eng.AssistBranch(branch) {
	case eng.AssistReinforceWave:
		ability, _ := ev.Details["ability"].(string)
		if ability == "" {
			ability = "ability"
		}
		return fmt.Sprintf("fired %s for ATT", ability)
	case eng.AssistAutoWave:
		wave, _ := intFromDetails(ev.Details, "wave")
		return fmt.Sprintf("auto-launched wave %d", wave)
	case eng.AssistQueueEnemy:
		enemyType, _ := ev.Details["enemy_type"].(string)
		queue, _ := intFromDetails(ev.Details, "queue")
		return strings.TrimSpace(fmt.Sprintf("queued %c %s   queue %d",
			enemyGlyph(enemyType), enemyDisplayName(enemyType), queue))
	default:
		return "branch not recorded"
	}
}

// --- small helpers -------------------------------------------------------

func sideForRole(role string) string {
	switch role {
	case "defender":
		return "DEF"
	case "attacker":
		return "ATT"
	default:
		return "--"
	}
}

func posStr(p *eng.Position) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(p.Y) + "," + strconv.Itoa(p.X)
}

// intFromDetails reads an int out of a ReplayEvent.Details map, tolerant of
// the several numeric shapes that can land there (a literal int from
// in-process code, or float64/int64 from a JSON round-trip).
func intFromDetails(d map[string]interface{}, key string) (int, bool) {
	if d == nil {
		return 0, false
	}
	switch n := d[key].(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

// humanizeFeedText turns an engine-internal snake_case reason string into
// readable words, mirroring engine/report_markdown.go's humanizeReason
// (unexported there, so not callable from package main -- this is a
// deliberate, small duplication rather than an export change to engine).
func humanizeFeedText(reason string) string {
	if reason == "" {
		return "unknown"
	}
	return strings.ReplaceAll(reason, "_", " ")
}

// humanizeSubstitutedReason unpacks applyDecision's
// "substituted:<source>:<baseOutcome>" reason format (engine/actions.go)
// into "<source> -> <base outcome>", e.g.
// "substituted:provider_failure:applied_primary" becomes
// "provider failure -> applied primary". Falls back to a plain humanize if
// the reason doesn't have the expected two-colon shape.
func humanizeSubstitutedReason(reason string) string {
	parts := strings.SplitN(reason, ":", 3)
	if len(parts) != 3 {
		return humanizeFeedText(reason)
	}
	return humanizeFeedText(parts[1]) + " -> " + humanizeFeedText(parts[2])
}

// --- Collapsing ----------------------------------------------------------

// collapseFeedRows merges consecutive rows with identical rendered content
// (same kind/side/action/target/marker/detail -- everything except tick and
// count) into a single row with count incremented, per requirement 1. Wave
// separators are never merged into (or with) anything, even if two
// happened to render identical text, since they mark structural boundaries
// rather than repeated player behavior.
//
// The merged row keeps the LATEST occurrence's tick and origIndex, not the
// first: for priority selection (selectFeedRowsV2), a run's recency should
// reflect when it was last still happening, not when it started -- e.g. a
// rejection run that started 200 ticks ago and is still recurring right now
// should compete for a scarce row slot as "current", not "stale".
func collapseFeedRows(rows []feedRow) []feedRow {
	if len(rows) == 0 {
		return rows
	}
	out := make([]feedRow, 0, len(rows))
	for _, r := range rows {
		if n := len(out); n > 0 {
			last := &out[n-1]
			if last.kind != feedWaveSeparator && r.kind != feedWaveSeparator && sameFeedContent(*last, r) {
				last.count++
				last.tick = r.tick
				last.origIndex = r.origIndex
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

func sameFeedContent(a, b feedRow) bool {
	return a.kind == b.kind && a.side == b.side && a.action == b.action &&
		a.target == b.target && a.marker == b.marker && a.detail == b.detail
}

// --- Priority selection ---------------------------------------------------

// selectFeedRowsV2 returns AT MOST budget rows, in original chronological
// order. When rows already fit (len(rows) <= budget), nothing is dropped
// and chronology alone determines what's shown -- priority only kicks in
// once rows are genuinely scarce, exactly as the task brief specifies
// ("priority beats chronology when rows are scarce", not always).
//
// Selection is stable-sort by (priorityTier ascending, origIndex
// descending) -- i.e. lowest tier number first, and within a tier the most
// recent row first -- then the first `budget` of that ordering are kept,
// then the result is re-filtered back into original chronological order
// for display (newest at the bottom, requirement 6).
func selectFeedRowsV2(rows []feedRow, budget int) []feedRow {
	if budget <= 0 {
		return nil
	}
	if len(rows) <= budget {
		return rows
	}

	order := make([]int, len(rows))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		ri, rj := rows[order[i]], rows[order[j]]
		ti, tj := priorityTier(ri), priorityTier(rj)
		if ti != tj {
			return ti < tj
		}
		return ri.origIndex > rj.origIndex
	})

	keep := make(map[int]bool, budget)
	for _, i := range order[:budget] {
		keep[i] = true
	}

	out := make([]feedRow, 0, budget)
	for i, r := range rows {
		if keep[i] {
			out = append(out, r)
		}
	}
	return out
}

// --- Rendering to exact width --------------------------------------------

// Fixed column widths for a normal (non-separator) row. These are
// deliberately NOT re-measured with lipgloss.Width after concatenation --
// every piece is already forced to its exact width via padCells/
// truncateCells before being joined, so the arithmetic below is exact by
// construction, and the final padCells call at the end of
// renderFeedRowText is what actually GUARANTEES the exact-width invariant
// regardless of whether this arithmetic is even right (defense in depth:
// requirement 5 -- "truncate, never wrap" -- must hold even if a column
// width assumption above turns out wrong).
const (
	feedTickW   = 5
	feedSideW   = 3
	feedActionW = 18
	feedMarkerW = 2
	feedColSep  = " │ "
)

// renderFeedRowText renders one feedRow into EXACTLY width display columns,
// truncating (never wrapping) the detail field first as width shrinks --
// requirement 5. tick/side/action/marker are fixed-width columns that only
// get hard-truncated (via the closing padCells) at pathologically small
// widths well below any width main_layout_v2.go's compact/minimum modes
// actually produce (the pane floor is 60 columns); detail is the one field
// whose budget shrinks continuously with width, and reaches zero first.
func renderFeedRowText(r feedRow, width int) string {
	if width <= 0 {
		return ""
	}

	var tickStr, sideStr, actionStr, markerStr string
	fixedW := feedTickW + len(feedColSep) + feedSideW + len(feedColSep) + feedActionW + len(feedColSep)

	if r.kind == feedWaveSeparator {
		tickStr = strings.Repeat("─", feedTickW)
		sideStr = strings.Repeat("─", feedSideW)
		actionStr = strings.Repeat("─", feedActionW)
		// no marker column for a separator row -- the message goes straight
		// into the detail slot.
	} else {
		tickStr = fmt.Sprintf("%*d", feedTickW, r.tick)
		if len(tickStr) > feedTickW {
			tickStr = tickStr[len(tickStr)-feedTickW:] // never widen the column for a huge tick
		}
		// The side column is the one field in this pane that carries the
		// Phase 3 palette: it is what a reader scans down to find "who did
		// this", and colouring it means the engine's own rows (">>>", amber)
		// separate from the two players' at a glance. Styling is applied
		// AFTER padCells so the pad is plain spaces inside the colour run
		// rather than outside it, and so the column's display width is
		// decided before any escape bytes exist.
		sideStr = sideStyleV2(r.side).Render(padCells(r.side, feedSideW))
		actLine := r.action
		if r.target != "" {
			actLine += " " + r.target
		}
		actionStr = padCells(truncateCells(actLine, feedActionW), feedActionW)
		markerStr = padCells(r.marker, feedMarkerW)
		fixedW += feedMarkerW + 1 // marker column plus the space before detail
	}

	countSuffix := ""
	if r.count > 1 {
		countSuffix = fmt.Sprintf(" x%d", r.count)
	}

	detailBudget := width - fixedW - len(countSuffix)
	if detailBudget < 0 {
		detailBudget = 0
	}
	detailStr := truncateCells(r.detail, detailBudget)

	var b strings.Builder
	b.WriteString(tickStr)
	b.WriteString(feedColSep)
	b.WriteString(sideStr)
	b.WriteString(feedColSep)
	b.WriteString(actionStr)
	b.WriteString(feedColSep)
	if r.kind != feedWaveSeparator {
		b.WriteString(markerStr)
		b.WriteString(" ")
	}
	b.WriteString(detailStr)
	b.WriteString(countSuffix)

	// padCells is the actual guarantor of the exact-width invariant: it
	// pads a short row with plain spaces, or hard-truncates a row that ran
	// long despite the budgeting above -- e.g. a pathologically small
	// width, or a countSuffix that pushed just past the edge. It is
	// ANSI-safe, which now matters: since Phase 3 the side column carries
	// colour, so rows from this pane must be measured with lipgloss.Width,
	// never with mockup_fit_test.go's frameDisplayWidth.
	return padCells(b.String(), width)
}

// --- Entry point -----------------------------------------------------------

// RenderFeedV2 renders events (a game's full ReplayEvents history) into
// EXACTLY height rows of EXACTLY width columns -- layout.feed's contract,
// checked by checkFits. Building goes: filter+enrich into feedRows
// (buildFeedRowsV2), collapse consecutive duplicates (collapseFeedRows,
// requirement 1), then keep at most `height` of them by priority
// (selectFeedRowsV2, the "priority beats chronology when scarce" rule) --
// and only THEN render each surviving row to width and pad the top with
// blank rows if there were fewer events than the budget, so newest ends up
// at the bottom (requirement 6) exactly like view_render.go's
// renderMoveFeed does today.
func RenderFeedV2(events []eng.ReplayEvent, width, height int) []string {
	if height <= 0 {
		return blankRows(0, width)
	}
	if width <= 0 {
		return blankRows(height, width)
	}

	rows := buildFeedRowsV2(events)
	rows = collapseFeedRows(rows)
	rows = selectFeedRowsV2(rows, height)

	lines := make([]string, 0, height)
	for _, r := range rows {
		lines = append(lines, renderFeedRowText(r, width))
	}
	for len(lines) < height {
		lines = append([]string{padCells("", width)}, lines...)
	}
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	return lines
}
