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

	char := t["char"].(rune)

	// Handle numeric values that could be either int or float64
	damage := toInt(t["damage"])
	maxCD := toInt(t["cooldown"])
	rangeVal := toInt(t["range"])
	cost := toInt(t["cost"])

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

// Helper function to convert interface{} to int
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
			// Apply damage and log it
			oldHealth := targets[i].enemy.Health
			targets[i].enemy.Health -= t.Damage
			fmt.Printf("Tower %s damaged enemy %s: %d → %d health\n", 
				t.TowerType, targets[i].enemy.EnemyType, oldHealth, targets[i].enemy.Health)
			hitEnemies = append(hitEnemies, targets[i].enemy)
		}
	} else {
		// Single target attack
		oldHealth := targets[0].enemy.Health
		targets[0].enemy.Health -= t.Damage
		fmt.Printf("Tower %s damaged enemy %s: %d → %d health\n", 
			t.TowerType, targets[0].enemy.EnemyType, oldHealth, targets[0].enemy.Health)
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
		"basic":  {"char": 'o', "health": float64(100), "speed": float64(1.0), "reward": float64(20)},
		"fast":   {"char": '>', "health": float64(50), "speed": float64(2.0), "reward": float64(15)},
		"tank":   {"char": '□', "health": float64(300), "speed": float64(0.5), "reward": float64(50)},
		"custom": {"char": '?', "health": float64(150), "speed": float64(1.2), "reward": float64(25)},
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
	fmt.Println("\n=== ChatGPT's Turn ===")
	fmt.Printf("Current resources: %d\n", gameState["resources"].(map[string]interface{})["chatgpt"].(int))
	fmt.Printf("Current towers: %d\n", len(gameState["towers"].([]interface{})))
	fmt.Printf("Current enemies: %d\n", len(gameState["enemies"].([]interface{})))

	prompt := h.createTowerPrompt(gameState)
	fmt.Println("Sending prompt to ChatGPT...")

	// Create request body
	reqBody := map[string]interface{}{
		"model": "gpt-4o-mini-2024-07-18",
		"messages": []map[string]interface{}{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
		"max_tokens":  150,
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Println("Error marshaling request:", err)
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqJSON))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+openaiAPIKey)
	req.Header.Set("Content-Type", "application/json")

	fmt.Println("Sending request to OpenAI API...")
	resp, err := h.Client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		fmt.Println("Error decoding response:", err)
		return nil, err
	}

	// Extract response content
	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		fmt.Println("No choices in response")
		return map[string]interface{}{"action": "none", "reason": "API response error"}, nil
	}

	choice := choices[0].(map[string]interface{})
	message := choice["message"].(map[string]interface{})
	content := message["content"].(string)
	fmt.Printf("ChatGPT response: %s\n", content)

	return h.parseTowerResponse(content)
}

func (h *OpenAIHandler) createTowerPrompt(gameState map[string]interface{}) string {
	enemies := gameState["enemies"].([]interface{})
	towers := gameState["towers"].([]interface{})
	resources := gameState["resources"].(map[string]interface{})["chatgpt"].(int)
	wave := gameState["wave"].(int)

	prompt := fmt.Sprintf(
		"You are playing a tower defense game as ChatGPT. You have %d resources and are on wave %d. "+
			"There are %d enemies and %d towers on the map.\n\n"+
			"Your goal is to defend your base by placing towers strategically. "+
			"CRITICAL: YOU MUST PLACE TOWERS IMMEDIATELY TO DEFEND AGAINST ENEMIES! "+
			"If you don't place towers, you will lose lives when enemies reach the end.\n\n"+
			"Available towers:\n"+
			"- basic (100): Balanced tower, good for early waves\n"+
			"- sniper (250): High damage, long range, good for strong enemies\n"+
			"- splash (200): Area damage, good for groups of weak enemies\n\n"+
			"Current wave: %d\n"+
			"Current resources: %d\n"+
			"Current towers: %d\n"+
			"Active enemies: %d\n\n"+
			"IMPORTANT: You MUST choose one of these actions NOW:\n"+
			"1. Place a new tower (specify type)\n"+
			"2. Save resources for a stronger tower (ONLY if you have a specific plan and already have some towers)\n\n"+
			"Respond ONLY in this exact JSON format: {\"action\": \"place\", \"tower_type\": \"basic\"}\n"+
			"Valid actions: place, save\n"+
			"Valid tower types: basic, sniper, splash",
		resources, wave, len(enemies), len(towers), wave, resources, len(towers), len(enemies),
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
			// Validate the decision
			action, hasAction := decision["action"].(string)
			if hasAction {
				if action == "place" {
					// Make sure there's a tower_type field
					towerType, hasTowerType := decision["tower_type"].(string)
					if !hasTowerType || towerType == "" {
						decision["tower_type"] = "basic" // Default to basic if missing
					}
					// Add position if missing
					if _, hasPos := decision["position"].([]interface{}); !hasPos {
						decision["position"] = []int{10, 10}
					}
					return decision, nil
				} else if action == "save" {
					return decision, nil
				}
			}
		}
	}

	// Fallback to basic parsing
	responseText := strings.ToLower(response)
	if strings.Contains(responseText, "place") {
		towerType := "basic"
		if strings.Contains(responseText, "sniper") {
			towerType = "sniper"
		} else if strings.Contains(responseText, "splash") {
			towerType = "splash"
		}
		return map[string]interface{}{
			"action":     "place",
			"tower_type": towerType,
			"position":   []int{10, 10},
			"reason":     "Extracted from text response",
		}, nil
	} else if strings.Contains(responseText, "save") {
		return map[string]interface{}{
			"action": "save",
			"reason": "Saving resources",
		}, nil
	}

	// Default to placing a basic tower
	return map[string]interface{}{
		"action":     "place",
		"tower_type": "basic",
		"position":   []int{10, 10},
		"reason":     "Default action - placing basic tower",
	}, nil
}

type GeminiHandler struct {
	*AIHandler
}

func (h *GeminiHandler) GetEnemyDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	fmt.Println("\n=== Gemini's Turn ===")
	fmt.Printf("Current resources: %d\n", gameState["resources"].(map[string]interface{})["gemini"].(int))
	fmt.Printf("Current wave: %d\n", gameState["wave"].(int))
	fmt.Printf("Current enemies: %d\n", len(gameState["enemies"].([]interface{})))

	prompt := h.createEnemyPrompt(gameState)
	fmt.Println("Sending prompt to Gemini...")

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
		fmt.Println("Error marshaling request:", err)
		return nil, err
	}

	req, err := http.NewRequest("POST",
		fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", googleAPIKey),
		bytes.NewBuffer(reqJSON))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	fmt.Println("Sending request to Gemini API...")
	resp, err := h.Client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		fmt.Println("Error decoding response:", err)
		return nil, err
	}

	// Extract response content
	candidates, ok := result["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		fmt.Println("No candidates in response")
		return map[string]interface{}{"action": "none", "reason": "API response error"}, nil
	}

	candidate := candidates[0].(map[string]interface{})
	content := candidate["content"].(map[string]interface{})
	parts := content["parts"].([]interface{})
	text := parts[0].(map[string]interface{})["text"].(string)
	fmt.Printf("Gemini response: %s\n", text)

	return h.parseEnemyResponse(text)
}

func (h *GeminiHandler) createEnemyPrompt(gameState map[string]interface{}) string {
	enemies := gameState["enemies"].([]interface{})
	towers := gameState["towers"].([]interface{})
	resources := gameState["resources"].(map[string]interface{})["gemini"].(int)
	wave := gameState["wave"].(int)

	prompt := fmt.Sprintf(
		"You are playing a tower defense game as Gemini. You have %d resources and are on wave %d. "+
			"There are %d active enemies and %d defensive towers.\n\n"+
			"Your goal is to overwhelm the opponent by sending enemies. "+
			"CRITICAL: YOU MUST SPAWN ENEMIES IMMEDIATELY TO ATTACK THE OPPONENT!\n\n"+
			"Available enemies and their costs:\n"+
			"- basic (20): Balanced enemy, good for early waves\n"+
			"- fast (30): Fast but weak, good for overwhelming\n"+
			"- tank (50): Slow but strong, good for late waves\n\n"+
			"Current wave: %d\n"+
			"Current resources: %d\n"+
			"Active enemies: %d\n"+
			"Defensive towers: %d\n\n"+
			"IMPORTANT: You MUST choose exactly one of these actions NOW:\n"+
			"1. Spawn a single enemy (specify type)\n"+
			"2. Launch a wave (costs 100 resources, sends multiple enemies)\n"+
			"3. Save resources (ONLY if you have a specific plan and already sent some enemies)\n\n"+
			"Respond ONLY in this exact JSON format: {\"action\": \"spawn\", \"enemy_type\": \"fast\"}\n"+
			"Valid actions are ONLY: \"spawn\", \"wave\", or \"save\"\n"+
			"Valid enemy types: \"basic\", \"fast\", or \"tank\"",
		resources, wave, len(enemies), len(towers), wave, resources, len(enemies), len(towers),
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
			// Explicitly check for valid action types
			action, hasAction := decision["action"].(string)
			if hasAction {
				if action == "spawn" {
					// Make sure there's an enemy_type field
					enemyType, hasEnemyType := decision["enemy_type"].(string)
					if !hasEnemyType || enemyType == "" {
						decision["enemy_type"] = "basic" // Default to basic if missing
					}
					return decision, nil
				} else if action == "wave" {
					return map[string]interface{}{
						"action": "wave",
						"reason": "Launching wave attack",
					}, nil
				} else if action == "save" {
					return map[string]interface{}{
						"action": "save",
						"reason": "Saving resources",
					}, nil
				}
			}
		}
	}

	// Fallback to basic parsing based on text content
	responseText := strings.ToLower(response)
	if strings.Contains(responseText, "spawn") {
		enemyType := "basic"
		if strings.Contains(responseText, "fast") {
			enemyType = "fast"
		} else if strings.Contains(responseText, "tank") {
			enemyType = "tank"
		}

		return map[string]interface{}{
			"action":     "spawn",
			"enemy_type": enemyType,
			"reason":     "Spawning enemy based on text analysis",
		}, nil
	} else if strings.Contains(responseText, "wave") {
		return map[string]interface{}{
			"action": "wave",
			"reason": "Launching wave based on text analysis",
		}, nil
	} else if strings.Contains(responseText, "save") {
		return map[string]interface{}{
			"action": "save",
			"reason": "Saving resources based on text analysis",
		}, nil
	}

	// Default to spawning a basic enemy
	return map[string]interface{}{
		"action":     "spawn",
		"enemy_type": "basic",
		"reason":     "Default action - spawning basic enemy",
	}, nil
}

// Game struct and methods
type Game struct {
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

	CurrentTurn    string // "chatgpt" or "gemini"
	LastActionTime time.Time
	MaxResources   int
	MaxWaves       int
	TurnTimeout    time.Duration
}

func NewGame() *Game {
	// Initialize with fixed dimensions
	width := 80
	height := 24
	mapHeight := height - 10

	game := &Game{
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
		AIDecisionInterval: map[string]int{"chatgpt": 2, "gemini": 2},
		LastAIDecision: map[string]time.Time{
			"chatgpt": time.Now(),
			"gemini":  time.Now(),
		},
		CurrentTurn:    "chatgpt", // ChatGPT goes first
		LastActionTime: time.Now(),
		MaxResources:   1000,             // Maximum resources per player
		MaxWaves:       50,               // Maximum number of waves before game ends
		TurnTimeout:    30 * time.Second, // Timeout for each turn
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

func (g *Game) handleAIDecisions() {
	currentTime := time.Now()
	gameState := g.getGameState()

	// Check if game should end due to timeout or max waves
	if currentTime.Sub(g.LastActionTime) > g.TurnTimeout {
		fmt.Println("\n=== Game Over! ===")
		fmt.Println("Game ended due to inactivity")
		g.GameOver = true
		g.Winner = "none"
		return
	}

	// Print turn information at the start of each turn
	if !g.AIThinking["chatgpt"] && !g.AIThinking["gemini"] {
		fmt.Printf("\n=== Current Turn: %s ===\n", g.CurrentTurn)
	}

	// Only allow AI to make a move if it's their turn
	if g.CurrentTurn == "chatgpt" && !g.AIThinking["chatgpt"] && g.AIEnabled {
		fmt.Println("\n=== ChatGPT's Turn ===")
		g.AIThinking["chatgpt"] = true
		g.LastActionTime = currentTime

		go func() {
			decision, err := g.OpenAIHandler.GetTowerDecision(gameState)
			if err == nil {
				action, _ := decision["action"].(string)
				fmt.Printf("ChatGPT decided to: %s\n", action)

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
						fmt.Printf("Successfully placed %s tower\n", towerType)
						g.LastDecisions["chatgpt"] = fmt.Sprintf("Placed %s tower", towerType)
						g.CurrentTurn = "gemini" // Switch to Gemini's turn
					} else {
						fmt.Printf("Failed to place %s tower\n", towerType)
						g.LastDecisions["chatgpt"] = fmt.Sprintf("Failed to place %s tower", towerType)
						g.CurrentTurn = "gemini" // Still switch turns even on failure
					}
				} else {
					fmt.Println("ChatGPT decided to save resources")
					g.LastDecisions["chatgpt"] = "Saving resources"
					g.CurrentTurn = "gemini" // Switch to Gemini's turn
				}
			} else {
				fmt.Printf("ChatGPT API error: %v\n", err)
				g.LastDecisions["chatgpt"] = "API error"
				g.CurrentTurn = "gemini" // Switch to Gemini's turn on error
			}

			g.AIThinking["chatgpt"] = false
		}()
	} else if g.CurrentTurn == "gemini" && !g.AIThinking["gemini"] && g.AIEnabled {
		fmt.Println("\n=== Gemini's Turn ===")
		g.AIThinking["gemini"] = true
		g.LastActionTime = currentTime

		go func() {
			decision, err := g.GeminiHandler.GetEnemyDecision(gameState)
			if err == nil {
				action, _ := decision["action"].(string)
				fmt.Printf("Gemini decided to: %s\n", action)

				if action == "spawn" {
					enemyType, _ := decision["enemy_type"].(string)
					if enemyType == "" {
						enemyType = "basic"
					}

					fmt.Printf("Attempting to spawn %s enemy\n", enemyType)
					spawned := g.spawnEnemy(enemyType, nil)
					if spawned {
						fmt.Printf("Successfully spawned %s enemy\n", enemyType)
						g.LastDecisions["gemini"] = fmt.Sprintf("Spawned %s enemy", enemyType)
						g.CurrentTurn = "chatgpt" // Switch to ChatGPT's turn
					} else {
						fmt.Printf("Failed to spawn %s enemy (not enough resources or invalid type)\n", enemyType)
						g.LastDecisions["gemini"] = fmt.Sprintf("Failed to spawn %s enemy", enemyType)
						g.CurrentTurn = "chatgpt" // Still switch turns even on failure
					}
				} else if action == "wave" {
					waveCost := g.Wave * 50 // Make sure this matches the cost in spawnWave
					fmt.Printf("Attempting to launch wave %d (cost: %d, available: %d)\n",
						g.Wave, waveCost, g.Resources["gemini"])

					if g.spawnWave() {
						fmt.Printf("Wave %d launched successfully\n", g.Wave)
						g.LastDecisions["gemini"] = fmt.Sprintf("Launched wave %d", g.Wave)
						g.CurrentTurn = "chatgpt" // Switch to ChatGPT's turn
					} else {
						fmt.Printf("Failed to launch wave (not enough resources, need %d)\n", waveCost)
						g.LastDecisions["gemini"] = "Failed to launch wave (not enough resources)"
						g.CurrentTurn = "chatgpt" // Still switch turns even on failure
					}
				} else if action == "save" {
					fmt.Println("Gemini decided to save resources")
					g.LastDecisions["gemini"] = "Saving resources"
					g.CurrentTurn = "chatgpt" // Switch to ChatGPT's turn
				} else {
					fmt.Printf("Gemini made an invalid decision: %s\n", action)
					g.LastDecisions["gemini"] = "Invalid decision"
					g.CurrentTurn = "chatgpt" // Switch to ChatGPT's turn on error
				}
			} else {
				fmt.Printf("Gemini API error: %v\n", err)
				g.LastDecisions["gemini"] = "API error"
				g.CurrentTurn = "chatgpt" // Switch to ChatGPT's turn on error
			}

			g.AIThinking["gemini"] = false
		}()
	}
}

func (g *Game) updateGameState() {
	// Check if we've reached max waves
	if g.Wave > g.MaxWaves {
		fmt.Println("\n=== Game Over! ===")
		fmt.Println("Game ended - maximum waves reached")
		g.GameOver = true
		
		// Determine winner based on score
		if g.Score["chatgpt"] > g.Score["gemini"] {
			g.Winner = "chatgpt"
			fmt.Printf("ChatGPT wins with score %d vs Gemini's %d!\n", 
				g.Score["chatgpt"], g.Score["gemini"])
		} else if g.Score["gemini"] > g.Score["chatgpt"] {
			g.Winner = "gemini"
			fmt.Printf("Gemini wins with score %d vs ChatGPT's %d!\n", 
				g.Score["gemini"], g.Score["chatgpt"])
		} else {
			g.Winner = "tie"
			fmt.Printf("Game ended in a tie! Both scores: %d\n", g.Score["chatgpt"])
		}
		return
	}

	// Cap resources at maximum
	if g.Resources["chatgpt"] > g.MaxResources {
		g.Resources["chatgpt"] = g.MaxResources
	}
	if g.Resources["gemini"] > g.MaxResources {
		g.Resources["gemini"] = g.MaxResources
	}

	// Process wave queue - spawn enemies from queue
	if len(g.WaveQueue) > 0 && len(g.Enemies) < 20 {
		enemyType := g.WaveQueue[0]
		g.WaveQueue = g.WaveQueue[1:]

		// Create enemy from queue without reducing resources
		// (resources were already deducted when the wave was created)
		if len(g.Path) > 0 {
			startPos := g.Path[0]
			enemy := NewEnemy(startPos.Y, startPos.X, enemyType, nil)
			g.Enemies = append(g.Enemies, &enemy)
			fmt.Printf("Spawned enemy from wave queue: %s (Health: %d, Speed: %.1f)\n",
				enemyType, enemy.Health, enemy.Speed)
		}
	}

	// Auto-progress wave if no enemies and no wave queue, but only if both AIs have made at least one decision
	if len(g.Enemies) == 0 && len(g.WaveQueue) == 0 && !g.AIThinking["chatgpt"] && !g.AIThinking["gemini"] {
		// Only progress if both AIs have had a chance to act in this wave
		if g.LastDecisions["chatgpt"] != "None" && g.LastDecisions["gemini"] != "None" {
			// Reset decision tracking for the new wave
			g.LastDecisions["chatgpt"] = "None"
			g.LastDecisions["gemini"] = "None"
			
			g.Wave++
			fmt.Printf("\n=== Wave %d Starting ===\n", g.Wave)
			// Give resources to both players at the start of each wave
			baseResourceAmount := 50
			// Scale resources with wave number for better late-game balance
			waveBonus := int(math.Min(float64(g.Wave), 20.0)) * 5 // Cap at wave 20
			resourceAmount := baseResourceAmount + waveBonus
			
			g.Resources["chatgpt"] += resourceAmount
			g.Resources["gemini"] += resourceAmount
			fmt.Printf("Resources added - ChatGPT: +%d, Gemini: +%d\n", resourceAmount, resourceAmount)

			// For every 10th wave, give bonus resources
			if g.Wave%10 == 0 {
				bonusAmount := 100 + (g.Wave / 10) * 50 // Scales with wave number
				g.Resources["chatgpt"] += bonusAmount
				g.Resources["gemini"] += bonusAmount
				fmt.Printf("BONUS resources for wave %d - ChatGPT: +%d, Gemini: +%d\n",
					g.Wave, bonusAmount, bonusAmount)
			}
			
			// Check if we've reached max waves after incrementing
			if g.Wave > g.MaxWaves {
				fmt.Println("\n=== Game Over! ===")
				fmt.Println("Game ended - maximum waves reached")
				g.GameOver = true
				
				// Determine winner based on score
				if g.Score["chatgpt"] > g.Score["gemini"] {
					g.Winner = "chatgpt"
					fmt.Printf("ChatGPT wins with score %d vs Gemini's %d!\n", 
						g.Score["chatgpt"], g.Score["gemini"])
				} else if g.Score["gemini"] > g.Score["chatgpt"] {
					g.Winner = "gemini"
					fmt.Printf("Gemini wins with score %d vs ChatGPT's %d!\n", 
						g.Score["gemini"], g.Score["chatgpt"])
				} else {
					g.Winner = "tie"
					fmt.Printf("Game ended in a tie! Both scores: %d\n", g.Score["chatgpt"])
				}
				return
			}
			
			// Reset turn to ChatGPT at the start of each wave
			g.CurrentTurn = "chatgpt"
			fmt.Println("Turn reset to ChatGPT at the start of the new wave")
			// Reset the last action time to prevent timeout
			g.LastActionTime = time.Now()
		}
	}

	// Update towers
	for _, tower := range g.Towers {
		if tower.Cooldown > 0 {
			tower.Cooldown--
		}

		if tower.CanAttack() {
			hitEnemies := tower.Attack(g.Enemies)
			if len(hitEnemies) > 0 {
				fmt.Printf("Tower %s attacked %d enemies\n", tower.TowerType, len(hitEnemies))
			}
		}
	}

	// Update enemies
	for i := 0; i < len(g.Enemies); i++ {
		enemy := g.Enemies[i]

		// Check if enemy is dead
		if enemy.Health <= 0 {
			fmt.Printf("Enemy %s died, reward: %d\n", enemy.EnemyType, enemy.Reward)
			g.Resources["chatgpt"] += enemy.Reward
			g.Score["chatgpt"] += enemy.Reward
			g.Enemies = append(g.Enemies[:i], g.Enemies[i+1:]...)
			i--
			continue
		}

		// Move enemy along path
		enemy.DistanceMoved += enemy.Speed
		pathIndex := int(enemy.DistanceMoved)
		
		// Make sure we don't go beyond the path length
		if pathIndex >= len(g.Path) {
			// Enemy reached the end
			fmt.Printf("Enemy %s reached the end, lives lost: 1\n", enemy.EnemyType)
			g.Lives["chatgpt"]--
			g.Resources["gemini"] += enemy.Reward / 2
			g.Score["gemini"] += enemy.Reward
			g.Enemies = append(g.Enemies[:i], g.Enemies[i+1:]...)
			i--
		} else {
			// Update enemy position to current path position
			enemy.Pos = g.Path[pathIndex]
		}
	}

	// Check win/lose conditions
	if g.Lives["chatgpt"] <= 0 {
		fmt.Println("\n=== Game Over! ===")
		fmt.Printf("Gemini wins! ChatGPT lost all lives. Final score - ChatGPT: %d, Gemini: %d\n",
			g.Score["chatgpt"], g.Score["gemini"])
		g.GameOver = true
		g.Winner = "gemini"
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
	// Validate tower type
	validTypes := map[string]bool{
		"basic":  true,
		"sniper": true,
		"splash": true,
		"custom": true,
	}
	
	if !validTypes[towerType] {
		fmt.Printf("Invalid tower type: %s\n", towerType)
		return false
	}
	
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
	cost, exists := towerCosts[towerType]
	if !exists {
		cost = 100 // Default to basic cost if type not found
	}
	
	fmt.Printf("Attempting to place %s tower (cost: %d, available: %d)\n",
		towerType, cost, g.Resources["chatgpt"])
		
	if g.Resources["chatgpt"] < cost {
		fmt.Printf("Not enough resources to place %s tower\n", towerType)
		return false
	}

	// Place tower
	tower := NewTower(y, x, towerType, nil)
	g.Towers = append(g.Towers, &tower)
	g.Resources["chatgpt"] -= tower.Cost
	return true
}

func (g *Game) spawnEnemy(enemyType string, params map[string]interface{}) bool {
	// Validate the enemy type
	validTypes := map[string]bool{
		"basic":  true,
		"fast":   true,
		"tank":   true,
		"custom": true,
	}

	if !validTypes[enemyType] {
		fmt.Printf("Invalid enemy type: %s\n", enemyType)
		return false
	}

	enemyCosts := map[string]int{"basic": 20, "fast": 30, "tank": 50, "custom": 40}
	cost := enemyCosts[enemyType]

	fmt.Printf("Attempting to spawn %s enemy (cost: %d, available: %d)\n",
		enemyType, cost, g.Resources["gemini"])

	if g.Resources["gemini"] < cost {
		fmt.Printf("Not enough resources to spawn %s enemy\n", enemyType)
		return false
	}

	// Get starting position (beginning of path)
	if len(g.Path) == 0 {
		fmt.Println("No path defined for enemies")
		return false
	}

	startPos := g.Path[0]

	// Create and add enemy
	enemy := NewEnemy(startPos.Y, startPos.X, enemyType, params)
	g.Enemies = append(g.Enemies, &enemy)
	g.Resources["gemini"] -= cost

	fmt.Printf("Enemy spawned - Type: %s, Health: %d, Speed: %.1f, Reward: %d\n",
		enemy.EnemyType, enemy.Health, enemy.Speed, enemy.Reward)

	return true
}

func (g *Game) spawnWave() bool {
	// Calculate wave cost based on current wave with a more balanced formula
	// Base cost + scaling factor that increases more slowly in later waves
	baseCost := 50
	scalingFactor := int(math.Sqrt(float64(g.Wave)) * 10)
	waveCost := baseCost + scalingFactor
	
	// Cap the maximum cost to prevent it from becoming too expensive
	if waveCost > 300 {
		waveCost = 300
	}

	fmt.Printf("Attempting to launch wave %d (cost: %d, available: %d)\n",
		g.Wave, waveCost, g.Resources["gemini"])

	if g.Resources["gemini"] < waveCost {
		fmt.Printf("Not enough resources to launch wave %d\n", g.Wave)
		return false
	}

	// Create a mix of enemies based on the current wave
	numEnemies := g.Wave*2 + 5
	if numEnemies > 30 {
		numEnemies = 30 // Cap to avoid extremely large waves
	}

	fmt.Printf("Creating wave with %d enemies\n", numEnemies)

	enemyTypes := make([]string, 0)

	// More varied waves as game progresses
	if g.Wave < 5 {
		// Early waves: Mostly basic enemies
		basicCount := int(float64(numEnemies) * 0.8)
		fastCount := numEnemies - basicCount

		for i := 0; i < basicCount; i++ {
			enemyTypes = append(enemyTypes, "basic")
		}
		for i := 0; i < fastCount; i++ {
			enemyTypes = append(enemyTypes, "fast")
		}

	} else if g.Wave < 15 {
		// Mid waves: Mix of basic and fast enemies with a few tanks
		basicCount := int(float64(numEnemies) * 0.5)
		fastCount := int(float64(numEnemies) * 0.4)
		tankCount := numEnemies - basicCount - fastCount

		for i := 0; i < basicCount; i++ {
			enemyTypes = append(enemyTypes, "basic")
		}
		for i := 0; i < fastCount; i++ {
			enemyTypes = append(enemyTypes, "fast")
		}
		for i := 0; i < tankCount; i++ {
			enemyTypes = append(enemyTypes, "tank")
		}

	} else {
		// Late waves: Even mix with more tanks
		basicCount := int(float64(numEnemies) * 0.3)
		fastCount := int(float64(numEnemies) * 0.3)
		tankCount := numEnemies - basicCount - fastCount

		for i := 0; i < basicCount; i++ {
			enemyTypes = append(enemyTypes, "basic")
		}
		for i := 0; i < fastCount; i++ {
			enemyTypes = append(enemyTypes, "fast")
		}
		for i := 0; i < tankCount; i++ {
			enemyTypes = append(enemyTypes, "tank")
		}
	}

	// Shuffle the types for variety
	rand.Shuffle(len(enemyTypes), func(i, j int) {
		enemyTypes[i], enemyTypes[j] = enemyTypes[j], enemyTypes[i]
	})

	fmt.Printf("Wave composition: %d total enemies (%d in queue before adding)\n",
		len(enemyTypes), len(g.WaveQueue))

	// Add to wave queue
	g.WaveQueue = append(g.WaveQueue, enemyTypes...)

	// Subtract cost but DO NOT increment wave
	// Wave only increments when all enemies are cleared
	g.Resources["gemini"] -= waveCost

	fmt.Printf("Wave %d launched successfully with %d enemies in queue\n",
		g.Wave, len(g.WaveQueue))

	return true
}

func (g *Game) Run() {
	fmt.Println("\n=== Game Started ===")
	fmt.Printf("Initial resources - ChatGPT: %d, Gemini: %d\n", g.Resources["chatgpt"], g.Resources["gemini"])
	fmt.Printf("Initial lives - ChatGPT: %d\n", g.Lives["chatgpt"])
	fmt.Printf("Game speed: %.2f\n", g.GameSpeed)
	fmt.Printf("AI decision intervals - ChatGPT: %ds, Gemini: %ds\n",
		g.AIDecisionInterval["chatgpt"], g.AIDecisionInterval["gemini"])
	fmt.Println("================================\n")

	// Game loop
	ticker := time.NewTicker(time.Duration(g.GameSpeed * float64(time.Second)))
	defer ticker.Stop()

	running := true
	for running {
		select {
		case <-ticker.C:
			if !g.GameOver {
				g.handleAIDecisions()
				g.updateGameState()

				// Print current game state
				fmt.Printf("\n=== Game State ===\n")
				fmt.Printf("Wave: %d\n", g.Wave)
				fmt.Printf("Current Turn: %s\n", g.CurrentTurn)
				fmt.Printf("ChatGPT - Lives: %d, Resources: %d, Score: %d\n",
					g.Lives["chatgpt"], g.Resources["chatgpt"], g.Score["chatgpt"])
				fmt.Printf("Gemini - Resources: %d, Score: %d\n",
					g.Resources["gemini"], g.Score["gemini"])
				fmt.Printf("Active Towers: %d, Active Enemies: %d\n",
					len(g.Towers), len(g.Enemies))
				fmt.Printf("Wave Queue: %d enemies\n", len(g.WaveQueue))
				fmt.Printf("Time since last action: %.1f seconds\n", time.Since(g.LastActionTime).Seconds())
				fmt.Println("==================\n")
			} else {
				fmt.Println("\n=== Game Over! ===")
				winner := "ChatGPT"
				if g.Winner == "gemini" {
					winner = "Gemini"
				}
				fmt.Printf("%s wins!\n", winner)
				fmt.Printf("Final scores - ChatGPT: %d, Gemini: %d\n",
					g.Score["chatgpt"], g.Score["gemini"])
				running = false
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

	fmt.Println("API keys loaded successfully")

	// Create and run game
	rand.Seed(time.Now().UnixNano())
	game := NewGame()
	game.Run()
}
