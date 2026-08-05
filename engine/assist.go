package engine

// AssistBranch identifies which branch of applyAdaptivePressure the engine
// took on a player's behalf. These are stable strings -- they are used as
// map keys (g.EngineAssists / MatchResult.EngineAssistCounts) and as
// ReplayEngineAssist's Reason/Details["branch"] values, so a consumer can
// depend on them not changing across engine versions the way a free-text
// description could.
type AssistBranch string

const (
	// AssistReinforceWave is applyAdaptivePressure's branch 1: it calls
	// g.useAttackerAbility("reinforce_wave") directly, bypassing
	// applyDecision entirely -- so absent this instrumentation it never
	// reached g.DecisionSources or g.ActionCounters at all.
	AssistReinforceWave AssistBranch = "reinforce_wave"
	// AssistAutoWave is applyAdaptivePressure's branch 2: it calls
	// g.spawnWave() directly, which emits a ReplayWave event attributed to
	// the attacker that -- before this instrumentation -- was
	// indistinguishable from a wave the model itself chose. See spawnWave's
	// assisted parameter and ReplayEvent.EngineAssisted.
	//
	// This is a different code path from shouldAutoLaunchWave's
	// "applied_auto_wave" (core.go), which is invoked from inside
	// applyDecision and already recorded via ReplayOutcome. Both ultimately
	// call spawnWave, but only applyAdaptivePressure's call needed new
	// instrumentation -- shouldAutoLaunchWave's was deliberately left alone
	// per the task brief.
	AssistAutoWave AssistBranch = "auto_wave"
	// AssistQueueEnemy is applyAdaptivePressure's branch 3: it appends
	// "basic" straight onto g.WaveQueue with no game-state call at all, so
	// absent this instrumentation nothing -- no event, no counter -- ever
	// recorded it happening.
	AssistQueueEnemy AssistBranch = "queue_enemy"
)

// recordEngineAssist records that the engine, not playerID's model, took an
// action -- one of applyAdaptivePressure's three branches. It increments
// g.EngineAssists[playerID+":"+branch] (surfaced on MatchResult as
// EngineAssistCounts / EngineAssistTotal) and emits a ReplayEngineAssist
// event carrying enough detail for a one-line move-feed description.
//
// This is deliberately a separate counter and event stream from
// g.DecisionSources / markDecisionSource: those record who authored a
// decision the model WAS asked to make (a genuine model response vs. an
// engine substitution standing in for a failed one). This records the
// engine acting instead of ever asking -- folding it into DecisionSources
// would misattribute an engine-initiated action to playerID's turn, which
// is exactly the misattribution this task exists to fix. See
// ARENA-AUDIT.md and applyAdaptivePressure in actions.go.
//
// applyAdaptivePressure only ever acts on the attacker's behalf (it exists
// to keep pressure on the defender when the attacker goes quiet), so Role
// is always "attacker" here -- there is no defender-side equivalent to
// disambiguate.
func (g *Game) recordEngineAssist(playerID string, branch AssistBranch, details map[string]interface{}) {
	if g == nil {
		return
	}
	if g.EngineAssists == nil {
		g.EngineAssists = map[string]int{}
	}
	g.EngineAssists[playerID+":"+string(branch)]++
	if details == nil {
		details = map[string]interface{}{}
	}
	details["branch"] = string(branch)
	g.recordReplayEvent(ReplayEvent{
		Type:     ReplayEngineAssist,
		PlayerID: playerID,
		Role:     "attacker",
		Action:   "assist_" + string(branch),
		Reason:   string(branch),
		Details:  details,
	})
}
