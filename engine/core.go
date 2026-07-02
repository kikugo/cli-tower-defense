package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"regexp"
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

func (h *OpenAIHandler) GetTowerDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	prompt := h.createTowerPrompt(gameState)
	reqBody := map[string]interface{}{
		"model": "o3",
		"messages": []map[string]interface{}{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
		"max_tokens":  300,
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai returned status %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	content, ok := extractOpenAIChatContent(result)
	if !ok {
		return map[string]interface{}{"action": "none", "reason": "API response error"}, nil
	}
	return h.parseTowerResponse(content)
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

// promptCost reads a per-type cost map out of the game state with a fallback
// so prompts never render zeros if a key is missing.
func promptCost(gameState map[string]interface{}, mapKey, name string, fallback int) int {
	costs, ok := gameState[mapKey].(map[string]int)
	if !ok {
		return fallback
	}
	if cost, ok := costs[name]; ok {
		return cost
	}
	return fallback
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
			"Your available tools this turn:\n"+
			"Actions:\n"+
			"1. {\"action\": \"place\", \"tower_type\": \"basic|sniper|splash|buffer\", \"position\": [y, x], \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"   Costs: basic (%d), splash (%d), sniper (%d), buffer (%d)\n"+
			"   Rules: Position must be inside map, not on path, not on obstacle, not on another tower.\n"+
			"2. {\"action\": \"upgrade\", \"tower_id\": <int>, \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"   Cost: 150 * (current_level + 1). Increases damage and range.\n"+
			"   Rules: tower_id must exist.\n"+
			"3. {\"action\": \"place_slow_zone\", \"position\": [y, x], \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"   Cost: 150. Reduces enemy speed by 50%%. MUST be on a path.\n"+
			"4. {\"action\": \"research\", \"tech\": \"economy|range|control\", \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"   Cost: economy (180), range (160), control (140). Unlocks persistent defender bonuses.\n"+
			"5. {\"action\": \"invest\", \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"   Cost: 150. Permanently increases passive income.\n"+
			"6. {\"action\": \"save\", \"reason\": \"...\", \"taunt\": \"...\"}\n\n"+
			"State summary:\n%s\n\n"+
			"Strategic Advice:\n"+
			"- Buffer towers (B) increase damage of nearby towers by 50%%. Place them in clusters.\n"+
			"- Watch out for Healer enemies (H) and Shielded enemies (S).\n"+
			"- Invest early if you can afford to, but don't let your lives drop too low.\n"+
			"- If last_rejected_reason is non-empty, avoid repeating the same invalid action pattern.\n"+
			"- You can send a taunt message to your opponent!\n\n"+
			"Respond with exactly one JSON object only.",
		gameState["resources"], gameState["income"], wave, pathsCount,
		formatAffordableActions(gameState), rejectionFeedbackLine(gameState),
		promptCost(gameState, "tower_costs", "basic", 100),
		promptCost(gameState, "tower_costs", "splash", 200),
		promptCost(gameState, "tower_costs", "sniper", 250),
		promptCost(gameState, "tower_costs", "buffer", 300),
		stateSummary,
	)
	return prompt
}

func (h *OpenAIHandler) parseTowerResponse(response string) (map[string]interface{}, error) {
	re := regexp.MustCompile(`\{.*\}`)
	match := re.FindString(response)
	if match != "" {
		var decision map[string]interface{}
		if err := json.Unmarshal([]byte(match), &decision); err == nil {
			action, hasAction := decision["action"].(string)
			if hasAction {
				if action == "place" {
					towerType, hasTowerType := decision["tower_type"].(string)
					if !hasTowerType || towerType == "" {
						decision["tower_type"] = "basic"
					}
					if _, hasPos := decision["position"].([]interface{}); !hasPos {
						decision["position"] = []interface{}{float64(10), float64(10)}
					}
					return decision, nil
				}
				return decision, nil
			}
		}
	}
	return map[string]interface{}{
		"action":     "place",
		"tower_type": "basic",
		"position":   []interface{}{float64(10), float64(10)},
		"reason":     "Default fallback",
	}, nil
}

type GeminiHandler struct {
	*AIHandler
	APIKey string
}

func (h *GeminiHandler) GetEnemyDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	prompt := h.createEnemyPrompt(gameState)
	reqBody := map[string]interface{}{
		"contents":         []map[string]interface{}{{"parts": []map[string]interface{}{{"text": prompt}}}},
		"generationConfig": map[string]interface{}{"temperature": 0.7, "maxOutputTokens": 150},
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return getFallbackEnemyDecision(100), nil
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=%s", h.APIKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqJSON))
	if err != nil {
		return getFallbackEnemyDecision(100), nil
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.Client.Do(req)
	if err != nil {
		return getFallbackEnemyDecision(100), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return getFallbackEnemyDecision(100), nil
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return getFallbackEnemyDecision(100), nil
	}
	text, ok := extractGeminiContentText(result)
	if !ok {
		return getFallbackEnemyDecision(100), nil
	}
	return h.parseEnemyResponse(text)
}

func (h *GeminiHandler) createEnemyPrompt(gameState map[string]interface{}) string {
	wave := gameState["wave"].(int)
	waveCost := waveCostForWave(wave)
	stateSummary := summarizePromptState(gameState)
	prompt := fmt.Sprintf(
		"You are the Attacker in a Tower Defense Battleground. Goal: Overwhelm the Defender.\n"+
			"Current Resources: %v, Base Income: %v, Wave: %d, Paths: %d\n"+
			"You can currently afford ONLY these actions: %s\n"+
			"%s\n"+
			"Current objective: convert resources into breaches quickly while maintaining wave pressure.\n"+
			"Legal action schema: exactly one JSON object with keys action, reason, taunt and action-specific fields.\n"+
			"Your available tools this turn:\n"+
			"Enemy Options (cost):\n"+
			"- basic (%d): Standard unit\n"+
			"- fast (%d): Quick and nimble\n"+
			"- tank (%d): High durability\n"+
			"- shielded (%d): Takes 50%% less damage from all towers\n"+
			"- healer (%d): Heals nearby enemies in an area\n"+
			"- wave (%d): Massive multi-path assault\n\n"+
			"Actions:\n"+
			"1. {\"action\": \"spawn\", \"enemy_type\": \"basic|fast|tank|shielded|healer\", \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"2. {\"action\": \"wave\", \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"3. {\"action\": \"ability\", \"ability\": \"surge|shield_burst|reinforce_wave\", \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"   Costs/cooldowns: surge (80/12), shield_burst (90/14), reinforce_wave (70/10).\n"+
			"4. {\"action\": \"invest\", \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"   Cost: 150. Permanently increases passive income.\n"+
			"5. {\"action\": \"save\", \"reason\": \"...\", \"taunt\": \"...\"}\n\n"+
			"State summary:\n%s\n\n"+
			"Strategic Advice:\n"+
			"- Mix tank and healer units to create a slow but steady push.\n"+
			"- Shielded enemies are best against sniper towers.\n"+
			"- Sending a wave splits enemies across all %d paths.\n"+
			"- If last_rejected_reason is non-empty, choose a different legal action next turn.\n"+
			"- Taunt your opponent to get inside their circuits!\n\n"+
			"Respond with exactly one JSON object only.",
		gameState["resources"], gameState["income"], wave, gameState["paths_count"],
		formatAffordableActions(gameState), rejectionFeedbackLine(gameState),
		promptCost(gameState, "spawn_costs", "basic", 20),
		promptCost(gameState, "spawn_costs", "fast", 30),
		promptCost(gameState, "spawn_costs", "tank", 50),
		promptCost(gameState, "spawn_costs", "shielded", 40),
		promptCost(gameState, "spawn_costs", "healer", 30),
		waveCost, stateSummary, gameState["paths_count"],
	)
	return prompt
}

func (h *GeminiHandler) parseEnemyResponse(response string) (map[string]interface{}, error) {
	if response == "" {
		return map[string]interface{}{"action": "spawn", "enemy_type": "basic", "reason": "Empty response"}, nil
	}
	re := regexp.MustCompile(`\{.*\}`)
	match := re.FindString(response)
	if match != "" {
		var decision map[string]interface{}
		if err := json.Unmarshal([]byte(match), &decision); err == nil {
			action, hasAction := decision["action"].(string)
			if hasAction {
				if action == "spawn" {
					enemyType, hasEnemyType := decision["enemy_type"].(string)
					if !hasEnemyType || enemyType == "" {
						decision["enemy_type"] = "basic"
					}
					return decision, nil
				}
				return decision, nil
			}
		}
	}
	return map[string]interface{}{"action": "spawn", "enemy_type": "basic", "reason": "Default fallback"}, nil
}

func getFallbackEnemyDecision(resources int) map[string]interface{} {
	if resources >= 200 {
		return map[string]interface{}{"action": "wave", "reason": "Fallback: High resources"}
	} else if resources >= 50 {
		return map[string]interface{}{"action": "spawn", "enemy_type": "tank", "reason": "Fallback: Tank"}
	} else if resources >= 30 {
		return map[string]interface{}{"action": "spawn", "enemy_type": "fast", "reason": "Fallback: Fast"}
	} else if resources >= 20 {
		return map[string]interface{}{"action": "spawn", "enemy_type": "basic", "reason": "Fallback: Basic"}
	}
	return map[string]interface{}{"action": "save", "reason": "Fallback: Saving"}
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
	Height              int
	Width               int
	MapHeight           int
	MapWidth            int
	MapType             string
	Paths               [][]Position
	PathTileSet         map[string]struct{}
	EnemyTileIndex      map[string][]*Enemy
	Towers              []*Tower
	Enemies             []*Enemy
	SlowZones           []*SlowZone
	Obstacles           []Position
	ObstacleTileSet     map[string]struct{}
	Particles           []*Particle
	Resources           map[string]int
	Income              map[string]int
	Lives               map[string]int
	Wave                int
	Score               map[string]int
	LastDecisions       map[string]string
	LastReasoning       map[string]string
	LastTaunt           map[string]string
	WaveQueue           []string
	GameOver            bool
	Winner              string
	AIEnabled           bool
	AIThinking          map[string]bool
	DecisionRouter      *DecisionRouter
	GameSpeed           float64
	AIDecisionInterval  map[string]int
	LastAIDecision      map[string]time.Time
	CurrentTurn         string
	LastActionTime      time.Time
	MaxResources        int
	MaxWaves            int
	TurnTimeout         time.Duration
	PauseBetweenTurns   bool
	PauseDuration       time.Duration
	lastStatePrintTime  time.Time
	lastEnemyCount      int
	lastTowerCount      int
	stateChangeCounter  int
	rng                 *rand.Rand
	Logs                []string
	MaxLogs             int
	MaxWaveQueue        int
	TickCount           int64
	StartedAt           time.Time
	ReplayEvents        []ReplayEvent
	MaxReplayEvents     int
	ActionCounters      map[string]int
	RejectedActions     map[string]int
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
	AutoWaveMinResource int
	AutoDefendMinStreak int
	AssistsDisabled     bool
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
	Player1             string
	Player2             string
	pendingTurnResults  chan turnResult
	mapInitRecorded     bool
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
		Resources: map[string]int{p1: 300, p2: 300}, Income: map[string]int{p1: 5, p2: 5}, Lives: map[string]int{p1: 20, p2: 20},
		Score: map[string]int{p1: 0, p2: 0}, LastDecisions: map[string]string{p1: "None", p2: "None"},
		LastReasoning: map[string]string{p1: "Thinking...", p2: "Thinking..."}, LastTaunt: map[string]string{p1: "", p2: ""},
		WaveQueue: make([]string, 0), GameOver: false, AIEnabled: true, AIThinking: map[string]bool{p1: false, p2: false},
		Defender: p1, Attacker: p2, ModelNames: map[string]string{p1: resolved.Player1.Model, p2: resolved.Player2.Model}, Player1: p1, Player2: p2,
		DecisionRouter: router,
		Balance:        DefaultBalanceConfig(),
		GameSpeed:      0.1, AIDecisionInterval: map[string]int{p1: 2, p2: 2},
		LastAIDecision: map[string]time.Time{p1: time.Now(), p2: time.Now()},
		CurrentTurn:    p1, LastActionTime: time.Now(), StartedAt: time.Now(), MaxResources: 800, MaxWaves: 30, TurnTimeout: 45 * time.Second,
		PauseBetweenTurns: true, PauseDuration: 1 * time.Second, lastStatePrintTime: time.Now(), rng: rng, Logs: make([]string, 0), MaxLogs: 250, MaxWaveQueue: 200, ReplayEvents: make([]ReplayEvent, 0), MaxReplayEvents: 10000, ActionCounters: map[string]int{}, RejectedActions: map[string]int{}, ProviderErrors: map[string]int{}, ProviderCalls: map[string]int{}, ProviderLatencyMS: map[string]int64{}, ProviderTokenUsage: map[string]int{}, ProviderCostMicros: map[string]int64{}, TokenPricing: map[string]tokenPricing{p1: pricingFromConfig(resolved.Player1), p2: pricingFromConfig(resolved.Player2)}, LastActionStatus: map[string]string{p1: "none", p2: "none"}, LastRejectedReason: map[string]string{p1: "", p2: ""}, NoopStreak: map[string]int{p1: 0, p2: 0}, RejectionStreak: map[string]int{p1: 0, p2: 0}, AutoWaveMinResource: 260, AutoDefendMinStreak: 2, FogOfWar: true, DefenderVisionRange: 8, BaseVisionRange: 6, ResearchLevels: map[string]int{"economy": 0, "range": 0, "control": 0}, AbilityCooldowns: map[string]int{"surge": 0, "shield_burst": 0, "reinforce_wave": 0},
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
	gameState := g.getPlayerGameState(player, role)
	if !g.isDecisionIntervalElapsed(player, currentTime) {
		return
	}
	g.handlePlayerTurn(player, role, gameState)
}

func (g *Game) switchTurn() {
	if g.CurrentTurn == g.Player1 {
		g.CurrentTurn = g.Player2
	} else {
		g.CurrentTurn = g.Player1
	}
	g.LastActionTime = time.Now()
	if g.PauseBetweenTurns {
		time.Sleep(g.PauseDuration)
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
	decision = normalizeDecision(role, decision)
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
	g.recordReplayEvent(ReplayEvent{
		Type:     ReplayDecision,
		PlayerID: playerID,
		Role:     role,
		Action:   action,
		Reason:   reason,
		Details:  map[string]interface{}{"decision": decision},
	})
	applied := false
	outcome := "rejected"
	if role == "defender" {
		if action == "place" {
			towerType, _ := decision["tower_type"].(string)
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
			if g.spawnWave() {
				g.LastDecisions[playerID] = "Launched wave (auto)"
				action = "wave"
				applied = true
				outcome = "applied_auto_wave"
				autoWaveLaunched = true
			}
		}
		if action == "spawn" {
			enemyType, _ := decision["enemy_type"].(string)
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
			} else if g.spawnWave() {
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
	g.ActionCounters[playerID+":"+action]++
	if applied {
		g.RejectionStreak[playerID] = 0
	} else {
		g.RejectionStreak[playerID]++
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
	if applied && originalAction == "save" && outcome == "applied_primary" {
		g.NoopStreak[playerID]++
	} else if applied {
		g.NoopStreak[playerID] = 0
	}
	g.LastActionStatus[playerID] = outcome
}

func classifyActionOutcome(outcome string) string {
	switch {
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
				g.ProviderErrors[result.playerID+":"+providerErrorLabel(result.err)]++
				g.recordReplayEvent(ReplayEvent{
					Type:     ReplayProviderErr,
					PlayerID: result.playerID,
					Role:     result.role,
					Reason:   providerErrorLabel(result.err),
					Details:  map[string]interface{}{"error": result.err.Error()},
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
