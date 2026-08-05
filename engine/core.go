package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// Global settings
var runWithUI bool = true

// Game entities
type Position struct {
	Y int
	X int
}

type Entity struct {
	Pos       Position
	Char      rune
	Health    int
	MaxHealth int
	Damage    int
	Cooldown  int
	MaxCD     int
}

type Tower struct {
	Entity
	TowerType string
	Range     int
	Cost      int
	Strategy  string
	Level     int
}

func (t *Tower) Upgrade() {
	t.Level++
	switch t.TowerType {
	case "sniper":
		t.Damage = int(float64(t.Damage) * 1.65)
		t.Range = int(float64(t.Range) * 1.15)
		t.MaxCD = int(float64(t.MaxCD) * 0.92)
	case "splash":
		t.Damage = int(float64(t.Damage) * 1.25)
		t.Range = int(float64(t.Range) * 1.1)
		t.MaxCD = int(float64(t.MaxCD) * 0.9)
	case "buffer":
		t.Range++
	default:
		t.Damage = int(float64(t.Damage) * 1.45)
		t.Range = int(float64(t.Range) * 1.2)
		t.MaxCD = int(float64(t.MaxCD) * 0.9)
	}
	if t.MaxCD < 1 {
		t.MaxCD = 1
	}
}

// NewTower builds a tower from default balance numbers. Game code should use
// g.newTower so a tuned Game.Balance applies.
func NewTower(y, x int, towerType string, params map[string]interface{}) Tower {
	g := &Game{Balance: DefaultBalanceConfig()}
	return g.newTower(y, x, towerType, params)
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	default:
		return 0
	}
}

func (t *Tower) CanAttack() bool {
	return t.Cooldown <= 0
}

func (t *Tower) Attack(enemies []*Enemy) []*Enemy {
	if len(enemies) == 0 {
		return nil
	}

	type Target struct {
		distance float64
		enemy    *Enemy
	}

	var targets []Target
	for _, enemy := range enemies {
		if enemy.Health <= 0 {
			// Killed earlier this tick; the spatial index is only rebuilt
			// between phases, so corpses must be skipped here or towers
			// waste shots on them (and kills would be rewarded twice).
			continue
		}
		distance := math.Sqrt(math.Pow(float64(t.Pos.Y-enemy.Pos.Y), 2) + math.Pow(float64(t.Pos.X-enemy.Pos.X), 2))
		if distance <= float64(t.Range) {
			sortKey := distance
			if t.Strategy == "strongest" {
				sortKey = float64(-enemy.Health)
			} else if t.Strategy == "fastest" {
				sortKey = float64(-enemy.Speed)
			}
			targets = append(targets, Target{distance: sortKey, enemy: enemy})
		}
	}

	if len(targets) == 0 {
		return nil
	}

	for i := 0; i < len(targets)-1; i++ {
		for j := i + 1; j < len(targets); j++ {
			if targets[i].distance > targets[j].distance {
				targets[i], targets[j] = targets[j], targets[i]
			}
		}
	}

	var hitEnemies []*Enemy
	if t.TowerType == "splash" {
		limit := 3
		if len(targets) < limit {
			limit = len(targets)
		}
		for i := 0; i < limit; i++ {
			enemy := targets[i].enemy
			damage := t.Damage
			if enemy.Shield > 0 {
				damage /= (enemy.Shield + 1)
			}
			enemy.Health -= damage
			hitEnemies = append(hitEnemies, enemy)
		}
	} else {
		enemy := targets[0].enemy
		damage := t.Damage
		if enemy.Shield > 0 {
			damage /= (enemy.Shield + 1)
		}
		enemy.Health -= damage
		hitEnemies = append(hitEnemies, enemy)
	}

	t.Cooldown = t.MaxCD
	return hitEnemies
}

type Particle struct {
	Pos      Position
	Char     rune
	Lifetime int
	Color    string
}

type Enemy struct {
	Entity
	EnemyType     string
	Speed         float64
	Reward        int
	DistanceMoved float64
	PathIndex     int
	PathID        int
	Shield        int
	// WaveNumber is g.Wave at the moment this enemy was actually placed on
	// the field (spawnEnemy's direct "spawn" action, or the WaveQueue-drain
	// step in UpdateGameState) -- never the wave that queued it. A queued
	// entry that drains after a later spawnWave() call is tagged with the
	// wave in progress at drain time, not the one that enqueued it; that is
	// deliberate, not a bug -- see the per-wave aggregation helpers in
	// telemetry.go, which bucket "sent" the same way. It never changes after
	// creation, so it is a stable key for per-wave aggregation regardless of
	// when the enemy resolves.
	WaveNumber int
}

// NewEnemy builds an enemy from default balance numbers. Game code should use
// g.newEnemy so a tuned Game.Balance applies.
func NewEnemy(y, x int, enemyType string, params map[string]interface{}) Enemy {
	g := &Game{Balance: DefaultBalanceConfig()}
	return g.newEnemy(y, x, enemyType, params)
}

type AIHandler struct {
	Client *http.Client
	rng    *rand.Rand
}

func NewAIHandler(rng *rand.Rand) *AIHandler {
	return &AIHandler{Client: &http.Client{Timeout: 20 * time.Second}, rng: rng}
}

type OpenAIHandler struct {
	*AIHandler
	APIKey string
}

// formatAffordableActions renders the affordable_actions list for prompts.
// When tower placement is affordable, the first legal cells are shown inline
// so models pick a valid position instead of guessing.
func formatAffordableActions(gameState map[string]interface{}) string {
	actions, ok := gameState["affordable_actions"].([]string)
	if !ok || len(actions) == 0 {
		return "save"
	}
	if len(actions) == 1 && actions[0] == "save" {
		return "save (nothing else is affordable this turn)"
	}
	line := strings.Join(actions, ", ")
	hasPlace := false
	for _, action := range actions {
		if strings.HasPrefix(action, "place:") {
			hasPlace = true
			break
		}
	}
	if hasPlace {
		if candidates, ok := gameState["valid_tower_candidates"].([][]int); ok && len(candidates) > 0 {
			limit := 3
			if len(candidates) < limit {
				limit = len(candidates)
			}
			cells := make([]string, 0, limit)
			for _, cell := range candidates[:limit] {
				cells = append(cells, fmt.Sprintf("%v", cell))
			}
			line += "; valid tower cells include " + strings.Join(cells, ", ")
		}
	}
	return line
}

// promptCost reads a per-type cost map out of the game state, falling back to
// the default balance config so prompts never render zeros — and never go
// stale, since the fallback is derived rather than hardcoded.
func promptCost(gameState map[string]interface{}, mapKey, name string) int {
	if costs, ok := gameState[mapKey].(map[string]int); ok {
		if cost, ok := costs[name]; ok {
			return cost
		}
	}
	def := DefaultBalanceConfig()
	if mapKey == "tower_costs" {
		return def.Towers[name].Cost
	}
	return def.Enemies[name].SpawnCost
}

// rejectionFeedbackLine renders a forceful warning when the player's previous
// action(s) were rejected, escalating with the streak so models break out of
// repeat-invalid-action loops. Empty when the last action was applied.
func rejectionFeedbackLine(gameState map[string]interface{}) string {
	streak, _ := toIntFromAny(gameState["your_rejection_streak"])
	if streak <= 0 {
		return ""
	}
	reason, _ := gameState["your_last_rejected_reason"].(string)
	reason = strings.TrimPrefix(reason, "rejected:")
	if reason == "" {
		reason = "invalid action"
	}
	if streak == 1 {
		return fmt.Sprintf("WARNING: your previous action was REJECTED (%s). Do not repeat it; pick from the affordable actions above.\n", reason)
	}
	return fmt.Sprintf("STOP: your last %d actions were ALL REJECTED (%s). You MUST choose a different action or position this turn.\n", streak, reason)
}

func (h *OpenAIHandler) createTowerPrompt(gameState map[string]interface{}) string {
	pathsCount := gameState["paths_count"].(int)
	wave := gameState["wave"].(int)
	stateSummary := summarizePromptState(gameState)
	prompt := fmt.Sprintf(
		"You are the Defender in a Tower Defense Battleground. Goal: Stop enemies from reaching the end.\n"+
			"Current Resources: %v, Base Income: %v, Wave: %d, Paths: %d\n"+
			"You can currently afford ONLY these actions: %s\n"+
			"%s\n"+
			"Current objective: maximize survival and defend lives while keeping future economy viable.\n"+
			"Legal action schema: exactly one JSON object with keys action, reason, taunt and action-specific fields.\n"+
			"Your available tools this turn:\n%s\n"+
			"State summary:\n%s\n\n"+
			"Tower stats (damage / range / cooldown / cost):\n"+
			"- basic  34 / 5 / 2 / 100. Your workhorse: cheap and fires often.\n"+
			"- sniper 50 / 12 / 15 / 250. Long reach and hard hitting, but fires once every 15 ticks.\n"+
			"- splash 10 / 3 / 3 / 200. Low damage, hits groups.\n"+
			"- buffer 0 / 2 / n/a / 300. Deals no damage at all.\n\n"+
			"Strategic Advice:\n"+
			"- A buffer tower adds +50%% damage to every OTHER tower within 2 tiles, and the bonus\n"+
			"  stacks if several buffers overlap. It never attacks, so it is only worth its 300 cost\n"+
			"  when it sits beside two or more damage towers. Placing buffers next to each other\n"+
			"  achieves nothing, because a buffer has no damage to boost.\n"+
			"- Shielded enemies (S) carry shield 2, which divides ALL incoming damage by 3: a sniper\n"+
			"  hit drops from 50 to 16. Volume of fire beats single big hits against them.\n"+
			"- Healer enemies (H) restore health to nearby enemies, so kill them first when both are\n"+
			"  in range.\n"+
			"- Enemy health for reference: basic 100, fast 50, tank 300, shielded 150, healer 80.\n"+
			"- Invest early if you can afford to, but don't let your lives drop too low.\n"+
			"- Only choose an action listed as affordable above. Anything else is rejected and you\n"+
			"  lose the turn.\n"+
			"- If last_rejected_reason is non-empty, change the action AND the position, not just one.\n"+
			"- You may include a taunt for your opponent.\n\n"+
			"Respond with exactly one JSON object only.",
		gameState["resources"], gameState["income"], wave, pathsCount,
		formatAffordableActions(gameState), rejectionFeedbackLine(gameState),
		buildDefenderActionMenu(gameState),
		stateSummary,
	)
	return prompt
}

func (h *OpenAIHandler) parseTowerResponse(response string) (map[string]interface{}, error) {
	if decision, ok := extractDecisionJSON(response); ok {
		action, hasAction := decision["action"].(string)
		if hasAction {
			if action == "place" {
				towerType, hasTowerType := decision["tower_type"].(string)
				if !hasTowerType || towerType == "" {
					decision["tower_type"] = "basic"
					markDecisionSource(decision, SourceParserUnparseable)
				}
				if _, hasPos := decision["position"].([]interface{}); !hasPos {
					decision["position"] = []interface{}{float64(10), float64(10)}
					markDecisionSource(decision, SourceParserUnparseable)
				}
				return decision, nil
			}
			return decision, nil
		}
	}
	fallback := map[string]interface{}{
		"action":     "place",
		"tower_type": "basic",
		"position":   []interface{}{float64(10), float64(10)},
		"reason":     "Default fallback",
	}
	markDecisionSource(fallback, SourceParserUnparseable)
	return fallback, nil
}

type GeminiHandler struct {
	*AIHandler
	APIKey string
}

func (h *GeminiHandler) createEnemyPrompt(gameState map[string]interface{}) string {
	wave := gameState["wave"].(int)
	stateSummary := summarizePromptState(gameState)
	prompt := fmt.Sprintf(
		"You are the Attacker in a Tower Defense Battleground. Goal: Overwhelm the Defender.\n"+
			"Current Resources: %v, Base Income: %v, Wave: %d, Paths: %d\n"+
			"You can currently afford ONLY these actions: %s\n"+
			"%s\n"+
			"Current objective: convert resources into breaches quickly while maintaining wave pressure.\n"+
			"Legal action schema: exactly one JSON object with keys action, reason, taunt and action-specific fields.\n"+
			"Your available tools this turn:\n%s\n"+
			"State summary:\n%s\n\n"+
			"Unit stats (health / speed / spawn cost):\n"+
			"- basic    100 / 1.0 / 20. Cheapest body.\n"+
			"- fast      50 / 2.0 / 30. Fragile, but crosses the map in half the time.\n"+
			"- tank     300 / 0.5 / 50. Six times a basic's health, slowest mover.\n"+
			"- shielded 150 / 0.8 / 40. Shield 2 divides all incoming damage by 3.\n"+
			"- healer    80 / 1.0 / 30. Restores health to nearby enemies.\n\n"+
			"Strategic Advice:\n"+
			"- Shielded units take one third damage from every tower, not just some of them. They\n"+
			"  are the efficient way through concentrated defences: 150 health behind shield 2\n"+
			"  absorbs roughly 450 raw damage for 40 resources.\n"+
			"- A healer behind tanks keeps the tanks alive far longer than either unit alone. Sent\n"+
			"  by itself a healer has nothing to heal and dies quickly.\n"+
			"- Fast units are for slipping past long-cooldown towers such as snipers, which fire\n"+
			"  once every 15 ticks and cannot re-target quickly.\n"+
			"- Sending a wave splits enemies across all %d paths, so it thins any single defence\n"+
			"  but commits your resources at once.\n"+
			"- Only choose an action listed as affordable above. Anything else is rejected and you\n"+
			"  lose the turn, which is the most common way to fall behind.\n"+
			"- If last_rejected_reason is non-empty, pick a genuinely different action, not the same\n"+
			"  one with a different unit.\n"+
			"- You may include a taunt for your opponent.\n\n"+
			"Respond with exactly one JSON object only.",
		gameState["resources"], gameState["income"], wave, gameState["paths_count"],
		formatAffordableActions(gameState), rejectionFeedbackLine(gameState),
		buildAttackerActionMenu(gameState),
		stateSummary, gameState["paths_count"],
	)
	return prompt
}

func (h *GeminiHandler) parseEnemyResponse(response string) (map[string]interface{}, error) {
	if response == "" {
		decision := map[string]interface{}{"action": "spawn", "enemy_type": "basic", "reason": "Empty response"}
		markDecisionSource(decision, SourceParserEmpty)
		return decision, nil
	}
	if decision, ok := extractDecisionJSON(response); ok {
		action, hasAction := decision["action"].(string)
		if hasAction {
			if action == "spawn" {
				enemyType, hasEnemyType := decision["enemy_type"].(string)
				if !hasEnemyType || enemyType == "" {
					decision["enemy_type"] = "basic"
					markDecisionSource(decision, SourceParserUnparseable)
				}
				return decision, nil
			}
			return decision, nil
		}
	}
	fallback := map[string]interface{}{"action": "spawn", "enemy_type": "basic", "reason": "Default fallback"}
	markDecisionSource(fallback, SourceParserUnparseable)
	return fallback, nil
}

type SlowZone struct {
	Pos Position
}

type AbilitySpec struct {
	Name        string `json:"name"`
	Cost        int    `json:"cost"`
	Cooldown    int    `json:"cooldown"`
	Description string `json:"description"`
	CurrentCD   int    `json:"current_cooldown,omitempty"`
}

type Game struct {
	Height          int
	Width           int
	MapHeight       int
	MapWidth        int
	MapType         string
	Paths           [][]Position
	PathTileSet     map[string]struct{}
	EnemyTileIndex  map[string][]*Enemy
	Towers          []*Tower
	Enemies         []*Enemy
	SlowZones       []*SlowZone
	Obstacles       []Position
	ObstacleTileSet map[string]struct{}
	Particles       []*Particle
	Resources       map[string]int
	Income          map[string]int
	Lives           map[string]int
	// StartingLives is the defender's life total at kickoff -- 20 by
	// default (NewGameFromResolvedConfig), overwritten by ApplyRuleset when
	// a ruleset configures StartingLives explicitly. It anchors
	// WaveSummary's LivesStart/LivesEnd ledger at a known, fixed origin
	// instead of reconstructing one from the current Lives value, which is
	// mutable and therefore the wrong thing to derive a timeline from --
	// see buildWaveSummaries in telemetry.go.
	StartingLives      int
	Wave               int
	Score              map[string]int
	LastDecisions      map[string]string
	LastReasoning      map[string]string
	LastTaunt          map[string]string
	WaveQueue          []string
	GameOver           bool
	Winner             string
	AIEnabled          bool
	AIThinking         map[string]bool
	DecisionRouter     *DecisionRouter
	GameSpeed          float64
	AIDecisionInterval map[string]int
	LastAIDecision     map[string]time.Time
	CurrentTurn        string
	LastActionTime     time.Time
	MaxResources       int
	MaxWaves           int
	TurnTimeout        time.Duration
	PauseBetweenTurns  bool
	PauseDuration      time.Duration
	pauseDeadline      time.Time
	lastStatePrintTime time.Time
	lastEnemyCount     int
	lastTowerCount     int
	stateChangeCounter int
	rng                *rand.Rand
	Logs               []string
	MaxLogs            int
	MaxWaveQueue       int
	TickCount          int64
	StartedAt          time.Time
	ReplayEvents       []ReplayEvent
	MaxReplayEvents    int
	ActionCounters     map[string]int
	RejectedActions    map[string]int
	DecisionSources    map[string]int
	// EngineAssists counts how many times applyAdaptivePressure acted on a
	// player's behalf, keyed "playerID:branch" (branch is an AssistBranch
	// string) exactly like DecisionSources' "playerID:source" keys. See
	// recordEngineAssist in assist.go and MatchResult.EngineAssistCounts.
	EngineAssists map[string]int
	// BreachCount counts how many times an enemy reached the end of its
	// path, for the whole match -- not keyed per playerID. A breach is one
	// event the attacker causes and the defender suffers; storing it twice
	// under both playerIDs would just be the same number under two names,
	// inviting a caller to sum a map and double-count it. It increments in
	// the same branch as Lives[g.Defender]--, so it always equals the
	// defender's total lost lives -- see MatchResult.BreachCount.
	BreachCount int
	// AuthoredSaves is the cumulative count of "save" decisions counted by
	// applyDecision's NoopStreak increment -- see the block in
	// applyDecision guarded by
	// `source == SourceModel || source == SourceSkippedForcedSave` with
	// `originalAction == "save"` and `baseOutcome == "applied_primary"`.
	// This field increments in that exact same branch, alongside
	// g.NoopStreak[playerID]++, so the two can never drift apart: a save
	// counts here if and only if it also extended (or started) the current
	// streak. An engine-substituted save does not count, by design -- see
	// MatchResult.AuthoredSaves.
	AuthoredSaves map[string]int
	// DecisionsResolved is the denominator for AuthoredSaves: every call to
	// applyDecision for playerID increments it exactly once, regardless of
	// action, source, or outcome -- the same unconditional count as
	// DecisionSources' total (summed across all its "playerID:source"
	// keys), kept as a dedicated field so MatchResult.AuthoredSaves does
	// not need to iterate DecisionSources to find it.
	DecisionsResolved map[string]int
	// LeakWindow is a rolling window of the last LeakWindowSize enemy
	// resolutions (killed or leaked) across the whole board -- not per
	// player, since an enemy resolves once regardless of who "owns" the
	// outcome. true means that resolution was a breach (leaked through);
	// false means a tower killed it. Oldest entries are evicted from the
	// front once the window is full. See recordResolution and
	// MatchResult.RecentLeaks.
	LeakWindow []bool
	// LeakWindowTotal is the all-time count of resolutions ever recorded
	// into LeakWindow, independent of the window's fixed capacity -- it is
	// what lets a caller tell "fewer than LeakWindowSize enemies have
	// resolved all match" apart from "exactly LeakWindowSize resolved and
	// none leaked". See MatchResult.RecentLeaks.
	LeakWindowTotal int
	// WaveSummaries aggregates per-wave telemetry (sent/leaked/killed counts,
	// a Towers snapshot, a derived LivesStart/LivesEnd pair, and completion)
	// keyed by wave number. Entries are created lazily the first time a
	// wave number is touched by recordWaveEvent, so a wave with zero
	// activity never gets a row. LivesStart/LivesEnd are computed fresh by
	// buildWaveSummaries from the leak ledger, not tracked incrementally --
	// see telemetry.go and MatchResult.WaveSummaries.
	WaveSummaries       map[int]*WaveSummary
	ProviderErrors      map[string]int
	ProviderCalls       map[string]int
	ProviderLatencyMS   map[string]int64
	ProviderTokenUsage  map[string]int
	ProviderCostMicros  map[string]int64
	TokenPricing        map[string]tokenPricing
	Balance             BalanceConfig
	LastActionStatus    map[string]string
	LastRejectedReason  map[string]string
	NoopStreak          map[string]int
	RejectionStreak     map[string]int
	LastRejectedAction  map[string]string
	AutoWaveMinResource int
	AutoDefendMinStreak int
	AssistsDisabled     bool
	// SkipForcedSaveTurns mirrors ArenaRuleset.SkipForcedSaveTurns; see there
	// for what it does and why it defaults to false. Set via ApplyRuleset.
	SkipForcedSaveTurns bool
	FogOfWar            bool
	DefenderVisionRange int
	BaseVisionRange     int
	ResearchLevels      map[string]int
	AbilityCooldowns    map[string]int
	PressureLevel       int
	PressureTriggers    int
	Defender            string
	Attacker            string
	ModelNames          map[string]string
	// ProviderConfigs holds the resolved, non-secret provider configuration
	// (temperature, base URL, timeouts, retries, ...) keyed by playerID, so
	// run manifests can record what actually produced a result. Never holds
	// an API key -- see ProviderConfigRecord.
	ProviderConfigs    map[string]ProviderConfigRecord
	Player1            string
	Player2            string
	pendingTurnResults chan turnResult
	mapInitRecorded    bool
	// ReplayTruncated is true once trimReplayEvents has discarded any real
	// event to respect MaxReplayEvents. See replay.go.
	ReplayTruncated   bool
	winReasonOverride string
}

type turnResult struct {
	playerID string
	role     string
	decision map[string]interface{}
	err      error
	latency  time.Duration
}

func NewGame(openaiKey, googleKey string) *Game {
	defaultConfig := DefaultMatchConfig()
	resolved := ResolvedMatchConfig{
		Player1: ResolvedPlayerModelConfig{
			PlayerModelConfig: normalizePlayerConfig(defaultConfig.Player1),
			APIKey:            openaiKey,
		},
		Player2: ResolvedPlayerModelConfig{
			PlayerModelConfig: normalizePlayerConfig(defaultConfig.Player2),
			APIKey:            googleKey,
		},
	}
	return NewGameFromResolvedConfig(resolved)
}

func NewGameFromEnv() (*Game, error) {
	config, err := LoadMatchConfigFromEnv()
	if err != nil {
		return nil, err
	}
	resolved, err := ResolveMatchConfig(config)
	if err != nil {
		return nil, err
	}
	return NewGameFromResolvedConfig(resolved), nil
}

func NewGameFromResolvedConfig(resolved ResolvedMatchConfig) *Game {
	width := 80
	height := 24
	mapHeight := height - 10
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	p1, p2 := "p1", "p2"
	router := NewDecisionRouter()
	router.SetPlayerProvider(p1, providerFromResolvedConfig(resolved.Player1))
	router.SetPlayerProvider(p2, providerFromResolvedConfig(resolved.Player2))
	game := &Game{
		Height: height, Width: width, MapHeight: mapHeight, MapWidth: width,
		Towers: make([]*Tower, 0), Enemies: make([]*Enemy, 0), SlowZones: make([]*SlowZone, 0), Obstacles: make([]Position, 0), Particles: make([]*Particle, 0),
		Resources: map[string]int{p1: 300, p2: 300}, Income: map[string]int{p1: 5, p2: 5}, Lives: map[string]int{p1: 20, p2: 20}, StartingLives: 20,
		Score: map[string]int{p1: 0, p2: 0}, LastDecisions: map[string]string{p1: "None", p2: "None"},
		LastReasoning: map[string]string{p1: "Thinking...", p2: "Thinking..."}, LastTaunt: map[string]string{p1: "", p2: ""},
		WaveQueue: make([]string, 0), GameOver: false, AIEnabled: true, AIThinking: map[string]bool{p1: false, p2: false},
		Defender: p1, Attacker: p2, ModelNames: map[string]string{p1: resolved.Player1.Model, p2: resolved.Player2.Model},
		ProviderConfigs: map[string]ProviderConfigRecord{p1: newProviderConfigRecord(resolved.Player1), p2: newProviderConfigRecord(resolved.Player2)},
		Player1:         p1, Player2: p2,
		DecisionRouter: router,
		Balance:        DefaultBalanceConfig(),
		GameSpeed:      0.1, AIDecisionInterval: map[string]int{p1: 2, p2: 2},
		LastAIDecision: map[string]time.Time{p1: time.Now(), p2: time.Now()},
		CurrentTurn:    p1, LastActionTime: time.Now(), StartedAt: time.Now(), MaxResources: 800, MaxWaves: 30, TurnTimeout: 45 * time.Second,
		PauseBetweenTurns: true, PauseDuration: 1 * time.Second, lastStatePrintTime: time.Now(), rng: rng, Logs: make([]string, 0), MaxLogs: 250, MaxWaveQueue: 200, ReplayEvents: make([]ReplayEvent, 0), MaxReplayEvents: 10000, ActionCounters: map[string]int{}, RejectedActions: map[string]int{}, DecisionSources: map[string]int{}, EngineAssists: map[string]int{}, AuthoredSaves: map[string]int{p1: 0, p2: 0}, DecisionsResolved: map[string]int{p1: 0, p2: 0}, LeakWindow: make([]bool, 0, LeakWindowSize), WaveSummaries: map[int]*WaveSummary{}, ProviderErrors: map[string]int{}, ProviderCalls: map[string]int{}, ProviderLatencyMS: map[string]int64{}, ProviderTokenUsage: map[string]int{}, ProviderCostMicros: map[string]int64{}, TokenPricing: map[string]tokenPricing{p1: pricingFromConfig(resolved.Player1), p2: pricingFromConfig(resolved.Player2)}, LastActionStatus: map[string]string{p1: "none", p2: "none"}, LastRejectedReason: map[string]string{p1: "", p2: ""}, NoopStreak: map[string]int{p1: 0, p2: 0}, RejectionStreak: map[string]int{p1: 0, p2: 0}, LastRejectedAction: map[string]string{p1: "", p2: ""}, AutoWaveMinResource: 260, AutoDefendMinStreak: 2, FogOfWar: true, DefenderVisionRange: 8, BaseVisionRange: 6, ResearchLevels: map[string]int{"economy": 0, "range": 0, "control": 0}, AbilityCooldowns: map[string]int{"surge": 0, "shield_burst": 0, "reinforce_wave": 0},
		PathTileSet: make(map[string]struct{}), EnemyTileIndex: make(map[string][]*Enemy), ObstacleTileSet: make(map[string]struct{}), pendingTurnResults: make(chan turnResult, 8),
	}
	game.Paths = game.generatePaths()
	game.rebuildPathTileSet()
	game.generateObstacles()
	return game
}

func providerFromResolvedConfig(config ResolvedPlayerModelConfig) DecisionProvider {
	switch config.Provider {
	case ProviderGeminiNative:
		return NewGeminiNativeProvider(config)
	case ProviderScripted:
		return NewScriptedProvider(config)
	default:
		return NewOpenAICompatibleProvider(config)
	}
}

func (g *Game) generatePaths() [][]Position {
	switch g.MapType {
	case "straight":
		return [][]Position{g.generateStraightPath(g.MapHeight / 2)}
	case "forked":
		return [][]Position{g.generateStraightPath(g.MapHeight / 3), g.generateStraightPath(2 * g.MapHeight / 3)}
	case "choke":
		return [][]Position{g.generateChokePath()}
	case "open-field":
		return [][]Position{g.generateStraightPath(g.MapHeight / 3), g.generateStraightPath(g.MapHeight / 2), g.generateStraightPath(2 * g.MapHeight / 3)}
	case "zigzag":
		return [][]Position{g.generateSinglePath(0, 1)}
	case "switchback":
		return [][]Position{g.generateSwitchbackPath()}
	case "perimeter":
		return [][]Position{g.generatePerimeterPath()}
	}
	numPaths := 1
	if g.rng.Float64() > 0.6 {
		numPaths = 2
	}
	paths := make([][]Position, numPaths)
	for i := 0; i < numPaths; i++ {
		paths[i] = g.generateSinglePath(i, numPaths)
	}
	return paths
}

func (g *Game) generateStraightPath(y int) []Position {
	path := make([]Position, 0, g.MapWidth)
	for x := 0; x < g.MapWidth; x++ {
		path = append(path, Position{Y: y, X: x})
	}
	return path
}

func (g *Game) generateChokePath() []Position {
	path := make([]Position, 0, g.MapWidth)
	y := g.MapHeight / 3
	chokeY := g.MapHeight / 2
	for x := 0; x < g.MapWidth; x++ {
		if x > g.MapWidth/3 && x < 2*g.MapWidth/3 {
			y = chokeY
		}
		path = append(path, Position{Y: y, X: x})
	}
	return path
}

func (g *Game) generateSwitchbackPath() []Position {
	path := make([]Position, 0, g.MapWidth)
	y := max(2, g.MapHeight/4)
	direction := 1
	for x := 0; x < g.MapWidth; x++ {
		if x > 0 && x%8 == 0 {
			y += direction * 2
			if y >= g.MapHeight-2 {
				y = g.MapHeight - 3
				direction = -1
			}
			if y <= 1 {
				y = 2
				direction = 1
			}
		}
		path = append(path, Position{Y: y, X: x})
	}
	return path
}

func (g *Game) generatePerimeterPath() []Position {
	path := make([]Position, 0, g.MapWidth)
	top := 1
	bottom := g.MapHeight - 2
	for x := 0; x < g.MapWidth; x++ {
		y := top
		if x > g.MapWidth/3 && x <= (2*g.MapWidth)/3 {
			y = bottom
		}
		if x > (2*g.MapWidth)/3 {
			y = g.MapHeight / 2
		}
		path = append(path, Position{Y: y, X: x})
	}
	return path
}

func (g *Game) SetMapType(mapType string) {
	g.MapType = mapType
	g.Paths = g.generatePaths()
	g.rebuildPathTileSet()
	g.Obstacles = make([]Position, 0)
	g.ObstacleTileSet = make(map[string]struct{})
	g.generateObstacles()
}

func (g *Game) generateSinglePath(index, total int) []Position {
	path := make([]Position, 0)
	centerY := g.MapHeight / 2
	if total > 1 {
		if index == 0 {
			centerY = g.MapHeight / 3
		} else {
			centerY = 2 * g.MapHeight / 3
		}
	}
	x, y := 0, centerY
	path = append(path, Position{Y: y, X: x})
	for x < g.MapWidth-1 {
		move := g.rng.Float64()
		if move < 0.7 || x < 5 || x > g.MapWidth-10 {
			x++
		} else {
			if g.rng.Float64() > 0.5 && y < g.MapHeight-3 {
				y++
			} else if y > 2 {
				y--
			}
			x++
		}
		path = append(path, Position{Y: y, X: x})
	}
	return path
}

func (g *Game) generateObstacles() {
	numObstacles := 5 + g.rng.Intn(10)
	for i := 0; i < numObstacles; i++ {
		obs := Position{Y: 1 + g.rng.Intn(g.MapHeight-2), X: 1 + g.rng.Intn(g.MapWidth-2)}
		_, onPath := g.PathTileSet[tileKey(obs.Y, obs.X)]
		if !onPath {
			g.Obstacles = append(g.Obstacles, obs)
			g.ObstacleTileSet[tileKey(obs.Y, obs.X)] = struct{}{}
		}
	}
}

func (g *Game) rebuildPathTileSet() {
	g.PathTileSet = make(map[string]struct{})
	for _, path := range g.Paths {
		for _, p := range path {
			g.PathTileSet[tileKey(p.Y, p.X)] = struct{}{}
		}
	}
}

func tileKey(y, x int) string {
	return fmt.Sprintf("%d,%d", y, x)
}

func (g *Game) HandleAIDecisions() {
	if g.processPendingTurnResults() {
		// A decision was just applied. Return so the caller can advance the
		// simulation before the next turn is dispatched; dispatching in the
		// same call would keep AIThinking permanently true and starve ticks.
		return
	}

	if !g.AIEnabled || g.GameOver {
		return
	}
	currentTime := time.Now()
	if currentTime.Before(g.pauseDeadline) {
		// Between-turn pause (set by switchTurn) is still outstanding. Return
		// without dispatching the next player's decision; a later call to
		// HandleAIDecisions (the next tick) will re-check the deadline. This
		// keeps the watchability pause without ever blocking the caller, which
		// used to be a synchronous time.Sleep(PauseDuration) right here.
		return
	}
	if currentTime.Sub(g.lastStatePrintTime) > 10*time.Second {
		g.logf("\n=== Game State ===\nWave: %d\nCurrent Turn: %s (%s)\n%s (Def) - Lives: %d, Res: %d\n%s (Att) - Res: %d\nActive Towers: %d, Enemies: %d\n==================\n",
			g.Wave, g.CurrentTurn, g.ModelNames[g.CurrentTurn], g.ModelNames[g.Defender], g.Lives[g.Defender], g.Resources[g.Defender],
			g.ModelNames[g.Attacker], g.Resources[g.Attacker], len(g.Towers), len(g.Enemies))
		g.lastStatePrintTime = currentTime
	}
	if g.AIThinking[g.Player1] || g.AIThinking[g.Player2] {
		return
	}
	if currentTime.Sub(g.LastActionTime) > g.TurnTimeout {
		g.logf("Turn timeout! Switching turn from %s", g.CurrentTurn)
		g.switchTurn()
		return
	}
	player := g.CurrentTurn
	role := "defender"
	if player == g.Attacker {
		role = "attacker"
	}
	if !g.isDecisionIntervalElapsed(player, currentTime) {
		return
	}
	if g.SkipForcedSaveTurns {
		if actions := g.affordableActions(player, role); len(actions) == 1 && actions[0] == "save" {
			// Nothing to decide: "save" is the only legal action, so a real
			// provider call could only ever come back as "save" (scripted
			// providers already take exactly this branch off the same
			// affordable_actions list -- see provider_scripted.go). Skip the
			// round trip, apply the save directly, and still advance the turn
			// exactly as the async path would; see applyForcedSave. Gated by
			// SkipForcedSaveTurns (opt-in, default off): a real provider
			// asked the same question might have proposed something
			// unaffordable instead and gotten rejected, and rejection rate
			// is a recorded discipline metric this default path must keep
			// producing unless a caller explicitly opts into skipping it.
			g.LastAIDecision[player] = currentTime
			g.applyForcedSave(player, role)
			return
		}
	}
	gameState := g.getPlayerGameState(player, role)
	g.handlePlayerTurn(player, role, gameState)
}

// applyForcedSave applies a "save" on playerID's behalf without ever
// dispatching to the provider, for the case where affordableActions is
// exactly {"save"}. It reuses applyDecision with a decision map tagged
// SourceSkippedForcedSave so it goes through the identical game-state path a
// genuine model-authored "save" would (including the auto-defend assist),
// and still switches the turn -- skipping that would deadlock the match,
// since the same player would face the identical all-save situation again
// on the next tick. See HandleAIDecisions and SourceSkippedForcedSave.
func (g *Game) applyForcedSave(playerID, role string) {
	decision := map[string]interface{}{"action": "save"}
	markDecisionSource(decision, SourceSkippedForcedSave)
	g.applyDecision(playerID, role, decision)
	g.switchTurn()
}

func (g *Game) switchTurn() {
	if g.CurrentTurn == g.Player1 {
		g.CurrentTurn = g.Player2
	} else {
		g.CurrentTurn = g.Player1
	}
	g.LastActionTime = time.Now()
	if g.PauseBetweenTurns {
		// Schedule the pause instead of blocking synchronously. switchTurn runs
		// inside HandleAIDecisions, which the Bubble Tea Update loop calls
		// directly (main.go), so a blocking time.Sleep(PauseDuration) here used
		// to stall the whole TUI (input, repaints, everything) for PauseDuration
		// on every turn switch. HandleAIDecisions now checks pauseDeadline on
		// each call and defers dispatch until it has passed, so this returns
		// immediately and the pacing is enforced without blocking.
		g.pauseDeadline = time.Now().Add(g.PauseDuration)
	}
}

func (g *Game) handlePlayerTurn(playerID, role string, gameState map[string]interface{}) {
	g.AIThinking[playerID] = true
	g.LastActionTime = time.Now()
	go func() {
		result := turnResult{playerID: playerID, role: role}
		started := time.Now()
		defer func() {
			if r := recover(); r != nil {
				result.err = fmt.Errorf("%w: %v", errTurnWorkerPanic, r)
			}
			result.latency = time.Since(started)
			g.pendingTurnResults <- result
		}()

		provider, err := g.DecisionRouter.ProviderForPlayer(playerID)
		if err != nil {
			result.err = err
			return
		}

		var decision map[string]interface{}
		if role == "defender" {
			decision, err = provider.GetTowerDecision(gameState)
		} else {
			decision, err = provider.GetEnemyDecision(gameState)
		}
		result.decision = decision
		result.err = err
	}()
}

func (g *Game) applyDecision(playerID, role string, decision map[string]interface{}) {
	// Provenance must be read off the raw decision BEFORE normalizeDecision
	// runs: normalizeDecision builds a brand new map, so a tag stamped by a
	// parser or provider (SourceParserUnparseable, SourceProviderFailure, ...)
	// would otherwise be lost. First writer wins: only adopt the normalizer's
	// own tag when nothing upstream already substituted this decision, so a
	// provider failure is never hidden behind a lesser "normalizer_default".
	source := takeDecisionSource(decision)
	decision = normalizeDecision(role, decision)
	if s := takeDecisionSource(decision); source == SourceModel {
		source = s
	}
	action, _ := decision["action"].(string)
	originalAction := action
	reason, _ := decision["reason"].(string)
	if reason == "" {
		reason = "No reasoning provided."
	}
	g.LastReasoning[playerID] = reason
	taunt, _ := decision["taunt"].(string)
	if taunt != "" {
		g.LastTaunt[playerID] = taunt
		g.logf("%s: %s", g.ModelNames[playerID], taunt)
	}
	modelName := g.ModelNames[playerID]
	g.logf("%s (%s) decided to: %s", modelName, role, action)
	g.DecisionSources[playerID+":"+string(source)]++
	// DecisionsResolved is AuthoredSaves' denominator: unconditional, once
	// per applyDecision call, mirroring what summing every DecisionSources
	// key for playerID would already give -- kept as its own counter so
	// MatchResult.AuthoredSaves does not need to iterate DecisionSources.
	g.DecisionsResolved[playerID]++
	g.recordReplayEvent(ReplayEvent{
		Type:     ReplayDecision,
		PlayerID: playerID,
		Role:     role,
		Action:   action,
		Reason:   reason,
		Details:  map[string]interface{}{"decision": decision, "source": string(source)},
	})
	applied := false
	outcome := "rejected"
	entityType := ""
	if role == "defender" {
		if action == "place" {
			towerType, _ := decision["tower_type"].(string)
			entityType = towerType
			y, x := parseDecisionPosition(decision["position"], 2, 2)
			if g.placeTower(y, x, towerType) {
				g.LastDecisions[playerID] = fmt.Sprintf("Placed %s tower at [%d,%d]", towerType, y, x)
				applied = true
				outcome = "applied_primary"
			} else if fy, fx, ok := g.findNearestTowerPlacement(y, x, 5); ok && g.placeTower(fy, fx, towerType) {
				g.LastDecisions[playerID] = fmt.Sprintf("Placed %s tower at fallback [%d,%d]", towerType, fy, fx)
				g.logf("%s fallback placed %s tower at [%d,%d] after invalid target [%d,%d]", modelName, towerType, fy, fx, y, x)
				applied = true
				outcome = "applied_fallback"
			} else {
				_, reason := g.canPlaceTowerAt(y, x)
				g.logf("%s defender place rejected at [%d,%d]: %s", modelName, y, x, reason)
				outcome = "rejected:" + reason
			}
		} else if action == "upgrade" {
			towerID := -1
			if id, ok := toIntFromAny(decision["tower_id"]); ok {
				towerID = id
			}
			if g.upgradeTower(towerID) {
				g.LastDecisions[playerID] = fmt.Sprintf("Upgraded tower #%d", towerID)
				applied = true
				outcome = "applied_primary"
			} else {
				outcome = "rejected:invalid_or_unaffordable_upgrade"
			}
		} else if action == "place_slow_zone" {
			y, x := parseDecisionPosition(decision["position"], -1, -1)
			if g.placeSlowZone(y, x) {
				g.LastDecisions[playerID] = fmt.Sprintf("Placed slow zone at [%d,%d]", y, x)
				applied = true
				outcome = "applied_primary"
			} else {
				outcome = "rejected:invalid_or_unaffordable_slow_zone"
			}
		} else if action == "research" {
			tech, _ := decision["tech"].(string)
			if g.researchTech(tech) {
				g.LastDecisions[playerID] = fmt.Sprintf("Researched %s", tech)
				applied = true
				outcome = "applied_primary"
			} else {
				outcome = "rejected:invalid_or_unaffordable_research"
			}
		} else if action == "invest" {
			if g.invest(playerID) {
				g.LastDecisions[playerID] = "Invested in economy"
				applied = true
				outcome = "applied_primary"
			} else {
				outcome = "rejected:unaffordable_invest"
			}
		} else {
			if g.shouldAutoDefendAfterSave(playerID) {
				if y, x, ok := g.findNearestTowerPlacement(g.MapHeight/2, g.MapWidth/3, 10); ok && g.placeTower(y, x, "basic") {
					g.LastDecisions[playerID] = fmt.Sprintf("Placed basic tower after repeated saves at [%d,%d]", y, x)
					action = "place"
					entityType = "basic"
					applied = true
					outcome = "applied_auto_defense"
				}
			}
			if !applied {
				g.LastDecisions[playerID] = "Saving resources"
				applied = true
				outcome = "applied_primary"
			}
		}
	} else {
		autoWaveLaunched := false
		if (action == "spawn" || action == "save") && g.shouldAutoLaunchWave(playerID) {
			if g.spawnWave(false) {
				g.LastDecisions[playerID] = "Launched wave (auto)"
				action = "wave"
				applied = true
				outcome = "applied_auto_wave"
				autoWaveLaunched = true
			}
		}
		if action == "spawn" {
			enemyType, _ := decision["enemy_type"].(string)
			entityType = enemyType
			if g.spawnEnemy(enemyType, nil) {
				g.LastDecisions[playerID] = fmt.Sprintf("Spawned %s enemy", enemyType)
				applied = true
				outcome = "applied_primary"
			} else {
				outcome = "rejected:invalid_or_unaffordable_spawn"
			}
		} else if action == "wave" {
			if autoWaveLaunched {
				// Auto-wave already consumed the wave action for this turn.
			} else if g.spawnWave(false) {
				g.LastDecisions[playerID] = "Launched wave"
				applied = true
				outcome = "applied_primary"
			} else {
				outcome = "rejected:unaffordable_wave"
			}
		} else if action == "ability" {
			ability, _ := decision["ability"].(string)
			if g.useAttackerAbility(ability) {
				g.LastDecisions[playerID] = fmt.Sprintf("Used %s ability", ability)
				applied = true
				outcome = "applied_primary"
			} else {
				outcome = "rejected:invalid_or_unavailable_ability"
			}
		} else if action == "invest" {
			if g.invest(playerID) {
				g.LastDecisions[playerID] = "Invested in economy"
				applied = true
				outcome = "applied_primary"
			} else {
				outcome = "rejected:unaffordable_invest"
			}
		} else {
			g.LastDecisions[playerID] = "Saving resources"
			applied = true
			outcome = "applied_primary"
		}
	}
	// baseOutcome is what actually happened in the game (applied/rejected and
	// why); outcome is what gets recorded and shown to the model. A decision
	// the engine substituted must never be indistinguishable from one the
	// model made, so a non-model source is folded into the recorded outcome
	// here -- after this point "applied_primary" can only mean the model's
	// own decision was applied as-is. SourceSkippedForcedSave is deliberately
	// excluded from this: it is not a substitution (the engine never invented
	// a value in place of something the model failed to supply), it is a
	// decision the model was never asked for in the first place, so tagging
	// it "substituted:" would be as wrong as tagging it "applied_primary"
	// unqualified -- see SourceSkippedForcedSave.
	baseOutcome := outcome
	if source != SourceModel && source != SourceSkippedForcedSave {
		outcome = "substituted:" + string(source) + ":" + outcome
	}
	g.ActionCounters[actionCounterKey(playerID, action, entityType)]++
	if applied {
		g.RejectionStreak[playerID] = 0
	} else {
		g.RejectionStreak[playerID]++
		g.LastRejectedAction[playerID] = action
		g.RejectedActions[playerID+":"+action]++
		g.LastRejectedReason[playerID] = outcome
		g.logf("%s (%s) action rejected: %s", modelName, action, outcome)
		g.recordReplayEvent(ReplayEvent{
			Type:     ReplayRejected,
			PlayerID: playerID,
			Role:     role,
			Action:   action,
			Reason:   outcome,
		})
	}
	g.recordReplayEvent(ReplayEvent{
		Type:     ReplayOutcome,
		PlayerID: playerID,
		Role:     role,
		Action:   action,
		Reason:   outcome,
		Details: map[string]interface{}{
			"quality": classifyActionOutcome(outcome),
		},
	})
	// NoopStreak only tracks a model genuinely choosing to save, or a save
	// forced because nothing else was legal (SourceSkippedForcedSave): both
	// are a real player-turn that resolved to "save" in the game. A
	// substituted "save" (e.g. a provider failure normalized to save) must
	// still not trip shouldAutoDefendAfterSave on the model's behalf --
	// that would mean the assist fires because the engine papered over a
	// failure, not because the player (model or forced) actually saved.
	if applied && (source == SourceModel || source == SourceSkippedForcedSave) && originalAction == "save" && baseOutcome == "applied_primary" {
		g.NoopStreak[playerID]++
		// AuthoredSaves counts exactly what NoopStreak counts, in the same
		// branch, so the two can never disagree -- see MatchResult.AuthoredSaves
		// and the field doc on Game.AuthoredSaves for why that matters.
		g.AuthoredSaves[playerID]++
	} else if applied {
		g.NoopStreak[playerID] = 0
	}
	g.LastActionStatus[playerID] = outcome
}

// actionCounterKey builds the ActionCounters map key. entityType (the
// tower/enemy type for "place"/"spawn") is appended as a third segment so
// per-content usage can be read directly off a match result instead of
// mined from ReplayEvents; when entityType is empty (every other action)
// the key stays "playerID:action" so existing consumers are unaffected.
func actionCounterKey(playerID, action, entityType string) string {
	key := playerID + ":" + action
	if entityType != "" {
		key += ":" + entityType
	}
	return key
}

func classifyActionOutcome(outcome string) string {
	switch {
	// Checked first: a substituted decision must never classify as
	// "primary", no matter what it was substituted for downstream.
	case strings.HasPrefix(outcome, "substituted:"):
		return "substituted"
	case strings.HasPrefix(outcome, "applied_primary"):
		return "primary"
	case strings.HasPrefix(outcome, "applied_fallback"):
		return "fallback"
	case strings.HasPrefix(outcome, "applied_auto_"):
		return "auto_corrected"
	case strings.HasPrefix(outcome, "rejected"):
		return "rejected"
	default:
		return "unknown"
	}
}

func (g *Game) logf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	g.Logs = append(g.Logs, msg)
	if g.MaxLogs > 0 && len(g.Logs) > g.MaxLogs {
		g.Logs = g.Logs[len(g.Logs)-g.MaxLogs:]
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (g *Game) isDecisionIntervalElapsed(playerID string, now time.Time) bool {
	intervalSecs := g.AIDecisionInterval[playerID]
	if intervalSecs <= 0 {
		return true
	}
	lastDecision, ok := g.LastAIDecision[playerID]
	if !ok {
		return true
	}
	return now.Sub(lastDecision) >= time.Duration(intervalSecs)*time.Second
}

func (g *Game) processPendingTurnResults() bool {
	processed := false
	for {
		select {
		case result := <-g.pendingTurnResults:
			processed = true
			g.AIThinking[result.playerID] = false
			g.LastAIDecision[result.playerID] = time.Now()
			g.ProviderCalls[result.playerID]++
			g.ProviderLatencyMS[result.playerID] += result.latency.Milliseconds()
			if usage, ok := takeTokenUsage(result.decision); ok {
				g.ProviderTokenUsage[result.playerID] += usage.Total
				if micros := g.tokenCostMicros(result.playerID, usage); micros > 0 {
					g.ProviderCostMicros[result.playerID] += micros
				}
			}
			if g.GameOver {
				continue
			}
			if result.err != nil {
				// A provider error means no decision was produced at all --
				// the turn is skipped, not substituted, per AUDIT-FOLLOWUP.md
				// Task 1. Still record provenance: this was a decision
				// *attempt* that failed, and it must count in the denominator
				// of ModelAuthored so a run of pure provider failures reads
				// as "0% authored", not "not measured".
				source := takeDecisionSource(result.decision)
				if source == SourceModel {
					// The provider didn't tag its own fallback (or there was
					// no decision at all, e.g. a turn-worker panic) -- still
					// an engine substitution, never the model's doing.
					source = SourceProviderFailure
				}
				g.DecisionSources[result.playerID+":"+string(source)]++
				g.ProviderErrors[result.playerID+":"+providerErrorLabel(result.err)]++
				g.recordReplayEvent(ReplayEvent{
					Type:     ReplayProviderErr,
					PlayerID: result.playerID,
					Role:     result.role,
					Reason:   providerErrorLabel(result.err),
					Details:  map[string]interface{}{"error": result.err.Error(), "source": string(source)},
				})
				g.logf("%s API error: %v", g.ModelNames[result.playerID], result.err)
				g.switchTurn()
				continue
			}
			g.applyDecision(result.playerID, result.role, result.decision)
			g.switchTurn()
		default:
			return processed
		}
	}
}

func (g *Game) SetRandomSeed(seed int64) {
	g.rng = rand.New(rand.NewSource(seed))
	g.Paths = g.generatePaths()
	g.rebuildPathTileSet()
	g.Obstacles = make([]Position, 0)
	g.ObstacleTileSet = make(map[string]struct{})
	g.generateObstacles()
}

var errTurnWorkerPanic = errors.New("turn worker panic")

func extractOpenAIChatContent(result map[string]interface{}) (string, bool) {
	choicesRaw, ok := result["choices"]
	if !ok {
		return "", false
	}
	choices, ok := choicesRaw.([]interface{})
	if !ok || len(choices) == 0 {
		return "", false
	}
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", false
	}
	messageRaw, ok := choice["message"]
	if !ok {
		return "", false
	}
	message, ok := messageRaw.(map[string]interface{})
	if !ok {
		return "", false
	}
	contentRaw, ok := message["content"]
	if !ok {
		return "", false
	}
	content, ok := contentRaw.(string)
	return content, ok
}

func extractGeminiContentText(result map[string]interface{}) (string, bool) {
	candidatesRaw, ok := result["candidates"]
	if !ok {
		return "", false
	}
	candidates, ok := candidatesRaw.([]interface{})
	if !ok || len(candidates) == 0 {
		return "", false
	}
	candidate, ok := candidates[0].(map[string]interface{})
	if !ok {
		return "", false
	}
	contentRaw, ok := candidate["content"]
	if !ok {
		return "", false
	}
	content, ok := contentRaw.(map[string]interface{})
	if !ok {
		return "", false
	}
	partsRaw, ok := content["parts"]
	if !ok {
		return "", false
	}
	parts, ok := partsRaw.([]interface{})
	if !ok || len(parts) == 0 {
		return "", false
	}
	part, ok := parts[0].(map[string]interface{})
	if !ok {
		return "", false
	}
	textRaw, ok := part["text"]
	if !ok {
		return "", false
	}
	text, ok := textRaw.(string)
	return text, ok
}

// extractGeminiFinishReason reads candidates[0].finishReason from a Gemini
// generateContent response -- e.g. "STOP", "MAX_TOKENS", "SAFETY". Providers
// use this to detect a completion truncated by the token budget rather than
// a genuine (if malformed) model response; see SourceParserFallbackTruncated.
func extractGeminiFinishReason(result map[string]interface{}) (string, bool) {
	candidatesRaw, ok := result["candidates"]
	if !ok {
		return "", false
	}
	candidates, ok := candidatesRaw.([]interface{})
	if !ok || len(candidates) == 0 {
		return "", false
	}
	candidate, ok := candidates[0].(map[string]interface{})
	if !ok {
		return "", false
	}
	reasonRaw, ok := candidate["finishReason"]
	if !ok {
		return "", false
	}
	reason, ok := reasonRaw.(string)
	return reason, ok
}

func toIntFromAny(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int32:
		return int(val), true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	case float32:
		return int(val), true
	case json.Number:
		i, err := val.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

// tokenUsage captures per-call token accounting returned by a provider.
type tokenUsage struct {
	Prompt     int
	Completion int
	Total      int
}

// tokenUsageKey is a reserved decision-map key used to carry provider token
// usage from the async turn worker back to the game loop without changing the
// DecisionProvider interface. It is stripped before a decision is applied.
const tokenUsageKey = "_token_usage"

func (u tokenUsage) normalized() tokenUsage {
	if u.Total == 0 {
		u.Total = u.Prompt + u.Completion
	}
	return u
}

func (u tokenUsage) empty() bool {
	return u.Prompt == 0 && u.Completion == 0 && u.Total == 0
}

// attachTokenUsage stashes usage on the decision map so the game loop can
// record it. Empty usage is not attached.
func attachTokenUsage(decision map[string]interface{}, u tokenUsage) {
	if decision == nil {
		return
	}
	u = u.normalized()
	if u.empty() {
		return
	}
	decision[tokenUsageKey] = u
}

// takeTokenUsage removes and returns any usage stashed on a decision map.
func takeTokenUsage(decision map[string]interface{}) (tokenUsage, bool) {
	if decision == nil {
		return tokenUsage{}, false
	}
	raw, ok := decision[tokenUsageKey]
	if !ok {
		return tokenUsage{}, false
	}
	delete(decision, tokenUsageKey)
	u, ok := raw.(tokenUsage)
	return u, ok
}

// extractOpenAIUsage reads the `usage` object from an OpenAI-compatible
// chat completion response.
func extractOpenAIUsage(result map[string]interface{}) (tokenUsage, bool) {
	usageRaw, ok := result["usage"].(map[string]interface{})
	if !ok {
		return tokenUsage{}, false
	}
	prompt, _ := toIntFromAny(usageRaw["prompt_tokens"])
	completion, _ := toIntFromAny(usageRaw["completion_tokens"])
	total, _ := toIntFromAny(usageRaw["total_tokens"])
	u := tokenUsage{Prompt: prompt, Completion: completion, Total: total}.normalized()
	return u, !u.empty()
}

// tokenPricing holds USD pricing per one million tokens for a player.
type tokenPricing struct {
	InputPerMillion  float64
	OutputPerMillion float64
}

func pricingFromConfig(cfg ResolvedPlayerModelConfig) tokenPricing {
	return tokenPricing{
		InputPerMillion:  cfg.PriceInputPerMillion,
		OutputPerMillion: cfg.PriceOutputPerMillion,
	}
}

// tokenCostMicros estimates the cost of a call in USD millionths (micros)
// using the player's configured pricing. Returns 0 when no pricing is set.
func (g *Game) tokenCostMicros(playerID string, u tokenUsage) int64 {
	p, ok := g.TokenPricing[playerID]
	if !ok || (p.InputPerMillion == 0 && p.OutputPerMillion == 0) {
		return 0
	}
	prompt, completion := u.Prompt, u.Completion
	if prompt == 0 && completion == 0 {
		completion = u.Total
	}
	micros := float64(prompt)*p.InputPerMillion + float64(completion)*p.OutputPerMillion
	if micros <= 0 {
		return 0
	}
	return int64(micros + 0.5)
}

// extractGeminiUsage reads the `usageMetadata` object from a Gemini
// generateContent response.
func extractGeminiUsage(result map[string]interface{}) (tokenUsage, bool) {
	usageRaw, ok := result["usageMetadata"].(map[string]interface{})
	if !ok {
		return tokenUsage{}, false
	}
	prompt, _ := toIntFromAny(usageRaw["promptTokenCount"])
	completion, _ := toIntFromAny(usageRaw["candidatesTokenCount"])
	total, _ := toIntFromAny(usageRaw["totalTokenCount"])
	u := tokenUsage{Prompt: prompt, Completion: completion, Total: total}.normalized()
	return u, !u.empty()
}

func parseDecisionPosition(raw interface{}, defaultY, defaultX int) (int, int) {
	pos, ok := raw.([]interface{})
	if !ok || len(pos) < 2 {
		return defaultY, defaultX
	}
	y, okY := toIntFromAny(pos[0])
	x, okX := toIntFromAny(pos[1])
	if !okY || !okX {
		return defaultY, defaultX
	}
	return y, x
}

func summarizePromptState(gameState map[string]interface{}) string {
	keys := []string{"active_enemies", "wave_queue", "paths_count", "resources", "income", "lives"}
	lines := make([]string, 0, len(keys)+2)
	for _, k := range keys {
		if v, ok := gameState[k]; ok {
			lines = append(lines, fmt.Sprintf("- %s: %v", k, v))
		}
	}
	if towers, ok := gameState["towers"].([]interface{}); ok {
		lines = append(lines, fmt.Sprintf("- towers_count: %d", len(towers)))
	}
	if enemies, ok := gameState["enemies"].([]interface{}); ok {
		lines = append(lines, fmt.Sprintf("- enemies_count: %d", len(enemies)))
	}
	if candidates, ok := gameState["valid_tower_candidates"]; ok {
		lines = append(lines, fmt.Sprintf("- valid_tower_candidates: %v", candidates))
	}
	if pressure, ok := gameState["pressure"]; ok {
		lines = append(lines, fmt.Sprintf("- pressure: %v", pressure))
	}
	if visibility, ok := gameState["visibility"]; ok {
		lines = append(lines, fmt.Sprintf("- visibility: %v", visibility))
	}
	if research, ok := gameState["research"]; ok {
		lines = append(lines, fmt.Sprintf("- research: %v", research))
	}
	if abilities, ok := gameState["attacker_abilities"]; ok {
		lines = append(lines, fmt.Sprintf("- attacker_abilities: %v", abilities))
	}
	if director, ok := gameState["director"]; ok {
		lines = append(lines, fmt.Sprintf("- director: %v", director))
	}
	if rejected, ok := gameState["last_rejected_reason"]; ok {
		lines = append(lines, fmt.Sprintf("- last_rejected_reason: %v", rejected))
	}
	return strings.Join(lines, "\n")
}

func (g *Game) TotalProviderErrorsForPlayer(playerID string) int {
	total := 0
	for key, count := range g.ProviderErrors {
		if len(key) >= len(playerID)+1 && key[:len(playerID)+1] == playerID+":" {
			total += count
		}
	}
	return total
}

func (g *Game) TotalRejectedActionsForPlayer(playerID string) int {
	total := 0
	for key, count := range g.RejectedActions {
		if len(key) >= len(playerID)+1 && key[:len(playerID)+1] == playerID+":" {
			total += count
		}
	}
	return total
}

func (g *Game) shouldAutoLaunchWave(playerID string) bool {
	if g.AssistsDisabled {
		return false
	}
	minResources := g.AutoWaveMinResource
	if minResources <= 0 {
		minResources = 260
	}
	if g.NoopStreak[playerID] >= g.AutoDefendMinStreak {
		minResources = 160
	}
	if g.Resources[playerID] < minResources {
		return false
	}
	if len(g.WaveQueue) > 3 || g.Wave >= g.MaxWaves {
		return false
	}
	return true
}

func (g *Game) shouldAutoDefendAfterSave(playerID string) bool {
	if g.AssistsDisabled {
		return false
	}
	minStreak := g.AutoDefendMinStreak
	if minStreak <= 0 {
		minStreak = 2
	}
	return g.NoopStreak[playerID] >= minStreak && g.Resources[playerID] >= 100 && len(g.Towers) < 5
}
