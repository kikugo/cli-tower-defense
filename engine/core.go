package engine

import (
	"bytes"
	"encoding/json"
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
	rng    *rand.Rand
}

func NewAIHandler(rng *rand.Rand) *AIHandler {
	return &AIHandler{
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
		rng: rng,
	}
}

type OpenAIHandler struct {
	*AIHandler
	APIKey string
}

func (h *OpenAIHandler) GetTowerDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	fmt.Println("\n=== ChatGPT's Turn ===")

	// Extract resources with backward-compat handling for both map[string]interface{} and map[string]int.
	curRes := 0
	switch resAny := gameState["resources"].(type) {
	case map[string]interface{}:
		if v, ok := resAny["chatgpt"].(int); ok {
			curRes = v
		}
	case map[string]int:
		curRes = resAny["chatgpt"]
	}
	fmt.Printf("Current resources: %d\n", curRes)
	fmt.Printf("Current towers: %d\n", len(gameState["towers"].([]interface{})))
	fmt.Printf("Current enemies: %d\n", len(gameState["enemies"].([]interface{})))

	prompt := h.createTowerPrompt(gameState)
	fmt.Println("Sending prompt to ChatGPT...")

	// Create request body
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
		fmt.Println("Error marshaling request:", err)
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqJSON))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+h.APIKey)
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
	// Safely extract values with type assertions and fallbacks
	enemies, _ := gameState["enemies"].([]interface{})
	towers, _ := gameState["towers"].([]interface{})
	wave := 1
	resources := 300
	lives := 20

	// Extract wave
	if waveVal, ok := gameState["wave"].(int); ok {
		wave = waveVal
	}

	// Extract resources safely
	if resourcesMap, ok := gameState["resources"].(map[string]interface{}); ok {
		if chatgptResources, ok := resourcesMap["chatgpt"].(int); ok {
			resources = chatgptResources
		}
	}

	// Extract lives safely
	if livesMap, ok := gameState["lives"].(map[string]interface{}); ok {
		if chatgptLives, ok := livesMap["chatgpt"].(int); ok {
			lives = chatgptLives
		}
	}

	// Count enemy types to provide better strategic information
	basicCount, fastCount, tankCount := 0, 0, 0
	for _, enemy := range enemies {
		if enemyMap, ok := enemy.(map[string]interface{}); ok {
			if enemyType, ok := enemyMap["type"].(string); ok {
				switch eType := enemyType; eType {
				case "basic":
					basicCount++
				case "fast":
					fastCount++
				case "tank":
					tankCount++
				}
			}
		}
	}

	// Count tower types for better decision making
	basicTowers, sniperTowers, splashTowers := 0, 0, 0
	for _, tower := range towers {
		if towerMap, ok := tower.(map[string]interface{}); ok {
			if towerType, ok := towerMap["type"].(string); ok {
				switch tType := towerType; tType {
				case "basic":
					basicTowers++
				case "sniper":
					sniperTowers++
				case "splash":
					splashTowers++
				}
			}
		}
	}

	// Calculate what tower we need most based on current wave, enemies, and resources
	var recommendedTower string

	// First determine the strategically best tower based on enemies and wave
	if tankCount > 2 || wave > 15 {
		recommendedTower = "sniper"
	} else if fastCount > 3 || wave > 8 {
		recommendedTower = "splash"
	} else {
		recommendedTower = "basic"
	}

	// Then adjust based on available resources
	towerCosts := map[string]string{
		"basic":  "100",
		"splash": "200",
		"sniper": "250",
	}

	// Now check if we can afford the recommended tower
	if recommendedTower == "sniper" && resources < 250 {
		if resources >= 200 {
			recommendedTower = "splash"
		} else {
			recommendedTower = "basic"
		}
	} else if recommendedTower == "splash" && resources < 200 {
		recommendedTower = "basic"
	}

	// Determine affordability message based on resources
	var affordabilityMsg string
	if resources < 100 {
		affordabilityMsg = "You don't have enough resources for any tower right now. You will need to save for your next turn."
	} else if resources < 200 {
		affordabilityMsg = "You can only afford a basic tower right now."
	} else if resources < 250 {
		affordabilityMsg = "You can afford a basic or splash tower right now."
	} else {
		affordabilityMsg = "You can afford any tower type."
	}

	// Expanded position options covering more of the map
	positionOptions := [][]int{
		{2, 2}, {2, 15}, {2, 25}, {2, 35}, {2, 45}, {2, 55}, {2, 65}, {2, 75},
		{5, 5}, {5, 15}, {5, 25}, {5, 35}, {5, 45}, {5, 55}, {5, 65}, {5, 75},
		{8, 5}, {8, 15}, {8, 25}, {8, 35}, {8, 45}, {8, 55}, {8, 65}, {8, 75},
		{12, 5}, {12, 15}, {12, 25}, {12, 35}, {12, 45}, {12, 55}, {12, 65}, {12, 75},
		{16, 5}, {16, 15}, {16, 25}, {16, 35}, {16, 45}, {16, 55}, {16, 65}, {16, 75},
		{20, 5}, {20, 15}, {20, 25}, {20, 35}, {20, 45}, {20, 55}, {20, 65}, {20, 75},
	}

	// Choose random position from options
	randomPos := positionOptions[h.rng.Intn(len(positionOptions))]

	// Determine example position based on number of existing towers
	examplePos := randomPos
	if len(towers) == 0 {
		// First tower - place at a corner for good coverage
		examplePos = []int{2, 2}
	} else if len(towers) == 1 {
		// Second tower - place at opposite corner
		examplePos = []int{2, 75}
	} else if len(towers)%2 == 0 {
		// Even towers - try top half
		examplePos = []int{2 + h.rng.Intn(5), 10 + h.rng.Intn(60)}
	} else {
		// Odd towers - try bottom half
		examplePos = []int{15 + h.rng.Intn(5), 10 + h.rng.Intn(60)}
	}

	prompt := fmt.Sprintf(
		"You are playing a tower defense game as ChatGPT. You have %d resources, %d lives, and are on wave %d.\n\n"+
			"CRITICAL: You MUST place towers aggressively to defend! Only choose tower types you can afford.\n\n"+
			"Enemy Analysis:\n"+
			"- Current Wave: %d\n"+
			"- Active Enemies: %d basic, %d fast, %d tank enemies\n"+
			"- Your Defense: %d basic, %d sniper, %d splash towers\n\n"+
			"Tower Options (cost):\n"+
			"- basic (%s): Good all-around, effective early game\n"+
			"- sniper (%s): High damage, excellent against tanks and late waves\n"+
			"- splash (%s): Area damage, optimal against groups of fast enemies\n\n"+
			"Current status:\n"+
			"- Resources: %d\n"+
			"- Lives: %d\n\n"+
			"AFFORDABILITY: %s\n\n"+
			"POSITIONING STRATEGY:\n"+
			"- If this is your first tower, try placing at corners or edges of map (like [2,2] or [2,75])\n"+
			"- IMPORTANT: The path validation has been improved! You can now place towers MUCH closer to the path!\n"+
			"- We now allow towers to be placed right next to each other, so you can build dense defensive clusters\n"+
			"- Good positions include: [2,2], [5,5], [2,75], [5,75], [20,20], [20,60], etc.\n"+
			"- If unsure, place near coordinates: [%d,%d] or try one of the built-in strategic positions\n"+
			"- Example strategic position: [%d,%d]\n\n"+
			"RESPONSE INSTRUCTIONS:\n"+
			"1. If wave > 15 AND you can afford it: Choose sniper towers\n"+
			"2. If fast enemies present AND you can afford it: Choose splash towers\n"+
			"3. If resources < 200: Choose basic towers\n"+
			"4. Recommended tower for current situation: %s\n\n"+
			"Respond ONLY in this exact JSON format: {\"action\": \"place\", \"tower_type\": \"%s\", \"position\": [%d, %d]}\n"+
			"Valid tower types: \"basic\", \"sniper\", \"splash\"\n"+
			"IMPORTANT: ALWAYS place a tower if you have resources. NEVER request a tower type you cannot afford.",
		resources, lives, wave,
		wave,
		basicCount, fastCount, tankCount,
		basicTowers, sniperTowers, splashTowers,
		towerCosts["basic"], towerCosts["sniper"], towerCosts["splash"],
		resources, lives,
		affordabilityMsg,
		randomPos[0], randomPos[1],
		examplePos[0], examplePos[1],
		recommendedTower,
		recommendedTower,             // Default to the recommended tower type in the example JSON
		examplePos[0], examplePos[1], // Use strategic position in example
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
					// Convert "save" to "place" action if it's an early wave or we have enough resources
					// This makes the AI more aggressive about tower placement
					return map[string]interface{}{
						"action":     "place",
						"tower_type": "basic",
						"position":   []int{10, 10},
						"reason":     "Converted save to place for better defense",
					}, nil
				}
			}
		}
	}

	// Fallback to basic parsing - prioritize placing towers over saving
	responseText := strings.ToLower(response)
	if strings.Contains(responseText, "place") || strings.Contains(responseText, "tower") ||
		strings.Contains(responseText, "sniper") || strings.Contains(responseText, "splash") ||
		strings.Contains(responseText, "basic") {
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
			"reason":     "Extracted tower type from text response",
		}, nil
	}

	// Default to placing a basic tower in almost all cases
	return map[string]interface{}{
		"action":     "place",
		"tower_type": "basic",
		"position":   []int{10, 10},
		"reason":     "Default action - placing basic tower for defense",
	}, nil
}

type GeminiHandler struct {
	*AIHandler
	APIKey string
}

func (h *GeminiHandler) GetEnemyDecision(gameState map[string]interface{}) (map[string]interface{}, error) {
	fmt.Println("\n=== Gemini's Turn ===")

	// Extract resources for decision making
	var resources int = 0
	if resourcesMap, ok := gameState["resources"].(map[string]interface{}); ok {
		if geminiResources, ok := resourcesMap["gemini"].(int); ok {
			resources = geminiResources
		}
	}

	// Extract wave number
	var wave int = 1
	if waveVal, ok := gameState["wave"].(int); ok {
		wave = waveVal
	}

	fmt.Printf("Current resources: %d\n", resources)
	fmt.Printf("Current wave: %d\n", wave)
	fmt.Printf("Current enemies: %d\n", len(gameState["enemies"].([]interface{})))

	// Make direct decision based on resources for efficiency if high resources
	if resources >= 200 {
		fmt.Println("Skipping API call - Resources sufficient for wave launch")
		return map[string]interface{}{
			"action": "wave",
			"reason": "Auto-decision: High resources available for wave launch",
		}, nil
	}

	prompt := h.createEnemyPrompt(gameState)
	fmt.Println("Sending prompt to Gemini...")

	// Create request body with proper structure
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
		// Return fallback decision on error
		return getFallbackEnemyDecision(resources), nil
	}

	req, err := http.NewRequest("POST",
		fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=%s", h.APIKey),
		bytes.NewBuffer(reqJSON))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return getFallbackEnemyDecision(resources), nil
	}

	req.Header.Set("Content-Type", "application/json")

	fmt.Println("Sending request to Gemini API...")
	resp, err := h.Client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return getFallbackEnemyDecision(resources), nil
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		fmt.Println("Error decoding response:", err)
		return getFallbackEnemyDecision(resources), nil
	}

	// Extract response content
	candidates, ok := result["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		fmt.Println("No candidates in response")
		return getFallbackEnemyDecision(resources), nil
	}

	candidate := candidates[0].(map[string]interface{})
	content := candidate["content"].(map[string]interface{})
	parts := content["parts"].([]interface{})
	text := parts[0].(map[string]interface{})["text"].(string)
	fmt.Printf("Gemini response: %s\n", text)

	return h.parseEnemyResponse(text)
}

func (h *GeminiHandler) createEnemyPrompt(gameState map[string]interface{}) string {
	// Safely extract values with type assertions and fallbacks
	enemies, _ := gameState["enemies"].([]interface{})
	towers, _ := gameState["towers"].([]interface{})
	wave := 1
	resources := 300
	lives := 20

	// Extract wave
	if waveVal, ok := gameState["wave"].(int); ok {
		wave = waveVal
	}

	// Extract resources safely
	if resourcesMap, ok := gameState["resources"].(map[string]interface{}); ok {
		if geminiResources, ok := resourcesMap["gemini"].(int); ok {
			resources = geminiResources
		}
	}

	// Extract lives safely
	if livesMap, ok := gameState["lives"].(map[string]interface{}); ok {
		if geminiLives, ok := livesMap["gemini"].(int); ok {
			lives = geminiLives
		}
	}

	// Count tower types to provide better strategic information
	basicTowers, sniperTowers, splashTowers := 0, 0, 0
	for _, tower := range towers {
		if towerMap, ok := tower.(map[string]interface{}); ok {
			if towerType, ok := towerMap["type"].(string); ok {
				switch tType := towerType; tType {
				case "basic":
					basicTowers++
				case "sniper":
					sniperTowers++
				case "splash":
					splashTowers++
				}
			}
		}
	}

	// Calculate wave cost with more affordable values to encourage spawning
	waveCost := 40 + (wave * 5)
	if waveCost > 200 {
		waveCost = 200 // Cap at 200 to ensure it's affordable
	}

	prompt := fmt.Sprintf(
		"You are playing a tower defense game as Gemini. You have %d resources, %d lives, and are on wave %d.\n\n"+
			"IMPORTANT: Your goal is to AGGRESSIVELY send enemies to overwhelm the opponent. You MUST spawn multiple enemies every turn!\n\n"+
			"Strategic Analysis:\n"+
			"- Opponent's Defense: %d basic, %d sniper, %d splash towers\n"+
			"- Active enemies: %d\n\n"+
			"Enemy Options (cost) - CHOOSE ONE NOW:\n"+
			"- basic (20): Good value basic enemy\n"+
			"- fast (30): Excellent against snipers\n"+
			"- tank (50): Strong against all towers\n"+
			"- wave (%d): Launches multiple enemies at once (best value)\n\n"+
			"Current status:\n"+
			"- Wave: %d\n"+
			"- Resources: %d\n\n"+
			"RESPONSE INSTRUCTIONS:\n"+
			"1. If you have 200+ resources: ALWAYS launch a wave\n"+
			"2. If you have 50+ resources: Spawn a tank enemy\n"+
			"3. Otherwise: Spawn a basic or fast enemy\n"+
			"Respond ONLY in this JSON format: {\"action\": \"spawn\", \"enemy_type\": \"fast\"}\n"+
			"Valid actions: \"spawn\" or \"wave\" ONLY\n"+
			"Valid enemy types: \"basic\", \"fast\", or \"tank\"",
		resources, lives, wave,
		basicTowers, sniperTowers, splashTowers, len(enemies),
		waveCost,
		wave, resources,
	)
	return prompt
}

func (h *GeminiHandler) parseEnemyResponse(response string) (map[string]interface{}, error) {
	// Handle empty response
	if response == "" {
		// Default to spawn action with basic enemy when no response
		fmt.Println("Empty response from Gemini API, using fallback action")
		return map[string]interface{}{
			"action":     "spawn",
			"enemy_type": "basic",
			"reason":     "Fallback due to empty API response",
		}, nil
	}

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
					// Only allow saving if resources are low and there are no enemies
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
	if strings.Contains(responseText, "wave") {
		return map[string]interface{}{
			"action": "wave",
			"reason": "Launching wave based on text analysis",
		}, nil
	} else if strings.Contains(responseText, "spawn") {
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
	} else if strings.Contains(responseText, "save") {
		return map[string]interface{}{
			"action":     "spawn", // Convert save to spawn for more aggressive gameplay
			"enemy_type": "basic",
			"reason":     "Converted save action to spawn basic",
		}, nil
	}

	// Make decision based on available resources
	if len(responseText) > 0 {
		fmt.Printf("Couldn't parse response '%s', using resource-based fallback\n", responseText)
	} else {
		fmt.Println("Empty or invalid response, using resource-based fallback")
	}

	// Default to spawning a basic enemy or wave based on available resources
	return map[string]interface{}{
		"action":     "spawn",
		"enemy_type": "basic",
		"reason":     "Default action - spawning basic enemy",
	}, nil
}

// Helper function to get a reasonable fallback decision based on resources
func getFallbackEnemyDecision(resources int) map[string]interface{} {
	if resources >= 200 {
		return map[string]interface{}{
			"action": "wave",
			"reason": "Fallback decision: high resources available",
		}
	} else if resources >= 50 {
		return map[string]interface{}{
			"action":     "spawn",
			"enemy_type": "tank",
			"reason":     "Fallback decision: spawn tank with medium resources",
		}
	} else if resources >= 30 {
		return map[string]interface{}{
			"action":     "spawn",
			"enemy_type": "fast",
			"reason":     "Fallback decision: spawn fast with low resources",
		}
	} else if resources >= 20 {
		return map[string]interface{}{
			"action":     "spawn",
			"enemy_type": "basic",
			"reason":     "Fallback decision: spawn basic with minimal resources",
		}
	} else {
		return map[string]interface{}{
			"action": "save",
			"reason": "Fallback decision: insufficient resources for any enemy",
		}
	}
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

	CurrentTurn       string // "chatgpt" or "gemini"
	LastActionTime    time.Time
	MaxResources      int
	MaxWaves          int
	TurnTimeout       time.Duration
	PauseBetweenTurns bool
	PauseDuration     time.Duration

	// State tracking for reduced output
	lastStatePrintTime time.Time
	lastEnemyCount     int
	lastTowerCount     int
	stateChangeCounter int
	rng                *rand.Rand

	// Collects textual events; UI can display from this instead of direct stdout.
	Logs []string

	// Role mapping
	Defender string // name of defending player
	Attacker string // name of attacking player
}

func NewGame(openaiKey, googleKey string) *Game {
	// Initialize with fixed dimensions
	width := 80
	height := 24
	mapHeight := height - 10

	// Create RNG for the game
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

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
		Defender:           "chatgpt",
		Attacker:           "gemini",
		OpenAIHandler:      &OpenAIHandler{AIHandler: NewAIHandler(rng), APIKey: openaiKey},
		GeminiHandler:      &GeminiHandler{AIHandler: NewAIHandler(rng), APIKey: googleKey},
		GameSpeed:          0.1,
		AIDecisionInterval: map[string]int{"chatgpt": 2, "gemini": 2},
		LastAIDecision: map[string]time.Time{
			"chatgpt": time.Now(),
			"gemini":  time.Now(),
		},
		CurrentTurn:        "chatgpt", // ChatGPT goes first
		LastActionTime:     time.Now(),
		MaxResources:       800,              // Reduced maximum resources to encourage spending
		MaxWaves:           30,               // Reduced to have a more focused game
		TurnTimeout:        45 * time.Second, // Increased timeout to allow more API response time
		PauseBetweenTurns:  true,             // Pause between turns for better visualization
		PauseDuration:      1 * time.Second,  // Duration of pause between turns
		lastStatePrintTime: time.Now(),
		lastEnemyCount:     0,
		lastTowerCount:     0,
		stateChangeCounter: 0,
		rng:                rng,
		Logs:               make([]string, 0),
	}

	// Generate path
	game.Path = game.generatePath()
	return game
}

func (g *Game) generatePath() []Position {
	path := make([]Position, 0)

	// Create a path that leaves more space for tower placement
	// Use a more compact zigzag that stays centered in the map

	// Adjust zigzag parameters to leave more space
	zigzagHeight := g.MapHeight / 5           // Reduced from /4 to /5 to make narrower
	centerY := g.MapHeight / 2                // Keep center at middle
	pathWidth := int(float64(g.Width) * 0.60) // Use only 60% of width instead of 65%

	// Entry point from left side - keep compact
	startY := centerY
	startX := 1
	for i := 0; i < 3; i++ { // Reduced from 5 to 3
		path = append(path, Position{Y: startY, X: startX + i})
	}

	// Calculate bounds of zigzag to keep it more centered
	leftBound := 7                           // Start zigzag further to the right (increased from 5)
	rightBound := pathWidth                  // End before reaching far right
	zigzagTop := centerY - zigzagHeight/2    // Raise the top of zigzag
	zigzagBottom := centerY + zigzagHeight/2 // Lower the bottom of zigzag

	// Create more gentle zigzag with increased spacing between legs
	x := leftBound
	goingDown := true

	for x < rightBound {
		x++ // Move right one step at a time

		// Use fewer zigzags with more horizontal space between them
		if goingDown {
			// Going from top to bottom
			for y := zigzagTop; y <= zigzagBottom; y++ {
				path = append(path, Position{Y: y, X: x})
			}
			goingDown = false
		} else {
			// Going from bottom to top
			for y := zigzagBottom; y >= zigzagTop; y-- {
				path = append(path, Position{Y: y, X: x})
			}
			goingDown = true
		}

		// Skip more horizontal space to make zigzag wider and leave room for towers
		if x < rightBound-10 {
			x += 6 // Increased from 2 to 6 for more space between zigzags
		}
	}

	// Exit path to right edge
	lastPos := path[len(path)-1]
	for i := 1; i <= 3; i++ { // Reduced from 5 to 3
		path = append(path, Position{Y: lastPos.Y, X: lastPos.X + i})
	}

	// Print path dimensions for debugging
	minY, maxY := g.MapHeight, 0
	minX, maxX := g.Width, 0

	for _, pos := range path {
		if pos.Y < minY {
			minY = pos.Y
		}
		if pos.Y > maxY {
			maxY = pos.Y
		}
		if pos.X < minX {
			minX = pos.X
		}
		if pos.X > maxX {
			maxX = pos.X
		}
	}

	pathWidthDim := maxX - minX
	pathHeightDim := maxY - minY

	g.logf("Path generated: %d positions, bounds: Y[%d-%d], X[%d-%d], dimensions: %d×%d",
		len(path), minY, maxY, minX, maxX, pathWidthDim, pathHeightDim)
	g.logf("Free area for towers: approximately %.1f%% of map",
		100.0-(float64(len(path)*4)/float64(g.MapHeight*g.Width)*100.0))

	return path
}

func (g *Game) HandleAIDecisions() {
	if !g.AIEnabled {
		return
	}

	currentTime := time.Now()
	gameState := g.getGameState()

	// Log game state periodically for debugging
	if currentTime.Sub(g.lastStatePrintTime) > 10*time.Second {
		g.logf("\n=== Game State ===")
		g.logf("Wave: %d", g.Wave)
		g.logf("Current Turn: %s", g.CurrentTurn)
		g.logf("ChatGPT - Lives: %d, Resources: %d, Score: %d",
			g.Lives["chatgpt"], g.Resources["chatgpt"], g.Score["chatgpt"])
		g.logf("Gemini - Resources: %d, Score: %d",
			g.Resources["gemini"], g.Score["gemini"])
		g.logf("Active Towers: %d, Active Enemies: %d",
			len(g.Towers), len(g.Enemies))
		g.logf("Wave Queue: %d enemies", len(g.WaveQueue))
		g.logf("Time since last action: %.1f seconds",
			currentTime.Sub(g.LastActionTime).Seconds())
		g.logf("==================\n")
		g.lastStatePrintTime = currentTime
	}

	// If any AI is thinking, don't allow new decisions
	if g.AIThinking["chatgpt"] || g.AIThinking["gemini"] {
		// Only print once every few seconds to avoid log spam
		timeLastPrinted := g.LastAIDecision[g.CurrentTurn].Add(2 * time.Second)
		if currentTime.After(timeLastPrinted) {
			g.logf("Waiting for %s to finish thinking...", g.CurrentTurn)
			g.LastAIDecision[g.CurrentTurn] = currentTime
		}
		return
	}

	// Check if game should end due to timeout or max waves
	if currentTime.Sub(g.LastActionTime) > g.TurnTimeout {
		g.logf("\n=== Game Over! ===")
		g.logf("Game ended due to inactivity")
		g.GameOver = true
		g.Winner = "none"
		return
	}

	// Print turn information at the start of each turn
	if !g.AIThinking["chatgpt"] && !g.AIThinking["gemini"] {
		g.logf("\n=== Current Turn: %s ===", g.CurrentTurn)
	}

	// Only allow AI to make a move if it's their turn
	if g.CurrentTurn == "chatgpt" && !g.AIThinking["chatgpt"] && g.AIEnabled {
		fmt.Println("\n=== ChatGPT's Turn ===")

		// Check if there are enough resources for at least a basic tower before making the API call
		chatgptResources := g.Resources["chatgpt"]
		lowestTowerCost := 100 // Cost of the cheapest tower (basic)

		if chatgptResources < lowestTowerCost {
			fmt.Printf("ChatGPT has insufficient resources (%d) for any tower. Saving resources.\n", chatgptResources)
			g.LastDecisions["chatgpt"] = "Insufficient resources for any tower"
			g.CurrentTurn = "gemini"       // Switch to Gemini's turn
			g.LastActionTime = currentTime // Update last action time to prevent timeout

			// Add pause between turns if enabled
			if g.PauseBetweenTurns {
				time.Sleep(g.PauseDuration)
			}

			return
		}

		g.AIThinking["chatgpt"] = true
		g.LastActionTime = currentTime

		go func() {
			decision, err := g.OpenAIHandler.GetTowerDecision(gameState)
			if err == nil {

				action, _ := decision["action"].(string)
				g.logf("ChatGPT decided to: %s", action)

				if action == "place" {
					towerType, _ := decision["tower_type"].(string)
					if towerType == "" {
						towerType = "basic"
					}

					// Check if we have enough resources for the chosen tower type
					towerCosts := map[string]int{"basic": 100, "sniper": 250, "splash": 200}
					cost, exists := towerCosts[towerType]
					if !exists {
						cost = 100 // Default to basic cost if type not found
					}

					if g.Resources["chatgpt"] < cost {
						// Not enough resources for this tower type, downgrade to a more affordable type
						if g.Resources["chatgpt"] >= 200 {
							g.logf("Downgrading choice from %s to splash tower due to resource constraints", towerType)
							towerType = "splash"
						} else if g.Resources["chatgpt"] >= 100 {
							g.logf("Downgrading choice from %s to basic tower due to resource constraints", towerType)
							towerType = "basic"
						} else {
							g.logf("Not enough resources (%d) for any tower, saving for next turn", g.Resources["chatgpt"])
							g.LastDecisions["chatgpt"] = "Insufficient resources - saving for next turn"
							g.CurrentTurn = "gemini" // Switch to Gemini's turn
							g.AIThinking["chatgpt"] = false
							return
						}
					}

					position, ok := decision["position"].([]interface{})

					// Set default position near the corners or edges for better coverage
					y, x := 2, 2 // Default to top-left corner

					// If position is specified in the response, use it
					if ok && len(position) >= 2 {
						y = int(position[0].(float64))
						x = int(position[1].(float64))
					} else {
						// Use strategic positions
						// Define good strategic positions away from the path
						goodPositions := [][]int{
							{2, 2}, {2, g.Width - 3},
							{g.MapHeight - 3, 2}, {g.MapHeight - 3, g.Width - 3},
							{g.MapHeight / 4, g.Width / 4},
							{g.MapHeight / 4, 3 * g.Width / 4},
							{3 * g.MapHeight / 4, g.Width / 4},
							{3 * g.MapHeight / 4, 3 * g.Width / 4},
						}

						// Select a random good position
						pos := goodPositions[g.rng.Intn(len(goodPositions))]
						y, x = pos[0], pos[1]
					}

					// Attempt to place the tower
					placed := g.placeTower(y, x, towerType)

					if placed {
						g.logf("Successfully placed %s tower", towerType)
						g.LastDecisions["chatgpt"] = fmt.Sprintf("Placed %s tower", towerType)
					} else {
						// Try a different tower type if placement failed - maybe the map is crowded
						if towerType != "basic" {
							g.logf("Failed to place %s tower, trying basic tower instead", towerType)
							placed = g.placeTower(y, x, "basic")
							if placed {
								g.logf("Successfully placed basic tower as fallback")
								g.LastDecisions["chatgpt"] = "Placed basic tower as fallback"
							} else {
								g.logf("Failed to place any tower - map may be too crowded")
								g.LastDecisions["chatgpt"] = "Failed to place any tower"
							}
						} else {
							g.logf("Failed to place basic tower - map may be too crowded")
							g.LastDecisions["chatgpt"] = "Failed to place basic tower"
						}
					}
					g.CurrentTurn = "gemini" // Switch to Gemini's turn

					// Add pause between turns if enabled
					if g.PauseBetweenTurns {
						time.Sleep(g.PauseDuration)
					}
				} else if action == "save" {
					// Only allow saving if we have a lot of towers already
					if len(g.Towers) >= 5 {
						g.logf("ChatGPT decided to save resources")
						g.LastDecisions["chatgpt"] = "Saving resources"
					} else {
						// Force tower placement for better defense if fewer than 5 towers
						g.logf("Converting save action to tower placement for better defense")
						towerType := "basic"
						if g.Resources["chatgpt"] >= 250 {
							towerType = "sniper"
						} else if g.Resources["chatgpt"] >= 200 {
							towerType = "splash"
						}

						// Use strategic default position
						y, x := 2, 2
						if len(g.Towers) == 1 {
							y, x = 2, g.Width-3 // Top right
						} else if len(g.Towers) == 2 {
							y, x = g.MapHeight-3, 2 // Bottom left
						} else if len(g.Towers) == 3 {
							y, x = g.MapHeight-3, g.Width-3 // Bottom right
						} else {
							y, x = g.MapHeight/2, g.Width/2 // Center
						}

						placed := g.placeTower(y, x, towerType)
						if placed {
							g.logf("Successfully placed %s tower at strategic position", towerType)
							g.LastDecisions["chatgpt"] = fmt.Sprintf("Placed %s tower at strategic position", towerType)
						} else {
							g.logf("Failed to place tower at strategic position")
							g.LastDecisions["chatgpt"] = "Failed to place tower at strategic position"
						}
					}
					g.CurrentTurn = "gemini" // Switch to Gemini's turn

					// Add pause between turns if enabled
					if g.PauseBetweenTurns {
						time.Sleep(g.PauseDuration)
					}
				} else {
					g.logf("ChatGPT made invalid decision, defaulting to placing basic tower")
					placed := g.placeTower(2, 2, "basic")
					if placed {
						g.LastDecisions["chatgpt"] = "Placed basic tower (default action)"
					} else {
						g.LastDecisions["chatgpt"] = "Failed to place tower (invalid decision)"
					}
					g.CurrentTurn = "gemini" // Switch to Gemini's turn

					// Add pause between turns if enabled
					if g.PauseBetweenTurns {
						time.Sleep(g.PauseDuration)
					}
				}
			} else {
				g.logf("ChatGPT API error: %v", err)
				g.LastDecisions["chatgpt"] = "API error"
				g.CurrentTurn = "gemini" // Switch to Gemini's turn on error

				// Add pause between turns if enabled
				if g.PauseBetweenTurns {
					time.Sleep(g.PauseDuration)
				}
			}

			g.AIThinking["chatgpt"] = false
		}()
	} else if g.CurrentTurn == "gemini" && !g.AIThinking["gemini"] && g.AIEnabled {
		fmt.Println("\n=== Gemini's Turn ===")

		// Check if Gemini has enough resources for at least a basic enemy
		if g.Resources["gemini"] < 20 { // 20 is cost of basic enemy
			fmt.Printf("Gemini has insufficient resources (%d) for any enemy. Saving resources.\n", g.Resources["gemini"])
			g.LastDecisions["gemini"] = "Insufficient resources for any enemy"
			g.CurrentTurn = "chatgpt"      // Switch to ChatGPT's turn
			g.LastActionTime = currentTime // Update last action time to prevent timeout

			// Add pause between turns if enabled
			if g.PauseBetweenTurns {
				time.Sleep(g.PauseDuration)
			}

			return
		}

		g.AIThinking["gemini"] = true
		g.LastActionTime = currentTime

		go func() {
			decision, err := g.GeminiHandler.GetEnemyDecision(gameState)
			if err == nil {
				action, _ := decision["action"].(string)
				g.logf("Gemini decided to: %s", action)

				if action == "spawn" {
					enemyType, _ := decision["enemy_type"].(string)
					if enemyType == "" {
						enemyType = "basic"
					}

					// Check if we have enough resources for the chosen enemy type
					enemyCosts := map[string]int{"basic": 20, "fast": 30, "tank": 50}
					cost, exists := enemyCosts[enemyType]
					if !exists {
						cost = 20 // Default to basic cost if type not found
					}

					// Downgrade enemy type if not enough resources
					if g.Resources["gemini"] < cost {
						if g.Resources["gemini"] >= 30 {
							g.logf("Downgrading choice from %s to fast enemy due to resource constraints", enemyType)
							enemyType = "fast"
						} else if g.Resources["gemini"] >= 20 {
							g.logf("Downgrading choice from %s to basic enemy due to resource constraints", enemyType)
							enemyType = "basic"
						} else {
							g.logf("Not enough resources (%d) for any enemy, saving for next turn", g.Resources["gemini"])
							g.LastDecisions["gemini"] = "Insufficient resources - saving for next turn"
							g.CurrentTurn = "chatgpt" // Switch to ChatGPT's turn
							g.AIThinking["gemini"] = false
							return
						}
					}

					g.logf("Attempting to spawn %s enemy", enemyType)
					spawned := g.spawnEnemy(enemyType, nil)
					if spawned {
						g.logf("Successfully spawned %s enemy", enemyType)
						g.LastDecisions["gemini"] = fmt.Sprintf("Spawned %s enemy", enemyType)
					} else {
						g.logf("Failed to spawn %s enemy (not enough resources or invalid type)", enemyType)
						g.LastDecisions["gemini"] = fmt.Sprintf("Failed to spawn %s enemy", enemyType)
					}
					g.CurrentTurn = "chatgpt" // Switch to ChatGPT's turn

					// Add pause between turns if enabled
					if g.PauseBetweenTurns {
						time.Sleep(g.PauseDuration)
					}
				} else if action == "wave" {
					waveCost := 40 + (g.Wave * 5) // Match the calculation in spawnWave
					if waveCost > 200 {
						waveCost = 200 // Cap at 200 to ensure it's affordable
					}

					g.logf("Attempting to launch wave %d (cost: %d, available: %d)",
						g.Wave, waveCost, g.Resources["gemini"])

					if g.spawnWave() {
						g.logf("Wave %d launched successfully", g.Wave)
						g.LastDecisions["gemini"] = fmt.Sprintf("Launched wave %d", g.Wave)
					} else {
						g.logf("Failed to launch wave (not enough resources, need %d)", waveCost)

						// Fall back to spawning a basic enemy if wave launch fails
						if g.Resources["gemini"] >= 20 {
							g.logf("Falling back to spawning basic enemy")
							if g.spawnEnemy("basic", nil) {
								g.LastDecisions["gemini"] = "Spawned basic enemy (fallback from wave)"
							} else {
								g.LastDecisions["gemini"] = "Failed to spawn fallback enemy"
							}
						} else {
							g.LastDecisions["gemini"] = "Failed to launch wave (not enough resources)"
						}
					}
					g.CurrentTurn = "chatgpt" // Switch to ChatGPT's turn

					// Add pause between turns if enabled
					if g.PauseBetweenTurns {
						time.Sleep(g.PauseDuration)
					}
				} else if action == "save" {
					// Only allow saving if resources are very low
					if g.Resources["gemini"] < 30 {
						g.logf("Gemini decided to save resources")
						g.LastDecisions["gemini"] = "Saving resources"
					} else {
						// Convert save to spawn for more aggression
						g.logf("Converting save action to spawn for better aggression")
						enemyType := "basic"
						if g.Resources["gemini"] >= 50 {
							enemyType = "tank"
						} else if g.Resources["gemini"] >= 30 {
							enemyType = "fast"
						}

						if g.spawnEnemy(enemyType, nil) {
							g.LastDecisions["gemini"] = fmt.Sprintf("Spawned %s enemy (converted from save)", enemyType)
						} else {
							g.LastDecisions["gemini"] = "Failed to spawn enemy (converted from save)"
						}
					}
					g.CurrentTurn = "chatgpt" // Switch to ChatGPT's turn

					// Add pause between turns if enabled
					if g.PauseBetweenTurns {
						time.Sleep(g.PauseDuration)
					}
				} else {
					g.logf("Gemini made an invalid decision: %s", action)

					// Default to spawning a basic enemy on invalid decision
					if g.Resources["gemini"] >= 20 {
						if g.spawnEnemy("basic", nil) {
							g.LastDecisions["gemini"] = "Spawned basic enemy (default action)"
						} else {
							g.LastDecisions["gemini"] = "Failed to spawn enemy (invalid decision)"
						}
					} else {
						g.LastDecisions["gemini"] = "Invalid decision - not enough resources"
					}
					g.CurrentTurn = "chatgpt" // Switch to ChatGPT's turn on error

					// Add pause between turns if enabled
					if g.PauseBetweenTurns {
						time.Sleep(g.PauseDuration)
					}
				}
			} else {
				g.logf("Gemini API error: %v", err)
				g.LastDecisions["gemini"] = "API error"
				g.CurrentTurn = "chatgpt" // Switch to ChatGPT's turn on error

				// Add pause between turns if enabled
				if g.PauseBetweenTurns {
					time.Sleep(g.PauseDuration)
				}
			}

			g.AIThinking["gemini"] = false
		}()
	}
}

// logf appends a formatted message to the Game log
func (g *Game) logf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	g.Logs = append(g.Logs, msg)
}

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
