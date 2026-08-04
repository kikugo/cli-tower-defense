package engine

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type ScriptedProvider struct {
	config ResolvedPlayerModelConfig

	// hoarderSpending is defender_hoarder's own state, carried across ticks:
	// once its balance reaches hoarderBankThreshold it flips true and keeps
	// spending every tick (through scriptedDefenderBuild) until that helper
	// can no longer place or upgrade, at which point it flips back to false
	// and the script banks again. A provider is constructed once per game
	// (see providerFromResolvedConfig / NewGameFromResolvedConfig) and its
	// GetTowerDecision is only ever invoked once at a time -- HandleAIDecisions
	// will not dispatch a new turn while either player's AIThinking flag is
	// still set from a prior dispatch -- so this field is safe to mutate
	// without synchronization. Unused by every other script.
	hoarderSpending bool

	// liveLikeDecisions / liveLikePendingWave / liveLikeSpawnCursor are
	// attacker_live_like's own state, carried across turns the same way
	// hoarderSpending is above (see that comment for why a plain struct
	// field is safe here without synchronization). All three implement
	// "banking toward a target" -- sticking on one scheduled thing across
	// however many consecutive decisions it takes to afford it, rather than
	// skipping ahead the moment it's unaffordable:
	//
	//   - liveLikeDecisions counts every GetEnemyDecision call while no wave
	//     is pending, so the "every 14th decision" wave-attempt timing lands
	//     deterministically. It is reset to 0 only when a wave actually
	//     fires (see liveLikePendingWave), not merely attempted.
	//   - liveLikePendingWave is set once liveLikeDecisions reaches
	//     liveLikeWaveInterval and stays set -- with the wave attempted on
	//     every subsequent decision -- until "wave" is actually affordable
	//     and taken. This is what makes the wave attempt sticky rather than
	//     a one-shot check that silently drops if unaffordable that tick.
	//   - liveLikeSpawnCursor indexes liveLikeSpawnSchedule and advances
	//     ONLY when a spawn is actually emitted, never on a "save". A
	//     scheduled unit that isn't affordable yet is retried at the same
	//     cursor position on every subsequent decision -- that stickiness is
	//     what "bank toward it" in the brief means: sitting on e.g. a
	//     40-cost shielded across as many turns as it takes, rather than the
	//     cursor moving on and letting a cheaper unit clear in its place.
	//
	// liveLikeDecisions and liveLikeSpawnCursor remain two separate counters
	// (not one): the measured spawn composition this script reproduces is a
	// share of spawns only, independent of how often a wave happens to
	// land, so coupling them would let wave timing distort the composition.
	// Unused by every other script.
	liveLikeDecisions   int
	liveLikePendingWave bool
	liveLikeSpawnCursor int
}

func NewScriptedProvider(config ResolvedPlayerModelConfig) *ScriptedProvider {
	return &ScriptedProvider{config: config}
}

func (p *ScriptedProvider) Name() string {
	return fmt.Sprintf("%s/%s", p.config.Provider, p.config.Model)
}

func (p *ScriptedProvider) GetTowerDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	switch p.config.Model {
	case "defender_invest":
		return map[string]interface{}{"action": "invest", "reason": "scripted"}, nil
	case "defender_baseline":
		// Competent-but-simple defense: build coverage while placement is
		// affordable, upgrade once the board saturates, save when broke.
		return scriptedDefenderBuild(gameState, "basic", "baseline"), nil
	case "defender_sniper":
		// Same build-coverage/upgrade/save logic as defender_baseline, but
		// commits exclusively to the sniper tower, so a sweep against it
		// isolates that tower's cost-efficiency from placement strategy.
		return scriptedDefenderBuild(gameState, "sniper", "sniper"), nil
	case "defender_splash":
		return scriptedDefenderBuild(gameState, "splash", "splash"), nil
	case "defender_buffer":
		return scriptedDefenderBuild(gameState, "buffer", "buffer"), nil
	case "defender_hoarder":
		// Banks to hoarderBankThreshold, then spends it down, instead of
		// defender_baseline's trickle-out-one-tower-the-moment-it-can-afford
		// behaviour. Same tower type (basic) and the same shared placement
		// helper as defender_baseline, so the two scripts differ only in
		// *when* they spend, never in *where* they place -- see
		// scriptedDefenderHoard.
		return p.scriptedDefenderHoard(gameState), nil
	case "defender_research_economy":
		// Commits to the economy research line before behaving exactly like
		// defender_baseline -- see scriptedDefenderResearch.
		return scriptedDefenderResearch(gameState, "economy", "research_economy"), nil
	case "defender_research_range":
		// Commits to the range research line before behaving exactly like
		// defender_baseline -- see scriptedDefenderResearch. Because this
		// script buys "range" before it owns any tower, and researchTech's
		// "range" branch (engine/actions.go) only bumps the Range of towers
		// that already exist at purchase time, this ordering makes the
		// range purchase a guaranteed no-op. That is the property under
		// measurement here, not a bug in this script -- see
		// TestResearchRangeOrderingIsANoOpWithoutExistingTowers.
		return scriptedDefenderResearch(gameState, "range", "research_range"), nil
	case "defender_research_control":
		// Commits to the control research line before behaving exactly like
		// defender_baseline -- see scriptedDefenderResearch.
		return scriptedDefenderResearch(gameState, "control", "research_control"), nil
	case "defender_slowzone":
		// Commits to placing slow zones on live enemy path tiles before
		// behaving exactly like defender_baseline -- see
		// scriptedDefenderSlowZone.
		return scriptedDefenderSlowZone(gameState), nil
	case "defender_basic_buffer":
		// Plays exactly defender_baseline, except it also builds buffer
		// towers when doing so actually boosts existing damage towers -- see
		// scriptedDefenderBasicBuffer. defender_buffer builds nothing but
		// buffers, which deal zero damage themselves (see runTowerPhase in
		// engine/actions.go), so that script can never measure the buffer's
		// cost-efficiency: it always loses. This mixed script is the one
		// that can.
		return scriptedDefenderBasicBuffer(gameState), nil
	default:
		if candidates, ok := gameState["valid_tower_candidates"].([][]int); ok && len(candidates) > 0 {
			return map[string]interface{}{
				"action":     "place",
				"tower_type": "basic",
				"position":   []interface{}{float64(candidates[0][0]), float64(candidates[0][1])},
				"reason":     "scripted",
			}, nil
		}
		return map[string]interface{}{"action": "save", "reason": "scripted"}, nil
	}
}

func (p *ScriptedProvider) GetEnemyDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	switch p.config.Model {
	case "attacker_spawn":
		return map[string]interface{}{"action": "spawn", "enemy_type": "basic", "reason": "scripted"}, nil
	case "attacker_shielded":
		// Spawns shielded (never basic/fast) whenever affordable, so a sweep
		// against this script isolates the shielded enemy's cost-efficiency
		// from the mixed-composition income cap that attacker_baseline is
		// bound by. Saves when shielded isn't affordable yet, rather than
		// falling back to a cheaper unit.
		affordable, _ := gameState["affordable_actions"].([]string)
		for _, action := range affordable {
			if action == "spawn:shielded" {
				return map[string]interface{}{"action": "spawn", "enemy_type": "shielded", "reason": "scripted: shielded"}, nil
			}
		}
		return map[string]interface{}{"action": "save", "reason": "scripted: save"}, nil
	case "attacker_healer":
		// Spawns real healer-typed units (never a restatted basic) whenever
		// affordable, so a sweep against this script isolates the healer's
		// heal ability -- keyed on EnemyType == "healer" in
		// engine/actions.go -- from its body stats, which restatting basic
		// cannot reach. Identical to the default/attacker_baseline branch
		// below in every other respect: same wave-launch condition, reading
		// only this player's own balance via your_resources (see the comment
		// on the default branch below for why gameState["resources"] itself
		// is the wrong thing to read here), and the same fallback to
		// spawning basic when healer isn't affordable yet.
		if r, ok := toIntFromAny(gameState["your_resources"]); ok && r >= 260 {
			return map[string]interface{}{"action": "wave", "reason": "scripted"}, nil
		}
		affordable, _ := gameState["affordable_actions"].([]string)
		for _, action := range affordable {
			if action == "spawn:healer" {
				return map[string]interface{}{"action": "spawn", "enemy_type": "healer", "reason": "scripted: healer"}, nil
			}
		}
		return map[string]interface{}{"action": "spawn", "enemy_type": "basic", "reason": "scripted"}, nil
	case "attacker_surge":
		// Fires the surge ability whenever legal, so a sweep against this
		// script isolates surge's cost-efficiency from the mixed spawn/wave
		// economy attacker_baseline is bound by. Otherwise identical to the
		// default/attacker_baseline branch -- see scriptedAttackerAbility.
		return scriptedAttackerAbility(gameState, "surge"), nil
	case "attacker_shield_burst":
		// Same isolation as attacker_surge, for shield_burst -- see
		// scriptedAttackerAbility.
		return scriptedAttackerAbility(gameState, "shield_burst"), nil
	case "attacker_reinforce":
		// Same isolation as attacker_surge, for reinforce_wave -- see
		// scriptedAttackerAbility. Note applyAdaptivePressure
		// (engine/actions.go) can itself fire reinforce_wave when the
		// attacker has been idle, independently of this script's own
		// choice; see the ActionCounters caveat documented on
		// scriptedAttackerAbility.
		return scriptedAttackerAbility(gameState, "reinforce_wave"), nil
	case "attacker_live_like":
		// Reproduces the decision mix and spawn composition measured from a
		// real model (gemini-2.5-flash-lite) playing this seat against
		// defender_baseline over 4 matches / 230 attacker decisions, rather
		// than attacker_baseline's spawn-basic-until-260-then-wave script,
		// which every existing balance number was measured against -- see
		// scriptedAttackerLiveLike for the wave-timing/spawn-schedule
		// mechanics and liveLikeSpawnSchedule for where the measured
		// shielded/basic/fast/tank composition comes from.
		return p.scriptedAttackerLiveLike(gameState), nil
	default:
		// Launch a wave once THIS player's own bank reaches the threshold --
		// "hold until rich enough for a big push". gameState["resources"] is
		// keyed over ALL players (see getGameState in engine/actions.go), so
		// scanning it for any entry >= 260 used to launch a wave in response
		// to the opponent's (here, the defender's) balance instead of this
		// player's own. your_resources (added in getPlayerGameState) is this
		// player's own balance, so read that instead.
		return scriptedAttackerDefault(gameState), nil
	}
}

// scriptedAttackerDefault is the attacker_baseline behaviour (also the
// switch's default branch above): launch a wave once this player's own bank
// reaches the threshold, otherwise spawn basic. Extracted into its own
// function so attacker_surge/attacker_shield_burst/attacker_reinforce can
// fall back to exactly this logic, rather than duplicating it, when their
// named ability isn't currently legal.
func scriptedAttackerDefault(gameState map[string]interface{}) map[string]interface{} {
	if r, ok := toIntFromAny(gameState["your_resources"]); ok && r >= 260 {
		return map[string]interface{}{"action": "wave", "reason": "scripted"}
	}
	return map[string]interface{}{"action": "spawn", "enemy_type": "basic", "reason": "scripted"}
}

// scriptedAttackerAbility isolates one attacker ability's cost-efficiency
// (see useAttackerAbility in engine/actions.go) from the mixed spawn/wave
// economy attacker_baseline is bound by: it fires the named ability
// whenever "ability:<ability>" is legal, and otherwise falls back to
// exactly scriptedAttackerDefault (attacker_baseline's own behaviour)
// rather than duplicating that logic. Shared by attacker_surge,
// attacker_shield_burst, and attacker_reinforce.
//
// Caveat for attacker_reinforce specifically: applyAdaptivePressure
// (engine/actions.go) can call useAttackerAbility("reinforce_wave")
// directly, on its own initiative, when the attacker has been idle for
// several ticks with a quiet board. That path never builds a decision map
// and never goes through applyDecision, so it does NOT increment
// g.ActionCounters the way a player-chosen ability use (via this function)
// does -- see TestApplyAdaptivePressureReinforceWaveDoesNotIncrementActionCounters.
// A sweep reading ActionCounters to see how often reinforce_wave "fired"
// will therefore undercount: it only reflects this script's own choices,
// not engine-initiated ones.
func scriptedAttackerAbility(gameState map[string]interface{}, ability string) map[string]interface{} {
	affordable, _ := gameState["affordable_actions"].([]string)
	abilityAction := "ability:" + ability
	for _, action := range affordable {
		if action == abilityAction {
			return map[string]interface{}{"action": "ability", "ability": ability, "reason": "scripted: " + ability}
		}
	}
	return scriptedAttackerDefault(gameState)
}

// liveLikeWaveInterval is how often (once every this-many decisions)
// attacker_live_like attempts a wave, calibrated to the measured live rate
// of 16 waves over 230 attacker decisions (7.0%): 1/14 = 7.14%, the closest
// integer interval to that measured share.
const liveLikeWaveInterval = 14

// liveLikeUnitWeight pairs a spawn enemy type with its integer share of the
// 40-slot composition cycle below. Order is significant: it is also the
// fixed tie-break order buildLiveLikeSpawnSchedule uses, so it must not
// change across runs (see that function).
type liveLikeUnitWeight struct {
	unitType string
	weight   int
}

// liveLikeUnitWeights is proportioned to the measured live-attacker spawn
// composition (share of the 88 measured spawns): shielded 39.8%, basic
// 33.0%, fast 22.7%, tank 4.5%. A 40-slot cycle of 16/13/9/2 lands within
// half a point of every one of those shares (40.0/32.5/22.5/5.0).
var liveLikeUnitWeights = []liveLikeUnitWeight{
	{"shielded", 16},
	{"basic", 13},
	{"fast", 9},
	{"tank", 2},
}

// liveLikeSpawnSchedule is the fixed, interleaved 40-entry unit cycle
// attacker_live_like steps through for its spawn decisions (see
// scriptedAttackerLiveLike), computed once at package init by
// buildLiveLikeSpawnSchedule so the measured composition is spread through
// a match instead of front-loaded into four separate blocks.
var liveLikeSpawnSchedule = buildLiveLikeSpawnSchedule()

// buildLiveLikeSpawnSchedule interleaves liveLikeUnitWeights into a single
// 40-entry cycle using smooth weighted round-robin (the same technique
// nginx uses to spread weighted upstreams evenly rather than in blocks):
// every slot, each unit's accumulator gains its own weight, the unit with
// the largest accumulator is emitted and has the cycle's total weight
// (40) subtracted back off. This is deterministic and does not depend on Go
// map iteration order -- liveLikeUnitWeights is an ordered slice, and ties
// (which occur, since weights are integers) are broken by that fixed order,
// because the loop below only replaces the running best on a strict >.
func buildLiveLikeSpawnSchedule() []string {
	const cycleLen = 40
	total := 0
	for _, uw := range liveLikeUnitWeights {
		total += uw.weight
	}
	accumulator := make([]int, len(liveLikeUnitWeights))
	schedule := make([]string, 0, cycleLen)
	for i := 0; i < cycleLen; i++ {
		best := -1
		for idx, uw := range liveLikeUnitWeights {
			accumulator[idx] += uw.weight
			if best == -1 || accumulator[idx] > accumulator[best] {
				best = idx
			}
		}
		schedule = append(schedule, liveLikeUnitWeights[best].unitType)
		accumulator[best] -= total
	}
	return schedule
}

// scriptedAttackerLiveLike implements attacker_live_like: calibrated to
// what a real model (gemini-2.5-flash-lite) actually did playing this seat
// against defender_baseline, rather than attacker_baseline's
// spawn-basic-until-rich-then-wave script that every existing balance
// number was measured against.
//
// Every liveLikeWaveInterval-th decision arms a pending wave (see
// liveLikePendingWave on ScriptedProvider): once armed, a wave is attempted
// on every subsequent decision -- not just the triggering one -- until
// "wave" is actually legal and taken, which then disarms it and resets the
// decision counter. This is "bank toward the wave" the same way the spawn
// schedule banks toward its scheduled unit below: an unaffordable wave
// attempt falls through to the spawn logic for that turn (never saves on
// principle), and the next decision tries the wave again rather than
// abandoning it.
//
// Every decision without an armed-and-firing wave, it consults
// liveLikeSpawnSchedule at p.liveLikeSpawnCursor (see that field's comment
// on ScriptedProvider): if the scheduled unit is affordable it is spawned
// and the cursor advances to the next slot; if not, this returns "save"
// without substituting a cheaper unit AND without moving the cursor, so the
// same slot is retried next time. Both of these -- refusing to substitute
// and refusing to advance past an unaffordable slot -- are what reproduces
// the measured ~55% save share and the measured composition as an emergent
// consequence of banking toward 40-cost shielded and 50-cost tank on a
// small income, rather than a hard-coded probability or a schedule that
// silently skips what it can't afford.
//
// Reads only gameState["affordable_actions"] -- there is no separate
// balance threshold to gate on here (unlike scriptedAttackerDefault's own
// wave-launch check), so unlike that function this never touches
// gameState["your_resources"] or gameState["resources"] at all.
func (p *ScriptedProvider) scriptedAttackerLiveLike(gameState map[string]interface{}) map[string]interface{} {
	p.liveLikeDecisions++
	affordable, _ := gameState["affordable_actions"].([]string)

	if !p.liveLikePendingWave && p.liveLikeDecisions%liveLikeWaveInterval == 0 {
		p.liveLikePendingWave = true
	}
	if p.liveLikePendingWave {
		for _, action := range affordable {
			if action == "wave" {
				p.liveLikePendingWave = false
				p.liveLikeDecisions = 0
				return map[string]interface{}{"action": "wave", "reason": "live_like: scheduled wave"}
			}
		}
		// Wave isn't legal yet -- stay armed (do not clear
		// liveLikePendingWave) and fall through to the spawn schedule below
		// rather than saving on principle. The next decision tries the wave
		// again before it tries anything else.
	}

	unitType := liveLikeSpawnSchedule[p.liveLikeSpawnCursor%len(liveLikeSpawnSchedule)]
	spawnAction := "spawn:" + unitType
	for _, action := range affordable {
		if action == spawnAction {
			p.liveLikeSpawnCursor++
			return map[string]interface{}{"action": "spawn", "enemy_type": unitType, "reason": "live_like: " + unitType}
		}
	}
	// Scheduled unit isn't affordable yet -- save toward it and leave the
	// cursor where it is (do not advance) so the same unit is retried next
	// decision instead of a cheaper one clearing in its place.
	return map[string]interface{}{"action": "save", "reason": "live_like: banking for " + unitType}
}

// scriptedDefenderBuild spreads towers of towerType along the path while
// placement is affordable, upgrades an existing tower once the board
// saturates, and saves when broke. Shared by defender_baseline and the
// single-tower-type sweep scripts (defender_sniper, defender_splash,
// defender_buffer) so they differ only in which tower they commit to.
func scriptedDefenderBuild(gameState map[string]interface{}, towerType, label string) map[string]interface{} {
	affordable, _ := gameState["affordable_actions"].([]string)
	canPlace := false
	upgradeID := -1
	placeAction := "place:" + towerType
	for _, action := range affordable {
		if action == placeAction {
			canPlace = true
		}
		if upgradeID < 0 && strings.HasPrefix(action, "upgrade:") {
			if id, err := strconv.Atoi(strings.TrimPrefix(action, "upgrade:")); err == nil {
				upgradeID = id
			}
		}
	}
	if canPlace {
		if candidates, ok := gameState["valid_tower_candidates"].([][]int); ok && len(candidates) > 0 {
			// Spread towers along the path instead of clustering at the
			// entrance: stride through the candidate list by how many
			// towers already exist.
			built := 0
			if towers, ok := gameState["towers"].([]interface{}); ok {
				built = len(towers)
			}
			idx := (built * 3) % len(candidates)
			return map[string]interface{}{
				"action": "place", "tower_type": towerType,
				"position": []interface{}{float64(candidates[idx][0]), float64(candidates[idx][1])},
				"reason":   label + ": build coverage",
			}
		}
	}
	if upgradeID >= 0 {
		return map[string]interface{}{"action": "upgrade", "tower_id": float64(upgradeID), "reason": label + ": strengthen"}
	}
	return map[string]interface{}{"action": "save", "reason": label + ": save"}
}

// hoarderBankThreshold is the balance defender_hoarder waits for before it
// starts spending. Chosen deliberately, not arbitrarily:
//
//   - It is the game's starting balance (see StartingResources in
//     engine/ruleset.go / DefaultArenaRuleset), so the very first decision of
//     a match is already at-or-above threshold and the script begins in
//     spend mode rather than needing several ticks to warm up.
//   - It is at or above every placeable tower's cost (basic 100, splash 200,
//     sniper 250, buffer 300 -- see DefaultBalanceConfig), so once the bank
//     reaches it every tower type, place_slow_zone (150), every research
//     track (140/160/180), and invest (150) are simultaneously legal. That
//     is the richest possible choice set the economy can produce at a single
//     decision point, which is exactly the property Task 2 measures.
//   - 100 (defender_baseline's own placement cost) was rejected on purpose:
//     banking to the cost of the very thing you are about to buy produces no
//     behavioural difference from spending the instant you can afford it --
//     it would just be defender_baseline with extra steps.
const hoarderBankThreshold = 300

// scriptedDefenderHoard implements defender_hoarder: save while below
// hoarderBankThreshold; once at or above it, delegate to
// scriptedDefenderBuild (same "basic" tower type and placement logic as
// defender_baseline, held constant on purpose) every tick until that helper
// can no longer place or upgrade -- i.e. it falls back to "save" on its own
// -- at which point hoarding resumes. This is a stateful, multi-tick spend
// phase, not just "place once whenever above threshold": p.hoarderSpending
// (see ScriptedProvider) tracks which phase the script is currently in
// across calls, so a spend phase begun at >= 300 keeps spending as the
// balance falls through 299, 200, 150, ... down to whatever the shared
// helper can no longer afford, rather than flipping back to banking the
// moment the balance dips back under the threshold.
func (p *ScriptedProvider) scriptedDefenderHoard(gameState map[string]interface{}) map[string]interface{} {
	if !p.hoarderSpending {
		res, ok := toIntFromAny(gameState["your_resources"])
		if !ok || res < hoarderBankThreshold {
			return map[string]interface{}{"action": "save", "reason": "hoarder: banking to threshold"}
		}
		p.hoarderSpending = true
	}
	decision := scriptedDefenderBuild(gameState, "basic", "hoarder")
	if action, _ := decision["action"].(string); action == "save" {
		// scriptedDefenderBuild itself could neither place nor upgrade --
		// genuinely broke, not just under threshold. End the spend phase so
		// the next call re-checks against hoarderBankThreshold instead of
		// spending on every future tick that happens to afford one tower.
		p.hoarderSpending = false
	}
	return decision
}

// scriptedDefenderResearch commits to buying one research tech line before
// behaving exactly like defender_baseline, in the same style
// scriptedDefenderBuild isolates a single tower type: while
// "research:<tech>" is legal, take it; while tech isn't maxed yet but that
// isn't currently affordable, save (bank toward it rather than trickling
// the money into a tower); once tech is exhausted (level 2, no longer
// offered), delegate to scriptedDefenderBuild("basic", ...) so the script
// plays exactly like defender_baseline from then on. Shared by
// defender_research_economy, defender_research_range, and
// defender_research_control -- see the callers in GetTowerDecision for the
// deliberate consequence this ordering has for "range" specifically.
func scriptedDefenderResearch(gameState map[string]interface{}, tech, label string) map[string]interface{} {
	affordable, _ := gameState["affordable_actions"].([]string)
	researchAction := "research:" + tech
	for _, action := range affordable {
		if action == researchAction {
			return map[string]interface{}{"action": "research", "tech": tech, "reason": label + ": research " + tech}
		}
	}
	maxed := false
	if research, ok := gameState["research"].(map[string]interface{}); ok {
		if lvl, ok := toIntFromAny(research[tech]); ok && lvl >= 2 {
			maxed = true
		}
	}
	if !maxed {
		return map[string]interface{}{"action": "save", "reason": label + ": banking toward " + tech}
	}
	return scriptedDefenderBuild(gameState, "basic", label)
}

// scriptedDefenderSlowZone commits to placing slow zones on live enemy path
// tiles before behaving exactly like defender_baseline: while
// "place_slow_zone" is legal AND the script can name a legal path tile
// (via scriptedSlowZoneTarget), place one there; otherwise fall through to
// scriptedDefenderBuild's build-coverage/upgrade/save behaviour, unchanged.
func scriptedDefenderSlowZone(gameState map[string]interface{}) map[string]interface{} {
	affordable, _ := gameState["affordable_actions"].([]string)
	canPlaceSlowZone := false
	for _, action := range affordable {
		if action == "place_slow_zone" {
			canPlaceSlowZone = true
			break
		}
	}
	if canPlaceSlowZone {
		if y, x, ok := scriptedSlowZoneTarget(gameState); ok {
			return map[string]interface{}{
				"action":   "place_slow_zone",
				"position": []interface{}{float64(y), float64(x)},
				"reason":   "slowzone: place on enemy path tile",
			}
		}
	}
	return scriptedDefenderBuild(gameState, "basic", "slowzone")
}

// scriptedSlowZoneTarget names a legal placeSlowZone target from
// gameState["enemies"]: enemies are always on a path (see getGameState /
// getPlayerGameState in engine/actions.go), so any enemy's position is
// guaranteed to be in g.PathTileSet, unlike valid_tower_candidates (which
// is off-path by construction and would always be rejected by
// placeSlowZone's path check). Prefers an enemy position not already
// covered by an existing slow zone (gameState["slow_zones"]). Returns
// ok=false, deliberately without guessing a fallback tile, when no enemy is
// visible -- e.g. behind fog of war, or before any enemy has spawned.
func scriptedSlowZoneTarget(gameState map[string]interface{}) (int, int, bool) {
	enemies, _ := gameState["enemies"].([]interface{})
	if len(enemies) == 0 {
		return 0, 0, false
	}
	existing := map[[2]int]struct{}{}
	if zones, ok := gameState["slow_zones"].([][]int); ok {
		for _, z := range zones {
			if len(z) == 2 {
				existing[[2]int{z[0], z[1]}] = struct{}{}
			}
		}
	}
	for _, raw := range enemies {
		enemy, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		y, x, ok := positionFromAny(enemy["position"])
		if !ok {
			continue
		}
		if _, taken := existing[[2]int{y, x}]; taken {
			continue
		}
		return y, x, true
	}
	return 0, 0, false
}

// scriptedDefenderBasicBuffer commits to defender_baseline's build-coverage/
// upgrade/save logic (scriptedDefenderBuild with the "basic" tower, exactly
// as defender_baseline plays it) except for one addition: while
// "place:buffer" is legal AND the defender already owns at least 2 non-buffer
// towers AND a legal placement candidate sits within the buffer's range of at
// least 2 of them, place a buffer there instead -- see scriptedBufferTarget
// for how the position is chosen. Otherwise it falls straight through to
// scriptedDefenderBuild, unchanged.
func scriptedDefenderBasicBuffer(gameState map[string]interface{}) map[string]interface{} {
	affordable, _ := gameState["affordable_actions"].([]string)
	canPlaceBuffer := false
	for _, action := range affordable {
		if action == "place:buffer" {
			canPlaceBuffer = true
			break
		}
	}
	if canPlaceBuffer {
		if y, x, ok := scriptedBufferTarget(gameState); ok {
			return map[string]interface{}{
				"action": "place", "tower_type": "buffer",
				"position": []interface{}{float64(y), float64(x)},
				"reason":   "basic_buffer: place buffer covering existing towers",
			}
		}
	}
	return scriptedDefenderBuild(gameState, "basic", "basic_buffer")
}

// bufferDefaultRange is the buffer tower's range, hardcoded as a last-resort
// fallback because it genuinely is not derivable from a game state that has
// no buffer on the board yet: gameState["tower_costs"] (see getGameState in
// engine/actions.go) carries only g.towerCost's int -- cost, nothing else --
// for every placeable tower type, and no other gameState field exposes the
// stats (range, damage, cooldown) of a tower type that doesn't exist on the
// board yet. The value matches the buffer entry in DefaultBalanceConfig
// (engine/balance.go) and the help text in engine/core.go ("adds +50% damage
// to every OTHER tower within 2 tiles").
const bufferDefaultRange = 2

// scriptedBufferTarget names a legal buffer placement from
// gameState["valid_tower_candidates"] that sits within the buffer's range of
// at least 2 of the defender's existing non-buffer towers (read from
// gameState["towers"]), or returns ok=false when fewer than 2 non-buffer
// towers exist or no candidate reaches 2 of them.
//
// The buffer's range is read off an already-placed buffer tower in
// gameState["towers"] when one exists, rather than assumed, because the
// "range" research line can raise an existing tower's Range after placement
// (see researchTech in engine/actions.go) -- an already-placed buffer's
// current Range is the true number a newly placed buffer would also get.
// Only falls back to bufferDefaultRange when no buffer is on the board yet.
// Distance uses the same Euclidean metric runTowerPhase (engine/actions.go)
// uses to decide the boost, so "covers" here means exactly what it will mean
// once the tower is actually placed.
//
// Candidates are walked in gameState["valid_tower_candidates"] order --
// never a map range, which Go randomizes -- and the first one reaching the
// highest coverage count wins ties, so the same game state always yields the
// same proposed position.
func scriptedBufferTarget(gameState map[string]interface{}) (int, int, bool) {
	towers, _ := gameState["towers"].([]interface{})
	type towerPos struct{ y, x int }
	var nonBuffers []towerPos
	bufferRange := bufferDefaultRange
	haveExistingBufferRange := false
	for _, raw := range towers {
		tower, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		y, x, ok := positionFromAny(tower["position"])
		if !ok {
			continue
		}
		towerType, _ := tower["type"].(string)
		if towerType == "buffer" {
			if !haveExistingBufferRange {
				if r, ok := toIntFromAny(tower["range"]); ok {
					bufferRange = r
					haveExistingBufferRange = true
				}
			}
			continue
		}
		nonBuffers = append(nonBuffers, towerPos{y, x})
	}
	if len(nonBuffers) < 2 {
		return 0, 0, false
	}

	candidates, ok := gameState["valid_tower_candidates"].([][]int)
	if !ok || len(candidates) == 0 {
		return 0, 0, false
	}

	rangeF := float64(bufferRange)
	bestCount := 0
	bestY, bestX := 0, 0
	for _, cand := range candidates {
		if len(cand) != 2 {
			continue
		}
		cy, cx := cand[0], cand[1]
		count := 0
		for _, t := range nonBuffers {
			dy := float64(cy - t.y)
			dx := float64(cx - t.x)
			if math.Sqrt(dy*dy+dx*dx) <= rangeF {
				count++
			}
		}
		if count > bestCount {
			bestCount = count
			bestY, bestX = cy, cx
		}
	}
	if bestCount < 2 {
		return 0, 0, false
	}
	return bestY, bestX, true
}

// positionFromAny parses a [y, x] position out of either the []int shape
// getGameState actually produces (see actions.go) or the []interface{}
// shape a normalized/hand-built decision or gameState might use, so this
// script works against both real engine state and synthetic test state.
func positionFromAny(v interface{}) (int, int, bool) {
	switch pos := v.(type) {
	case []int:
		if len(pos) == 2 {
			return pos[0], pos[1], true
		}
	case []interface{}:
		if len(pos) == 2 {
			y, okY := toIntFromAny(pos[0])
			x, okX := toIntFromAny(pos[1])
			if okY && okX {
				return y, x, true
			}
		}
	}
	return 0, 0, false
}
