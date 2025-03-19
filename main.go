package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/joho/godotenv"
)

// API Keys
var openaiAPIKey string
var googleAPIKey string

// Game entities
type Position struct {
	Y int
	X int
}

type Entity struct {
	Pos      Position
	Char     rune
	Health   int
	Damage   int
	Cooldown int
	MaxCD    int
}

type Tower struct {
	Entity
	TowerType string
	Range     int
	Cost      int
	Strategy  string
}

func NewTower(y, x int, towerType string, params map[string]interface{}) Tower {
	types := map[string]map[string]interface{}{
		"basic":  {"char": '^', "damage": 15, "range": 5, "cooldown": 5, "cost": 100},
		"sniper": {"char": '⌖', "damage": 50, "range": 12, "cooldown": 15, "cost": 250},
		"splash": {"char": '⊕', "damage": 10, "range": 3, "cooldown": 3, "cost": 200},
		"custom": {"char": '?', "damage": 20, "range": 7, "cooldown": 8, "cost": 150},
	}

	t := types[towerType]
	if towerType == "custom" && params != nil {
		for k, v := range params {
			t[k] = v
		}
	}

	char := []rune(t["char"].(string))[0]
	damage := int(t["damage"].(float64))
	maxCD := int(t["cooldown"].(float64))
	rangeVal := int(t["range"].(float64))
	cost := int(t["cost"].(float64))

	return Tower{
		Entity: Entity{
			Pos:      Position{Y: y, X: x},
			Char:     char,
			Health:   100,
			Damage:   damage,
			Cooldown: 0,
			MaxCD:    maxCD,
		},
		TowerType: towerType,
		Range:     rangeVal,
		Cost:      cost,
		Strategy:  "nearest",
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

	// Sort targets by distance/criteria
	for i := 0; i < len(targets)-1; i++ {
		for j := i + 1; j < len(targets); j++ {
			if targets[i].distance > targets[j].distance {
				targets[i], targets[j] = targets[j], targets[i]
			}
		}
	}

	var hitEnemies []*Enemy
	if t.TowerType == "splash" {
		// Attack up to 3 enemies in range
		limit := 3
		if len(targets) < limit {
			limit = len(targets)
		}
		for i := 0; i < limit; i++ {
			targets[i].enemy.Health -= t.Damage
			hitEnemies = append(hitEnemies, targets[i].enemy)
		}
	} else {
		// Single target attack
		targets[0].enemy.Health -= t.Damage
		hitEnemies = append(hitEnemies, targets[0].enemy)
	}

	t.Cooldown = t.MaxCD
	return hitEnemies
}

type Enemy struct {
	Entity
	EnemyType     string
	Speed         float64
	Reward        int
	DistanceMoved float64
	PathIndex     int
	Behavior      string
}

func NewEnemy(y, x int, enemyType string, params map[string]interface{}) Enemy {
	types := map[string]map[string]interface{}{
		"basic":  {"char": 'o', "health": 100, "speed": 1.0, "reward": 20},
		"fast":   {"char": '>', "health": 50, "speed": 2.0, "reward": 15},
		"tank":   {"char": '□', "health": 300, "speed": 0.5, "reward": 50},
		"custom": {"char": '?', "health": 150, "speed": 1.2, "reward": 25},
	}

	e := types[enemyType]
	if enemyType == "custom" && params != nil {
		for k, v := range params {
			e[k] = v
		}
	}

	char := []rune(e["char"].(string))[0]
	health := int(e["health"].(float64))
	speed := e["speed"].(float64)
	reward := int(e["reward"].(float64))

	return Enemy{
		Entity: Entity{
			Pos:      Position{Y: y, X: x},
			Char:     char,
			Health:   health,
			Damage:   0,
			Cooldown: 0,
			MaxCD:    0,
		},
		EnemyType:     enemyType,
		Speed:         speed,
		Reward:        reward,
		DistanceMoved: 0,
		PathIndex:     0,
		Behavior:      "direct",
	}
}

// AI API handlers
type AIHandler struct {
	Client *http.Client
}

func NewAIHandler() *AIHandler {
	return &AIHandler{
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type OpenAIHandler struct {
	*AIHandler
}

func (h *OpenAIHandler) GetTowerDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	prompt := h.createTowerPrompt(gameState)

	// Create request body
	reqBody := map[string]interface{}{
		"model": "gpt-3.5-turbo",
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

	req.Header.Set("Authorization", "Bearer "+openaiAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	// Extract response content
	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return map[string]interface{}{"action": "none", "reason": "API response error"}, nil
	}

	choice := choices[0].(map[string]interface{})
	message := choice["message"].(map[string]interface{})
	content := message["content"].(string)

	return h.parseTowerResponse(content)
}

func (h *OpenAIHandler) createTowerPrompt(gameState map[string]interface{}) string {
	enemies := gameState["enemies"].([]interface{})
	towers := gameState["towers"].([]interface{})
	resources := gameState["resources"].(map[string]interface{})["chatgpt"].(int)

	prompt := fmt.Sprintf(
		"You are playing a tower defense game as ChatGPT. You have %d resources. "+
			"There are %d enemies and %d towers on the map.\n\n"+
			"You can:\n"+
			"1. Place a tower: basic (100), sniper (250), splash (200)\n"+
			"2. Upgrade an existing tower\n"+
			"3. Change a tower's targeting strategy\n"+
			"4. Save resources for now\n\n"+
			"Return your decision in JSON format like: {\"action\": \"place\", \"tower_type\": \"basic\", \"position\": [10, 15], \"reason\": \"Explanation\"}\n"+
			"Valid actions are: place, upgrade, change_strategy, save",
		resources, len(enemies), len(towers),
	)
	return prompt
}

func (h *OpenAIHandler) parseTowerResponse(response string) (map[string]interface{}, error) {
	// Try to extract JSON from the response
	re := regexp.MustCompile(`\{.*\}`)
	match := re.FindString(response)

	if match != "" {
		var decision map[string]interface{}
		err := json.Unmarshal([]byte(match), &decision)
		if err == nil {
			return decision, nil
		}
	}

	// Fallback to basic parsing
	decision := map[string]interface{}{
		"action": "none",
		"reason": "Could not parse response",
	}

	if strings.Contains(strings.ToLower(response), "place") && strings.Contains(strings.ToLower(response), "basic") {
		decision = map[string]interface{}{
			"action":     "place",
			"tower_type": "basic",
			"position":   []int{10, 10},
		}
	} else if strings.Contains(strings.ToLower(response), "save") {
		decision = map[string]interface{}{
			"action": "save",
			"reason": "Saving resources",
		}
	}

	return decision, nil
}

type GeminiHandler struct {
	*AIHandler
}

func (h *GeminiHandler) GetEnemyDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	prompt := h.createEnemyPrompt(gameState)

	// Create request body
	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.7,
			"maxOutputTokens": 150,
		},
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST",
		fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent?key=%s", googleAPIKey),
		bytes.NewBuffer(reqJSON))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	// Extract response content
	candidates, ok := result["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return map[string]interface{}{"action": "none", "reason": "API response error"}, nil
	}

	candidate := candidates[0].(map[string]interface{})
	content := candidate["content"].(map[string]interface{})
	parts := content["parts"].([]interface{})
	text := parts[0].(map[string]interface{})["text"].(string)

	return h.parseEnemyResponse(text)
}

func (h *GeminiHandler) createEnemyPrompt(gameState map[string]interface{}) string {
	enemies := gameState["enemies"].([]interface{})
	towers := gameState["towers"].([]interface{})
	resources := gameState["resources"].(map[string]interface{})["gemini"].(int)
	wave := gameState["wave"].(int)

	prompt := fmt.Sprintf(
		"You are playing a tower defense game as Gemini. You have %d resources. "+
			"There are %d active enemies and %d defensive towers.\n"+
			"Current wave: %d\n\n"+
			"You can:\n"+
			"1. Spawn individual enemies: basic (20), fast (30), tank (50)\n"+
			"2. Launch a wave (costs 100 × wave number)\n"+
			"3. Create a custom enemy (costs 40)\n"+
			"4. Save resources for now\n\n"+
			"Return your decision in JSON format like: {\"action\": \"spawn\", \"enemy_type\": \"fast\", \"reason\": \"Explanation\"}\n"+
			"Valid actions are: spawn, wave, custom, save",
		resources, len(enemies), len(towers), wave,
	)
	return prompt
}

func (h *GeminiHandler) parseEnemyResponse(response string) (map[string]interface{}, error) {
	// Try to extract JSON from the response
	re := regexp.MustCompile(`\{.*\}`)
	match := re.FindString(response)

	if match != "" {
		var decision map[string]interface{}
		err := json.Unmarshal([]byte(match), &decision)
		if err == nil {
			return decision, nil
		}
	}

	// Fallback to basic parsing
	decision := map[string]interface{}{
		"action": "none",
		"reason": "Could not parse response",
	}

	if strings.Contains(strings.ToLower(response), "spawn") && strings.Contains(strings.ToLower(response), "fast") {
		decision = map[string]interface{}{
			"action":     "spawn",
			"enemy_type": "fast",
		}
	} else if strings.Contains(strings.ToLower(response), "wave") {
		decision = map[string]interface{}{
			"action": "wave",
		}
	}

	return decision, nil
}

// Game struct and methods
type Game struct {
	Screen        tcell.Screen
	Height        int
	Width         int
	MapHeight     int
	MapWidth      int
	Path          []Position
	Towers        []*Tower
	Enemies       []*Enemy
	Resources     map[string]int
	Lives         map[string]int
	Wave          int
	Score         map[string]int
	LastDecisions map[string]string
	WaveQueue     []string
	GameOver      bool
	Winner        string
	AIEnabled     bool
	AIThinking    map[string]bool

	// AI handlers
	OpenAIHandler *OpenAIHandler
	GeminiHandler *GeminiHandler

	// Game settings
	GameSpeed          float64
	AIDecisionInterval map[string]int
	LastAIDecision     map[string]time.Time
}

func NewGame(screen tcell.Screen) *Game {
	// Initialize screen
	width, height := screen.Size()
	mapHeight := height - 10 // Reserve space for UI

	game := &Game{
		Screen:             screen,
		Height:             height,
		Width:              width,
		MapHeight:          mapHeight,
		MapWidth:           width,
		Towers:             make([]*Tower, 0),
		Enemies:            make([]*Enemy, 0),
		Resources:          map[string]int{"chatgpt": 300, "gemini": 300},
		Lives:              map[string]int{"chatgpt": 20},
		Wave:               1,
		Score:              map[string]int{"chatgpt": 0, "gemini": 0},
		LastDecisions:      map[string]string{"chatgpt": "None", "gemini": "None"},
		WaveQueue:          make([]string, 0),
		GameOver:           false,
		AIEnabled:          true,
		AIThinking:         map[string]bool{"chatgpt": false, "gemini": false},
		OpenAIHandler:      &OpenAIHandler{AIHandler: NewAIHandler()},
		GeminiHandler:      &GeminiHandler{AIHandler: NewAIHandler()},
		GameSpeed:          0.1,
		AIDecisionInterval: map[string]int{"chatgpt": 5, "gemini": 5},
		LastAIDecision: map[string]time.Time{
			"chatgpt": time.Now(),
			"gemini":  time.Now(),
		},
	}

	// Generate path
	game.Path = game.generatePath()

	return game
}

func (g *Game) generatePath() []Position {
	path := make([]Position, 0)
	zigzagHeight := g.MapHeight / 3
	centerY := g.MapHeight / 2

	// Entry point
	for i := 0; i < 5; i++ {
		path = append(path, Position{Y: centerY, X: i})
	}

	// Zigzag across
	x := 5
	goingUp := true
	for x < g.Width-10 {
		x++
		if goingUp {
			for y := centerY; y > centerY-zigzagHeight; y-- {
				path = append(path, Position{Y: y, X: x})
			}
			goingUp = false
		} else {
			for y := centerY - zigzagHeight; y < centerY; y++ {
				path = append(path, Position{Y: y, X: x})
			}
			goingUp = true
		}
	}

	// Exit point
	lastPos := path[len(path)-1]
	for i := 1; i <= 5; i++ {
		path = append(path, Position{Y: lastPos.Y, X: lastPos.X + i})
	}

	return path
}

func (g *Game) drawUI() {
	// Clear screen
	g.Screen.Clear()

	// Draw path
	for _, pos := range g.Path {
		if pos.Y >= 0 && pos.Y < g.Height && pos.X >= 0 && pos.X < g.Width {
			g.Screen.SetContent(pos.X, pos.Y, '·', nil, tcell.StyleDefault)
		}
	}

	// Draw towers
	for _, tower := range g.Towers {
		if tower.Pos.Y >= 0 && tower.Pos.Y < g.Height && tower.Pos.X >= 0 && tower.Pos.X < g.Width {
			g.Screen.SetContent(tower.Pos.X, tower.Pos.Y, tower.Char, nil, tcell.StyleDefault.Foreground(tcell.ColorGreen))
		}
	}

	// Draw enemies
	for _, enemy := range g.Enemies {
		if enemy.Pos.Y >= 0 && enemy.Pos.Y < g.Height && enemy.Pos.X >= 0 && enemy.Pos.X < g.Width {
			g.Screen.SetContent(enemy.Pos.X, enemy.Pos.Y, enemy.Char, nil, tcell.StyleDefault.Foreground(tcell.ColorBlue))
		}
	}

	// Draw status bar
	statusY := g.Height - 9

	// ChatGPT status
	drawText(g.Screen, 1, statusY, "ChatGPT:", tcell.StyleDefault.Foreground(tcell.ColorGreen))
	drawText(g.Screen, 10, statusY, fmt.Sprintf("Lives: %d | Resources: %d | Score: %d",
		g.Lives["chatgpt"], g.Resources["chatgpt"], g.Score["chatgpt"]), tcell.StyleDefault)

	// Gemini status
	drawText(g.Screen, 1, statusY+1, "Gemini:", tcell.StyleDefault.Foreground(tcell.ColorBlue))
	drawText(g.Screen, 10, statusY+1, fmt.Sprintf("Resources: %d | Score: %d | Wave: %d",
		g.Resources["gemini"], g.Score["gemini"], g.Wave), tcell.StyleDefault)

	// AI thinking indicators
	thinkingStatus := "  "
	if g.AIThinking["chatgpt"] {
		thinkingStatus = "🤔"
	}
	drawText(g.Screen, g.Width-10, statusY, thinkingStatus, tcell.StyleDefault)

	thinkingStatus = "  "
	if g.AIThinking["gemini"] {
		thinkingStatus = "🤔"
	}
	drawText(g.Screen, g.Width-10, statusY+1, thinkingStatus, tcell.StyleDefault)

	// Last decisions
	drawText(g.Screen, 1, statusY+3, fmt.Sprintf("ChatGPT decision: %s", g.LastDecisions["chatgpt"]),
		tcell.StyleDefault.Foreground(tcell.ColorGreen))
	drawText(g.Screen, 1, statusY+4, fmt.Sprintf("Gemini decision: %s", g.LastDecisions["gemini"]),
		tcell.StyleDefault.Foreground(tcell.ColorBlue))

	// Controls and settings
	aiStatus := "ON"
	if !g.AIEnabled {
		aiStatus = "OFF"
	}
	drawText(g.Screen, 1, statusY+6, fmt.Sprintf("[Q]uit [A]I: %s [+/-] Speed: %.1fx",
		aiStatus, 1/g.GameSpeed), tcell.StyleDefault.Foreground(tcell.ColorYellow))
	drawText(g.Screen, 1, statusY+7, "Tower types: basic (^) sniper (⌖) splash (⊕) | Enemy types: basic (o) fast (>) tank (□)",
		tcell.StyleDefault.Foreground(tcell.ColorYellow))

	// Show the screen
	g.Screen.Show()
}

func drawText(s tcell.Screen, x, y int, text string, style tcell.Style) {
	for i, r := range []rune(text) {
		s.SetContent(x+i, y, r, nil, style)
	}
}

func (g *Game) getGameState() map[string]interface{} {
	towerData := make([]interface{}, len(g.Towers))
	for i, tower := range g.Towers {
		towerData[i] = map[string]interface{}{
			"type":     tower.TowerType,
			"position": []int{tower.Pos.Y, tower.Pos.X},
			"damage":   tower.Damage,
			"range":    tower.Range,
			"cooldown": tower.Cooldown,
		}
	}

	enemyData := make([]interface{}, len(g.Enemies))
	for i, enemy := range g.Enemies {
		pathPosition := int(math.Min(float64(enemy.DistanceMoved), float64(len(g.Path)-1)))
		totalPath := len(g.Path)
		enemyData[i] = map[string]interface{}{
			"type":     enemy.EnemyType,
			"position": []int{enemy.Pos.Y, enemy.Pos.X},
			"health":   enemy.Health,
			"speed":    enemy.Speed,
			"progress": float64(pathPosition) / float64(totalPath),
		}
	}

	return map[string]interface{}{
		"towers":  towerData,
		"enemies": enemyData,
		"resources": map[string]interface{}{
			"chatgpt": g.Resources["chatgpt"],
			"gemini":  g.Resources["gemini"],
		},
		"lives":       g.Lives["chatgpt"],
		"wave":        g.Wave,
		"score":       g.Score,
		"path_length": len(g.Path),
	}
}

func (g *Game) placeTower(y, x int, towerType string) bool {
	// Check if position is valid (not on path)
	for _, pos := range g.Path {
		if abs(pos.Y-y) < 2 && abs(pos.X-x) < 2 {
			return false
		}
	}

	// Check if tower already exists at position
	for _, tower := range g.Towers {
		if abs(tower.Pos.Y-y) < 2 && abs(tower.Pos.X-x) < 2 {
			return false
		}
	}

	// Check if enough resources
	towerCosts := map[string]int{"basic": 100, "sniper": 250, "splash": 200, "custom": 150}
	if g.Resources["chatgpt"] < towerCosts[towerType] {
		return false
	}

	// Place tower
	tower := NewTower(y, x, towerType, nil)
	g.Towers = append(g.Towers, &tower)
	g.Resources["chatgpt"] -= tower.Cost
	return true
}

func (g *Game) spawnEnemy(enemyType string, params map[string]interface{}) bool {
	enemyCosts := map[string]int{"basic": 20, "fast": 30, "tank": 50, "custom": 40}
	cost := enemyCosts[enemyType]

	if g.Resources["gemini"] < cost {
		return false
	}

	// Get starting position (beginning of path)
	if len(g.Path) == 0 {
		return false
	}

	startPos := g.Path[0]

	// Create and add enemy
	enemy := NewEnemy(startPos.Y, startPos.X, enemyType, params)
	g.Enemies = append(g.Enemies, &enemy)
	g.Resources["gemini"] -= cost
	return true
}

func (g *Game) spawnWave() bool {
	waveCost := g.Wave * 100
	if g.Resources["gemini"] < waveCost {
		return false
	}

	// Create a mix of enemies
	numEnemies := g.Wave*3 + 2
	enemyTypes := make([]string, 0)

	// More varied waves as game progresses
	if g.Wave < 3 {
		for i := 0; i < numEnemies; i++ {
			enemyTypes = append(enemyTypes, "basic")
		}
	} else if g.Wave < 5 {
		for i := 0; i < numEnemies/2; i++ {
			enemyTypes = append(enemyTypes, "basic")
		}
		for i := 0; i < numEnemies/2; i++ {
			enemyTypes = append(enemyTypes, "fast")
		}
	} else {
		for i := 0; i < numEnemies/3; i++ {
			enemyTypes = append(enemyTypes, "basic")
		}
		for i := 0; i < numEnemies/3; i++ {
			enemyTypes = append(enemyTypes, "fast")
		}
		for i := 0; i < numEnemies/3; i++ {
			enemyTypes = append(enemyTypes, "tank")
		}
	}

	// Shuffle the types
	rand.Shuffle(len(enemyTypes), func(i, j int) {
		enemyTypes[i], enemyTypes[j] = enemyTypes[j], enemyTypes[i]
	})

	g.WaveQueue = append(g.WaveQueue, enemyTypes...)

	g.Resources["gemini"] -= waveCost
	g.Wave++
	return true
}

func (g *Game) handleAIDecisions() {
	currentTime := time.Now()
	gameState := g.getGameState()

	// ChatGPT's turn to make a decision
	chatgptIntervalDuration := time.Duration(g.AIDecisionInterval["chatgpt"]) * time.Second
	if currentTime.Sub(g.LastAIDecision["chatgpt"]) >= chatgptIntervalDuration &&
		!g.AIThinking["chatgpt"] && g.AIEnabled {
		g.AIThinking["chatgpt"] = true

		// Run in a separate goroutine to avoid blocking
		go func() {
			// Get tower decision from ChatGPT
			decision, err := g.OpenAIHandler.GetTowerDecision(gameState)

			// Process decision (this callback runs after API response)
			if err == nil {
				action, _ := decision["action"].(string)

				if action == "place" {
					towerType, _ := decision["tower_type"].(string)
					if towerType == "" {
						towerType = "basic"
					}

					position, ok := decision["position"].([]interface{})
					y, x := 10, 10
					if ok && len(position) >= 2 {
						y = int(position[0].(float64))
						x = int(position[1].(float64))
					}

					// Place tower at a valid position near the suggested point
					placed := false
					for offY := -5; offY <= 5 && !placed; offY++ {
						for offX := -5; offX <= 5 && !placed; offX++ {
							tryY, tryX := y+offY, x+offX
							if tryY > 0 && tryY < g.MapHeight && tryX > 0 && tryX < g.Width {
								placed = g.placeTower(tryY, tryX, towerType)
							}
						}
					}

					if placed {
						g.LastDecisions["chatgpt"] = fmt.Sprintf("Placed %s tower", towerType)
					} else {
						g.LastDecisions["chatgpt"] = fmt.Sprintf("Failed to place %s tower", towerType)
					}
				} else {
					g.LastDecisions["chatgpt"] = "Saving resources"
				}
			} else {
				g.LastDecisions["chatgpt"] = "API error"
			}

			g.AIThinking["chatgpt"] = false
			g.LastAIDecision["chatgpt"] = time.Now()
		}()
	}

	// Gemini's turn to make a decision
	geminiIntervalDuration := time.Duration(g.AIDecisionInterval["gemini"]) * time.Second
	if currentTime.Sub(g.LastAIDecision["gemini"]) >= geminiIntervalDuration &&
		!g.AIThinking["gemini"] && g.AIEnabled {
		g.AIThinking["gemini"] = true

		// Run in a separate goroutine to avoid blocking
		go func() {
			// Get enemy decision from Gemini
			decision, err := g.GeminiHandler.GetEnemyDecision(gameState)

			// Process decision
			if err == nil {
				action, _ := decision["action"].(string)

				if action == "spawn" {
					enemyType, _ := decision["enemy_type"].(string)
					if enemyType == "" {
						enemyType = "basic"
					}

					spawned := g.spawnEnemy(enemyType, nil)
					if spawned {
						g.LastDecisions["gemini"] = fmt.Sprintf("Spawned %s enemy", enemyType)
					} else {
						g.LastDecisions["gemini"] = fmt.Sprintf("Failed to spawn %s enemy", enemyType)
					}
				} else if action == "wave" {
					if g.spawnWave() {
						g.LastDecisions["gemini"] = fmt.Sprintf("Launched wave %d", g.Wave)
					} else {
						g.LastDecisions["gemini"] = "Failed to launch wave (not enough resources)"
					}
				} else if action == "custom" {
					params := map[string]interface{}{
						"char":   "X",
						"health": 200.0,
						"speed":  1.5,
						"reward": 35.0,
					}

					customData, ok := decision["custom_params"].(map[string]interface{})
					if ok {
						for k, v := range customData {
							params[k] = v
						}
					}

					spawned := g.spawnEnemy("custom", params)
					if spawned {
						g.LastDecisions["gemini"] = "Spawned custom enemy"
					} else {
						g.LastDecisions["gemini"] = "Failed to spawn custom enemy"
					}
				} else {
					g.LastDecisions["gemini"] = "Saving resources"
				}
			} else {
				g.LastDecisions["gemini"] = "API error"
			}

			g.AIThinking["gemini"] = false
			g.LastAIDecision["gemini"] = time.Now()
		}()
	}
}

func (g *Game) updateGameState() {
	// Process wave queue
	if len(g.WaveQueue) > 0 && len(g.Enemies) < 20 {
		enemyType := g.WaveQueue[0]
		g.WaveQueue = g.WaveQueue[1:]
		g.spawnEnemy(enemyType, nil)
	}

	// Update towers
	for _, tower := range g.Towers {
		if tower.Cooldown > 0 {
			tower.Cooldown--
		}

		if tower.CanAttack() {
			hitEnemies := tower.Attack(g.Enemies)
			for range hitEnemies {
				// Animate attack (future implementation)
			}
		}
	}

	// Update enemies
	for i := 0; i < len(g.Enemies); i++ {
		enemy := g.Enemies[i]

		// Check if enemy is dead
		if enemy.Health <= 0 {
			g.Resources["chatgpt"] += enemy.Reward
			g.Score["chatgpt"] += enemy.Reward
			g.Enemies = append(g.Enemies[:i], g.Enemies[i+1:]...)
			i--
			continue
		}

		// Move enemy along path
		enemy.DistanceMoved += enemy.Speed
		pathIndex := int(math.Min(float64(enemy.DistanceMoved), float64(len(g.Path)-1)))
		if pathIndex < len(g.Path) {
			enemy.Pos = g.Path[pathIndex]
		}

		// Check if enemy reached the end
		if pathIndex >= len(g.Path)-1 {
			g.Lives["chatgpt"]--
			g.Resources["gemini"] += enemy.Reward / 2
			g.Score["gemini"] += enemy.Reward
			g.Enemies = append(g.Enemies[:i], g.Enemies[i+1:]...)
			i--
		}
	}

	// Check win/lose conditions
	if g.Lives["chatgpt"] <= 0 {
		g.GameOver = true
		g.Winner = "gemini"
	}
}

func (g *Game) handleInput(ev tcell.Event) bool {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyEscape, tcell.KeyCtrlC:
			return false
		case tcell.KeyRune:
			switch ev.Rune() {
			case 'q', 'Q':
				return false
			case 'a', 'A':
				g.AIEnabled = !g.AIEnabled
			case '+', '=':
				g.GameSpeed = math.Max(0.05, g.GameSpeed*0.9)
			case '-', '_':
				g.GameSpeed = math.Min(1.0, g.GameSpeed*1.1)
			}
		}
	}
	return true
}

func (g *Game) Run() {
	// Game loop
	ticker := time.NewTicker(time.Duration(g.GameSpeed * float64(time.Second)))
	defer ticker.Stop()

	running := true
	for running {
		// Handle events
		for {
			ev := g.Screen.PollEvent()
			if ev == nil {
				break
			}
			running = g.handleInput(ev)
		}

		select {
		case <-ticker.C:
			// AI decision logic
			if !g.GameOver {
				g.handleAIDecisions()
				g.updateGameState()
				g.drawUI()
			} else {
				// Draw game over screen
				g.Screen.Clear()
				winner := "ChatGPT"
				if g.Winner == "gemini" {
					winner = "Gemini"
				}
				drawText(g.Screen, g.Width/2-10, g.Height/2, fmt.Sprintf("Game Over! %s wins!", winner), tcell.StyleDefault.Foreground(tcell.ColorYellow))
				drawText(g.Screen, g.Width/2-15, g.Height/2+2, "Press 'Q' to quit", tcell.StyleDefault)
				g.Screen.Show()
			}
		default:
			time.Sleep(time.Millisecond * 10)
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func main() {
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file, using environment variables")
	}

	// Load API keys
	openaiAPIKey = os.Getenv("OPENAI_API_KEY")
	googleAPIKey = os.Getenv("GOOGLE_API_KEY")

	if openaiAPIKey == "" || googleAPIKey == "" {
		fmt.Println("Error: OPENAI_API_KEY and GOOGLE_API_KEY must be set")
		fmt.Println("Create a .env file or set them as environment variables")
		os.Exit(1)
	}

	// Initialize screen
	screen, err := tcell.NewScreen()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating screen: %v\n", err)
		os.Exit(1)
	}

	if err := screen.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing screen: %v\n", err)
		os.Exit(1)
	}

	// Set screen options
	screen.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorBlack).
		Foreground(tcell.ColorWhite))
	screen.Clear()

	// Create and run game
	rand.Seed(time.Now().UnixNano())
	game := NewGame(screen)

	// Clean up
	defer func() {
		screen.Fini()
	}()

	// Show welcome screen
	screen.Clear()
	drawText(screen, game.Width/2-30, game.Height/2-5, "ChatGPT vs Gemini Tower Defense", tcell.StyleDefault.Foreground(tcell.ColorYellow))
	drawText(screen, game.Width/2-25, game.Height/2-3, "A battle of AI models!", tcell.StyleDefault)
	drawText(screen, game.Width/2-20, game.Height/2, "Controls:", tcell.StyleDefault.Foreground(tcell.ColorGreen))
	drawText(screen, game.Width/2-15, game.Height/2+2, "[Q] - Quit game", tcell.StyleDefault)
	drawText(screen, game.Width/2-15, game.Height/2+3, "[A] - Toggle AI", tcell.StyleDefault)
	drawText(screen, game.Width/2-15, game.Height/2+4, "[+/-] - Change game speed", tcell.StyleDefault)
	drawText(screen, game.Width/2-25, game.Height/2+7, "Press any key to start the game...", tcell.StyleDefault.Foreground(tcell.ColorYellow))
	screen.Show()

	// Wait for key press
	for {
		ev := screen.PollEvent()
		switch ev.(type) {
		case *tcell.EventKey:
			game.Run()
			return
		}
	}
}
