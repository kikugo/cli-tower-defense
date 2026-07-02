package engine

// TowerStat and EnemyStat hold the tunable numbers for one entity type.
type TowerStat struct {
	Damage   int `json:"damage"`
	Range    int `json:"range"`
	Cooldown int `json:"cooldown"`
	Cost     int `json:"cost"`
}

type EnemyStat struct {
	Health    int     `json:"health"`
	Speed     float64 `json:"speed"`
	Reward    int     `json:"reward"`
	SpawnCost int     `json:"spawn_cost"`
	Shield    int     `json:"shield"`
}

// BalanceConfig is the single source of truth for combat/economy numbers.
// Game logic, affordability, and prompt text all read from it.
type BalanceConfig struct {
	Version              string               `json:"version"`
	Towers               map[string]TowerStat `json:"towers"`
	Enemies              map[string]EnemyStat `json:"enemies"`
	BreachResourceBounty int                  `json:"breach_resource_bounty"`
	BreachScore          int                  `json:"breach_score"`
}

// placeableTowerTypes is the ordered set models may build (prompt schema).
var placeableTowerTypes = []string{"basic", "splash", "sniper", "buffer"}

// attackerEnemyTypes is the ordered set models may spawn.
var attackerEnemyTypes = []string{"basic", "fast", "healer", "shielded", "tank"}

// Display glyphs are presentation, not balance; they stay fixed.
var towerChars = map[string]rune{"basic": '^', "sniper": '⌖', "splash": '⊕', "buffer": 'B', "custom": '?'}
var enemyChars = map[string]rune{"basic": 'o', "fast": '>', "tank": '□', "shielded": 'S', "healer": 'H', "custom": '?'}

func DefaultBalanceConfig() BalanceConfig {
	return BalanceConfig{
		Version: "v1",
		Towers: map[string]TowerStat{
			"basic":  {Damage: 15, Range: 5, Cooldown: 5, Cost: 100},
			"sniper": {Damage: 50, Range: 12, Cooldown: 15, Cost: 250},
			"splash": {Damage: 10, Range: 3, Cooldown: 3, Cost: 200},
			"buffer": {Damage: 0, Range: 2, Cooldown: 0, Cost: 300},
			"custom": {Damage: 20, Range: 7, Cooldown: 8, Cost: 150},
		},
		Enemies: map[string]EnemyStat{
			"basic":    {Health: 100, Speed: 1.0, Reward: 20, SpawnCost: 20},
			"fast":     {Health: 50, Speed: 2.0, Reward: 15, SpawnCost: 30},
			"tank":     {Health: 300, Speed: 0.5, Reward: 50, SpawnCost: 50},
			"shielded": {Health: 150, Speed: 0.8, Reward: 40, SpawnCost: 40, Shield: 2},
			"healer":   {Health: 80, Speed: 1.0, Reward: 30, SpawnCost: 30},
			"custom":   {Health: 150, Speed: 1.2, Reward: 25},
		},
		BreachResourceBounty: 30,
		BreachScore:          50,
	}
}

func (g *Game) towerCost(name string) (int, bool) {
	st, ok := g.Balance.Towers[name]
	if !ok {
		return 0, false
	}
	return st.Cost, true
}

func (g *Game) spawnCost(name string) (int, bool) {
	st, ok := g.Balance.Enemies[name]
	if !ok || st.SpawnCost <= 0 {
		return 0, false
	}
	return st.SpawnCost, true
}

// newTower builds a tower from the game's balance config. "custom" params
// may override individual fields (legacy behavior).
func (g *Game) newTower(y, x int, towerType string, params map[string]interface{}) Tower {
	st := g.Balance.Towers[towerType]
	if towerType == "custom" && params != nil {
		if v, ok := params["damage"]; ok {
			st.Damage = toInt(v)
		}
		if v, ok := params["range"]; ok {
			st.Range = toInt(v)
		}
		if v, ok := params["cooldown"]; ok {
			st.Cooldown = toInt(v)
		}
		if v, ok := params["cost"]; ok {
			st.Cost = toInt(v)
		}
	}
	char, ok := towerChars[towerType]
	if !ok {
		char = '^'
	}
	return Tower{
		Entity: Entity{
			Pos: Position{Y: y, X: x}, Char: char,
			Health: 100, MaxHealth: 100,
			Damage: st.Damage, Cooldown: 0, MaxCD: st.Cooldown,
		},
		TowerType: towerType, Range: st.Range, Cost: st.Cost, Strategy: "nearest",
	}
}

// newEnemy builds an enemy from the game's balance config.
func (g *Game) newEnemy(y, x int, enemyType string, params map[string]interface{}) Enemy {
	st := g.Balance.Enemies[enemyType]
	if enemyType == "custom" && params != nil {
		if v, ok := params["health"]; ok {
			st.Health = toInt(v)
		}
		if v, ok := params["speed"].(float64); ok {
			st.Speed = v
		}
		if v, ok := params["reward"]; ok {
			st.Reward = toInt(v)
		}
		if v, ok := params["shield"]; ok {
			st.Shield = toInt(v)
		}
	}
	char, ok := enemyChars[enemyType]
	if !ok {
		char = '?'
	}
	return Enemy{
		Entity: Entity{
			Pos: Position{Y: y, X: x}, Char: char,
			Health: st.Health, MaxHealth: st.Health,
		},
		EnemyType: enemyType, Speed: st.Speed, Reward: st.Reward, Shield: st.Shield,
	}
}
