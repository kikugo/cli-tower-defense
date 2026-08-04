package engine

import (
	"fmt"
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
	default:
		// Launch a wave once THIS player's own bank reaches the threshold --
		// "hold until rich enough for a big push". gameState["resources"] is
		// keyed over ALL players (see getGameState in engine/actions.go), so
		// scanning it for any entry >= 260 used to launch a wave in response
		// to the opponent's (here, the defender's) balance instead of this
		// player's own. your_resources (added in getPlayerGameState) is this
		// player's own balance, so read that instead.
		if r, ok := toIntFromAny(gameState["your_resources"]); ok && r >= 260 {
			return map[string]interface{}{"action": "wave", "reason": "scripted"}, nil
		}
		return map[string]interface{}{"action": "spawn", "enemy_type": "basic", "reason": "scripted"}, nil
	}
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
