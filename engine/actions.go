package engine

import (
	"math"
)

// getGameState returns a simplified snapshot for AI prompts or debugging.
func (g *Game) getGameState() map[string]interface{} {
	towers := make([]interface{}, len(g.Towers))
	for i, t := range g.Towers {
		towers[i] = map[string]interface{}{
			"id":       i,
			"type":     t.TowerType,
			"position": []int{t.Pos.Y, t.Pos.X},
			"damage":   t.Damage,
			"range":    t.Range,
			"cooldown": t.Cooldown,
			"level":    t.Level,
		}
	}

	enemies := make([]interface{}, len(g.Enemies))
	for i, e := range g.Enemies {
		pathLen := 1
		if e.PathID < len(g.Paths) {
			pathLen = len(g.Paths[e.PathID])
		}
		progress := float64(e.PathIndex) / float64(pathLen)

		enemies[i] = map[string]interface{}{
			"type":     e.EnemyType,
			"position": []int{e.Pos.Y, e.Pos.X},
			"health":   e.Health,
			"speed":    e.Speed,
			"shield":   e.Shield,
			"progress": progress,
			"path_id":  e.PathID,
		}
	}

	slowZones := make([][]int, len(g.SlowZones))
	for i, sz := range g.SlowZones {
		slowZones[i] = []int{sz.Pos.Y, sz.Pos.X}
	}

	obstacles := make([][]int, len(g.Obstacles))
	for i, obs := range g.Obstacles {
		obstacles[i] = []int{obs.Y, obs.X}
	}

	// Convert resources and lives to map[string]interface{}
	resourcesIface := make(map[string]interface{}, len(g.Resources))
	for k, v := range g.Resources {
		resourcesIface[k] = v
	}
	incomeIface := make(map[string]interface{}, len(g.Income))
	for k, v := range g.Income {
		incomeIface[k] = v
	}
	livesIface := make(map[string]interface{}, len(g.Lives))
	for k, v := range g.Lives {
		livesIface[k] = v
	}

	return map[string]interface{}{
		"towers":         towers,
		"enemies":        enemies,
		"slow_zones":     slowZones,
		"obstacles":      obstacles,
		"resources":      resourcesIface,
		"income":         incomeIface,
		"lives":          livesIface,
		"wave":           g.Wave,
		"score":          g.Score,
		"paths_count":    len(g.Paths),
		"wave_queue":     len(g.WaveQueue),
		"active_enemies": len(g.Enemies),
	}
}

// placeTower tries to build a tower and returns true on success.
func (g *Game) placeTower(y, x int, towerType string) bool {
	costs := map[string]int{"basic": 100, "splash": 200, "sniper": 250, "buffer": 300}
	cost, ok := costs[towerType]
	if !ok {
		g.logf("Invalid tower type: %s", towerType)
		return false
	}
	if g.Resources[g.Defender] < cost {
		return false
	}
	// simple bounds/path/obstacle check
	if y < 0 || y >= g.MapHeight || x < 0 || x >= g.MapWidth {
		return false
	}
	for _, path := range g.Paths {
		for _, pos := range path {
			if pos.Y == y && pos.X == x {
				return false
			}
		}
	}
	for _, obs := range g.Obstacles {
		if obs.Y == y && obs.X == x {
			return false
		}
	}
	for _, t := range g.Towers {
		if t.Pos.Y == y && t.Pos.X == x {
			return false
		}
	}
	tw := NewTower(y, x, towerType, nil)
	g.Towers = append(g.Towers, &tw)
	g.Resources[g.Defender] -= cost
	return true
}

func (g *Game) upgradeTower(id int) bool {
	if id < 0 || id >= len(g.Towers) {
		return false
	}
	t := g.Towers[id]
	cost := 150 * (t.Level + 1)
	if g.Resources[g.Defender] < cost {
		return false
	}
	t.Upgrade()
	g.Resources[g.Defender] -= cost
	return true
}

func (g *Game) placeSlowZone(y, x int) bool {
	cost := 150
	if g.Resources[g.Defender] < cost {
		return false
	}
	// Check if on path
	isOnPath := false
	for _, path := range g.Paths {
		for _, pos := range path {
			if pos.Y == y && pos.X == x {
				isOnPath = true
				break
			}
		}
		if isOnPath {
			break
		}
	}
	if !isOnPath {
		return false
	}
	// Check if already has slow zone
	for _, sz := range g.SlowZones {
		if sz.Pos.Y == y && sz.Pos.X == x {
			return false
		}
	}
	g.SlowZones = append(g.SlowZones, &SlowZone{Pos: Position{Y: y, X: x}})
	g.Resources[g.Defender] -= cost
	return true
}

func (g *Game) invest(playerID string) bool {
	cost := 150
	if g.Resources[playerID] < cost {
		return false
	}
	g.Income[playerID] += 2
	g.Resources[playerID] -= cost
	return true
}

// spawnEnemy deducts resources and adds an enemy to the field.
func (g *Game) spawnEnemy(enemyType string, _ map[string]interface{}) bool {
	costs := map[string]int{"basic": 20, "fast": 30, "tank": 50, "shielded": 40, "healer": 30}
	cost, ok := costs[enemyType]
	if !ok {
		g.logf("Invalid enemy type: %s", enemyType)
		return false
	}
	if g.Resources[g.Attacker] < cost {
		return false
	}

	pathIdx := g.rng.Intn(len(g.Paths))
	path := g.Paths[pathIdx]
	if len(path) == 0 {
		return false
	}
	start := path[0]
	en := NewEnemy(start.Y, start.X, enemyType, nil)
	en.PathID = pathIdx
	g.Enemies = append(g.Enemies, &en)
	g.Resources[g.Attacker] -= cost
	return true
}

// spawnWave queues a mix of enemies and deducts resources.
func (g *Game) spawnWave() bool {
	waveCost := 40 + g.Wave*5
	if waveCost > 200 {
		waveCost = 200
	}
	if g.Resources[g.Attacker] < waveCost {
		return false
	}
	num := 5 + g.Wave
	if num > 30 {
		num = 30
	}
	for i := 0; i < num; i++ {
		switch {
		case g.Wave > 15:
			g.WaveQueue = append(g.WaveQueue, []string{"tank", "fast", "shielded", "healer"}[i%4])
		case g.Wave > 5:
			g.WaveQueue = append(g.WaveQueue, []string{"fast", "basic", "tank", "shielded"}[i%4])
		default:
			g.WaveQueue = append(g.WaveQueue, []string{"basic", "fast"}[i%2])
		}
	}
	g.Resources[g.Attacker] -= waveCost
	g.Wave++
	return true
}

// UpdateGameState advances the simulation by one tick.
func (g *Game) UpdateGameState() {
	if g == nil || g.GameOver {
		return
	}

	// Passive income
	g.stateChangeCounter++
	if g.stateChangeCounter%10 == 0 {
		for p, inc := range g.Income {
			g.Resources[p] += inc
		}
	}

	// 1. Spawn queued enemies gradually
	if len(g.WaveQueue) > 0 {
		etype := g.WaveQueue[0]
		g.WaveQueue = g.WaveQueue[1:]
		pathIdx := g.rng.Intn(len(g.Paths))
		path := g.Paths[pathIdx]
		if len(path) > 0 {
			start := path[0]
			en := NewEnemy(start.Y, start.X, etype, nil)
			en.PathID = pathIdx
			g.Enemies = append(g.Enemies, &en)
		}
	}

	// 1.5 Special Ability: Healer Enemy
	for _, e := range g.Enemies {
		if e.EnemyType == "healer" && e.Health > 0 && e.Cooldown <= 0 {
			healed := false
			for _, target := range g.Enemies {
				if target == e || target.Health <= 0 {
					continue
				}
				dist := math.Sqrt(math.Pow(float64(e.Pos.Y-target.Pos.Y), 2) + math.Pow(float64(e.Pos.X-target.Pos.X), 2))
				if dist <= 3.0 {
					target.Health += 10
					if target.Health > target.MaxHealth {
						target.Health = target.MaxHealth
					}
					healed = true
				}
			}
			if healed {
				e.Cooldown = 10 // 1 second cooldown
				g.Particles = append(g.Particles, &Particle{Pos: e.Pos, Char: '+', Lifetime: 3, Color: "green"})
			}
		} else if e.Cooldown > 0 {
			e.Cooldown--
		}
	}

	// Update particles
	remainingParticles := make([]*Particle, 0)
	for _, p := range g.Particles {
		p.Lifetime--
		if p.Lifetime > 0 {
			remainingParticles = append(remainingParticles, p)
		}
	}
	g.Particles = remainingParticles

	// 2. Towers act (cooldown & attack).
	boosts := make(map[*Tower]float64)
	for _, t := range g.Towers {
		if t.TowerType == "buffer" {
			for _, target := range g.Towers {
				if target == t {
					continue
				}
				dist := math.Sqrt(math.Pow(float64(t.Pos.Y-target.Pos.Y), 2) + math.Pow(float64(t.Pos.X-target.Pos.X), 2))
				if dist <= float64(t.Range) {
					boosts[target] += 0.5
				}
			}
		}
	}

	for _, t := range g.Towers {
		if t.TowerType == "buffer" {
			continue
		}
		if t.Cooldown > 0 {
			t.Cooldown--
		}
		if t.CanAttack() {
			originalDamage := t.Damage
			boost := boosts[t]
			if boost > 1.0 {
				boost = 1.0
			} // Cap boost at 100%
			t.Damage = int(float64(t.Damage) * (1.0 + boost))

			killed := t.Attack(g.Enemies)
			for _, e := range killed {
				g.Particles = append(g.Particles, &Particle{Pos: e.Pos, Char: '*', Lifetime: 2, Color: "red"})
				if e.Health <= 0 {
					g.Score[g.Defender] += e.Reward
					g.Resources[g.Defender] += e.Reward
				}
			}
			t.Damage = originalDamage
		}
	}

	// 3. Move enemies & collect survivors.
	remaining := make([]*Enemy, 0, len(g.Enemies))
	for _, e := range g.Enemies {
		if e.Health <= 0 {
			continue
		}

		pathIdx := e.PathID
		if pathIdx >= len(g.Paths) {
			pathIdx = 0
		}
		path := g.Paths[pathIdx]

		actualSpeed := e.Speed
		for _, sz := range g.SlowZones {
			if sz.Pos.Y == e.Pos.Y && sz.Pos.X == e.Pos.X {
				actualSpeed *= 0.5
				break
			}
		}
		e.DistanceMoved += actualSpeed

		for e.DistanceMoved >= 1.0 && e.PathIndex < len(path)-1 {
			e.PathIndex++
			e.DistanceMoved -= 1.0
			p := path[e.PathIndex]
			e.Pos = Position{Y: p.Y, X: p.X}
		}

		if e.PathIndex >= len(path)-1 {
			g.Lives[g.Defender]--
			g.Resources[g.Attacker] += 30
			g.Score[g.Attacker] += 50

			if g.Lives[g.Defender] <= 0 {
				g.GameOver = true
				g.Winner = g.Attacker
			}
			continue
		}

		remaining = append(remaining, e)
	}
	g.Enemies = remaining

	// 4. Victory condition
	if g.Lives[g.Defender] > 0 && len(g.Enemies) == 0 && len(g.WaveQueue) == 0 && g.Wave >= g.MaxWaves {
		g.GameOver = true
		g.Winner = g.Defender
	}
}
