package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"regexp"
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
	t.Damage = int(float64(t.Damage) * 1.5)
	t.Range = int(float64(t.Range) * 1.2)
	t.MaxCD = int(float64(t.MaxCD) * 0.9)
	if t.MaxCD < 1 {
		t.MaxCD = 1
	}
}

func NewTower(y, x int, towerType string, params map[string]interface{}) Tower {
	types := map[string]map[string]interface{}{
		"basic":  {"char": '^', "damage": 15, "range": 5, "cooldown": 5, "cost": 100},
		"sniper": {"char": '⌖', "damage": 50, "range": 12, "cooldown": 15, "cost": 250},
		"splash": {"char": '⊕', "damage": 10, "range": 3, "cooldown": 3, "cost": 200},
		"buffer": {"char": 'B', "damage": 0, "range": 2, "cooldown": 0, "cost": 300}, // Buffs nearby towers
		"custom": {"char": '?', "damage": 20, "range": 7, "cooldown": 8, "cost": 150},
	}

	t := types[towerType]
	if towerType == "custom" && params != nil {
		for k, v := range params {
			t[k] = v
		}
	}

	char := t["char"].(rune)
	damage := toInt(t["damage"])
	maxCD := toInt(t["cooldown"])
	rangeVal := toInt(t["range"])
	cost := toInt(t["cost"])

	return Tower{
		Entity: Entity{
			Pos:       Position{Y: y, X: x},
			Char:      char,
			Health:    100,
			MaxHealth: 100,
			Damage:    damage,
			Cooldown:  0,
			MaxCD:     maxCD,
		},
		TowerType: towerType,
		Range:     rangeVal,
		Cost:      cost,
		Strategy:  "nearest",
	}
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

func NewEnemy(y, x int, enemyType string, params map[string]interface{}) Enemy {
	types := map[string]map[string]interface{}{
		"basic":    {"char": 'o', "health": float64(100), "speed": float64(1.0), "reward": float64(20)},
		"fast":     {"char": '>', "health": float64(50), "speed": float64(2.0), "reward": float64(15)},
		"tank":     {"char": '□', "health": float64(300), "speed": float64(0.5), "reward": float64(50)},
		"shielded": {"char": 'S', "health": float64(150), "speed": float64(0.8), "reward": float64(40), "shield": float64(2)},
		"healer":   {"char": 'H', "health": float64(80), "speed": float64(1.0), "reward": float64(30)},
		"custom":   {"char": '?', "health": float64(150), "speed": float64(1.2), "reward": float64(25)},
	}

	e := types[enemyType]
	if enemyType == "custom" && params != nil {
		for k, v := range params {
			e[k] = v
		}
	}

	char := e["char"].(rune)
	health := int(e["health"].(float64))
	speed := e["speed"].(float64)
	reward := int(e["reward"].(float64))
	shield := 0
	if s, ok := e["shield"]; ok {
		shield = int(s.(float64))
	}

	return Enemy{
		Entity: Entity{
			Pos:       Position{Y: y, X: x},
			Char:      char,
			Health:    health,
			MaxHealth: health,
			Damage:    0,
			Cooldown:  0,
			MaxCD:     0,
		},
		EnemyType:     enemyType,
		Speed:         speed,
		Reward:        reward,
		DistanceMoved: 0,
		PathIndex:     0,
		PathID:        0,
		Shield:        shield,
	}
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
		"max_tokens":  150,
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

func (h *OpenAIHandler) createTowerPrompt(gameState map[string]interface{}) string {
	pathsCount := gameState["paths_count"].(int)
	wave := gameState["wave"].(int)
	prompt := fmt.Sprintf(
		"You are the Defender in a Tower Defense Battleground. Goal: Stop enemies from reaching the end.\n"+
			"Current Resources: %v, Base Income: %v, Wave: %d, Paths: %d\n\n"+
			"Actions:\n"+
			"1. {\"action\": \"place\", \"tower_type\": \"basic|sniper|splash|buffer\", \"position\": [y, x], \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"   Costs: basic (100), splash (200), sniper (250), buffer (300)\n"+
			"2. {\"action\": \"upgrade\", \"tower_id\": <int>, \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"   Cost: 150 * (current_level + 1). Increases damage and range.\n"+
			"3. {\"action\": \"place_slow_zone\", \"position\": [y, x], \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"   Cost: 150. Reduces enemy speed by 50%%. MUST be on a path.\n"+
			"4. {\"action\": \"invest\", \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"   Cost: 150. Permanently increases passive income.\n"+
			"5. {\"action\": \"save\", \"reason\": \"...\", \"taunt\": \"...\"}\n\n"+
			"Strategic Advice:\n"+
			"- Buffer towers (B) increase damage of nearby towers by 50%%. Place them in clusters.\n"+
			"- Watch out for Healer enemies (H) and Shielded enemies (S).\n"+
			"- Invest early if you can afford to, but don't let your lives drop too low.\n"+
			"- You can send a taunt message to your opponent!\n\n"+
			"Respond with JSON only.",
		gameState["resources"], gameState["income"], wave, pathsCount,
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
				} else if action == "save" {
					return map[string]interface{}{
						"action":     "place",
						"tower_type": "basic",
						"position":   []interface{}{float64(10), float64(10)},
						"reason":     "Converted save to place for better defense",
					}, nil
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
	waveCost := 40 + (wave * 5)
	if waveCost > 200 {
		waveCost = 200
	}
	prompt := fmt.Sprintf(
		"You are the Attacker in a Tower Defense Battleground. Goal: Overwhelm the Defender.\n"+
			"Current Resources: %v, Base Income: %v, Wave: %d, Paths: %d\n\n"+
			"Enemy Options (cost):\n"+
			"- basic (20): Standard unit\n"+
			"- fast (30): Quick and nimble\n"+
			"- tank (50): High durability\n"+
			"- shielded (40): Takes 50%% less damage from all towers\n"+
			"- healer (30): Heals nearby enemies by 2 HP per tick\n"+
			"- wave (%d): Massive multi-path assault\n\n"+
			"Actions:\n"+
			"1. {\"action\": \"spawn\", \"enemy_type\": \"basic|fast|tank|shielded|healer\", \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"2. {\"action\": \"wave\", \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"3. {\"action\": \"invest\", \"reason\": \"...\", \"taunt\": \"...\"}\n"+
			"   Cost: 150. Permanently increases passive income.\n"+
			"4. {\"action\": \"save\", \"reason\": \"...\", \"taunt\": \"...\"}\n\n"+
			"Strategic Advice:\n"+
			"- Mix tank and healer units to create a slow but steady push.\n"+
			"- Shielded enemies are best against sniper towers.\n"+
			"- Sending a wave splits enemies across all %d paths.\n"+
			"- Taunt your opponent to get inside their circuits!\n\n"+
			"Respond with JSON only.",
		gameState["resources"], gameState["income"], wave, gameState["paths_count"],
		waveCost, gameState["paths_count"],
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

type Game struct {
	Height             int
	Width              int
	MapHeight          int
	MapWidth           int
	Paths              [][]Position
	Towers             []*Tower
	Enemies            []*Enemy
	SlowZones          []*SlowZone
	Obstacles          []Position
	Particles          []*Particle
	Resources          map[string]int
	Income             map[string]int
	Lives              map[string]int
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
	OpenAIHandler      *OpenAIHandler
	GeminiHandler      *GeminiHandler
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
	lastStatePrintTime time.Time
	lastEnemyCount     int
	lastTowerCount     int
	stateChangeCounter int
	rng                *rand.Rand
	Logs               []string
	Defender           string
	Attacker           string
	ModelNames         map[string]string
	Player1            string
	Player2            string
}

func NewGame(openaiKey, googleKey string) *Game {
	width := 80
	height := 24
	mapHeight := height - 10
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	p1, p2 := "p1", "p2"
	game := &Game{
		Height: height, Width: width, MapHeight: mapHeight, MapWidth: width,
		Towers: make([]*Tower, 0), Enemies: make([]*Enemy, 0), SlowZones: make([]*SlowZone, 0), Obstacles: make([]Position, 0), Particles: make([]*Particle, 0),
		Resources: map[string]int{p1: 300, p2: 300}, Income: map[string]int{p1: 5, p2: 5}, Lives: map[string]int{p1: 20, p2: 20},
		Score: map[string]int{p1: 0, p2: 0}, LastDecisions: map[string]string{p1: "None", p2: "None"},
		LastReasoning: map[string]string{p1: "Thinking...", p2: "Thinking..."}, LastTaunt: map[string]string{p1: "", p2: ""},
		WaveQueue: make([]string, 0), GameOver: false, AIEnabled: true, AIThinking: map[string]bool{p1: false, p2: false},
		Defender: p1, Attacker: p2, ModelNames: map[string]string{p1: "o3", p2: "gemini-2.5-pro"}, Player1: p1, Player2: p2,
		OpenAIHandler: &OpenAIHandler{AIHandler: NewAIHandler(rng), APIKey: openaiKey},
		GeminiHandler: &GeminiHandler{AIHandler: NewAIHandler(rng), APIKey: googleKey},
		GameSpeed:     0.1, AIDecisionInterval: map[string]int{p1: 2, p2: 2},
		LastAIDecision: map[string]time.Time{p1: time.Now(), p2: time.Now()},
		CurrentTurn:    p1, LastActionTime: time.Now(), MaxResources: 800, MaxWaves: 30, TurnTimeout: 45 * time.Second,
		PauseBetweenTurns: true, PauseDuration: 1 * time.Second, lastStatePrintTime: time.Now(), rng: rng, Logs: make([]string, 0),
	}
	game.Paths = game.generatePaths()
	game.generateObstacles()
	return game
}

func (g *Game) generatePaths() [][]Position {
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
		onPath := false
		for _, path := range g.Paths {
			for _, p := range path {
				if p.Y == obs.Y && p.X == obs.X {
					onPath = true
					break
				}
			}
		}
		if !onPath {
			g.Obstacles = append(g.Obstacles, obs)
		}
	}
}

func (g *Game) HandleAIDecisions() {
	if !g.AIEnabled || g.GameOver {
		return
	}
	currentTime := time.Now()
	gameState := g.getGameState()
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
		defer func() { g.AIThinking[playerID] = false }()
		var decision map[string]interface{}
		var err error
		if playerID == g.Player1 {
				if role == "defender" {
					if g.Resources[playerID] < 100 {
						g.logf("%s (Def) saving resources (%d)", g.ModelNames[playerID], g.Resources[playerID])
						g.LastAIDecision[playerID] = time.Now()
						g.switchTurn()
						return
					}
				decision, err = g.OpenAIHandler.GetTowerDecision(gameState)
				} else {
					if g.Resources[playerID] < 20 {
						g.logf("%s (Att) saving resources (%d)", g.ModelNames[playerID], g.Resources[playerID])
						g.LastAIDecision[playerID] = time.Now()
						g.switchTurn()
						return
					}
				decision, err = g.OpenAIHandler.GetEnemyDecision(gameState)
			}
		} else {
				if role == "defender" {
					if g.Resources[playerID] < 100 {
						g.logf("%s (Def) saving resources (%d)", g.ModelNames[playerID], g.Resources[playerID])
						g.LastAIDecision[playerID] = time.Now()
						g.switchTurn()
						return
					}
				decision, err = g.GeminiHandler.GetTowerDecision(gameState)
				} else {
					if g.Resources[playerID] < 20 {
						g.logf("%s (Att) saving resources (%d)", g.ModelNames[playerID], g.Resources[playerID])
						g.LastAIDecision[playerID] = time.Now()
						g.switchTurn()
						return
					}
				decision, err = g.GeminiHandler.GetEnemyDecision(gameState)
			}
		}
		if err != nil {
			g.logf("%s API error: %v", g.ModelNames[playerID], err)
			g.LastAIDecision[playerID] = time.Now()
			g.switchTurn()
			return
		}
		g.applyDecision(playerID, role, decision)
		g.LastAIDecision[playerID] = time.Now()
		g.switchTurn()
	}()
}

func (g *Game) applyDecision(playerID, role string, decision map[string]interface{}) {
	action, _ := decision["action"].(string)
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
	if role == "defender" {
		if action == "place" {
			towerType, _ := decision["tower_type"].(string)
			y, x := parseDecisionPosition(decision["position"], 2, 2)
			if g.placeTower(y, x, towerType) {
				g.LastDecisions[playerID] = fmt.Sprintf("Placed %s tower at [%d,%d]", towerType, y, x)
			}
		} else if action == "upgrade" {
			towerID := -1
			if id, ok := toIntFromAny(decision["tower_id"]); ok {
				towerID = id
			}
			if g.upgradeTower(towerID) {
				g.LastDecisions[playerID] = fmt.Sprintf("Upgraded tower #%d", towerID)
			}
		} else if action == "place_slow_zone" {
			y, x := parseDecisionPosition(decision["position"], -1, -1)
			if g.placeSlowZone(y, x) {
				g.LastDecisions[playerID] = fmt.Sprintf("Placed slow zone at [%d,%d]", y, x)
			}
		} else if action == "invest" {
			if g.invest(playerID) {
				g.LastDecisions[playerID] = "Invested in economy"
			}
		} else {
			g.LastDecisions[playerID] = "Saving resources"
		}
	} else {
		if action == "spawn" {
			enemyType, _ := decision["enemy_type"].(string)
			if g.spawnEnemy(enemyType, nil) {
				g.LastDecisions[playerID] = fmt.Sprintf("Spawned %s enemy", enemyType)
			}
		} else if action == "wave" {
			if g.spawnWave() {
				g.LastDecisions[playerID] = "Launched wave"
			}
		} else if action == "invest" {
			if g.invest(playerID) {
				g.LastDecisions[playerID] = "Invested in economy"
			}
		} else {
			g.LastDecisions[playerID] = "Saving resources"
		}
	}
}

func (g *Game) logf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	g.Logs = append(g.Logs, msg)
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
