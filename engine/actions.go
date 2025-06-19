package engine

// getGameState returns a simplified snapshot for AI prompts or debugging.
func (g *Game) getGameState() map[string]interface{} {
	towers := make([]interface{}, len(g.Towers))
	for i, t := range g.Towers {
		towers[i] = map[string]interface{}{
			"type":     t.TowerType,
			"position": []int{t.Pos.Y, t.Pos.X},
			"damage":   t.Damage,
			"range":    t.Range,
			"cooldown": t.Cooldown,
		}
	}

	enemies := make([]interface{}, len(g.Enemies))
	for i, e := range g.Enemies {
		progress := 0.0
		if len(g.Path) > 0 {
			progress = float64(e.PathIndex) / float64(len(g.Path))
		}
		enemies[i] = map[string]interface{}{
			"type":     e.EnemyType,
			"position": []int{e.Pos.Y, e.Pos.X},
			"health":   e.Health,
			"speed":    e.Speed,
			"progress": progress,
		}
	}

	// Convert resources and lives to map[string]interface{} for easier JSON-like handling.
	resourcesIface := make(map[string]interface{}, len(g.Resources))
	for k, v := range g.Resources {
		resourcesIface[k] = v
	}
	livesIface := make(map[string]interface{}, len(g.Lives))
	for k, v := range g.Lives {
		livesIface[k] = v
	}

	return map[string]interface{}{
		"towers":         towers,
		"enemies":        enemies,
		"resources":      resourcesIface,
		"lives":          livesIface,
		"wave":           g.Wave,
		"score":          g.Score,
		"path_length":    len(g.Path),
		"wave_queue":     len(g.WaveQueue),
		"active_enemies": len(g.Enemies),
	}
}

// placeTower tries to build a tower and returns true on success.
func (g *Game) placeTower(y, x int, towerType string) bool {
	costs := map[string]int{"basic": 100, "splash": 200, "sniper": 250}
	cost, ok := costs[towerType]
	if !ok {
		g.logf("Invalid tower type: %s", towerType)
		return false
	}
	if g.Resources["chatgpt"] < cost {
		return false
	}
	// simple bounds/path check
	if y < 0 || y >= g.MapHeight || x < 0 || x >= g.MapWidth {
		return false
	}
	for _, pos := range g.Path {
		if pos.Y == y && pos.X == x {
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
	g.Resources["chatgpt"] -= cost
	return true
}

// spawnEnemy deducts resources and adds an enemy to the field.
func (g *Game) spawnEnemy(enemyType string, _ map[string]interface{}) bool {
	costs := map[string]int{"basic": 20, "fast": 30, "tank": 50}
	cost, ok := costs[enemyType]
	if !ok {
		g.logf("Invalid enemy type: %s", enemyType)
		return false
	}
	if g.Resources["gemini"] < cost {
		return false
	}
	if len(g.Path) == 0 {
		return false
	}
	start := g.Path[0]
	en := NewEnemy(start.Y, start.X, enemyType, nil)
	g.Enemies = append(g.Enemies, &en)
	g.Resources["gemini"] -= cost
	return true
}

// spawnWave queues a mix of enemies and deducts resources.
func (g *Game) spawnWave() bool {
	waveCost := 40 + g.Wave*5
	if waveCost > 200 {
		waveCost = 200
	}
	if g.Resources["gemini"] < waveCost {
		return false
	}
	num := 5 + g.Wave
	if num > 30 {
		num = 30
	}
	for i := 0; i < num; i++ {
		switch {
		case g.Wave > 15:
			g.WaveQueue = append(g.WaveQueue, []string{"tank", "fast", "basic"}[i%3])
		case g.Wave > 5:
			g.WaveQueue = append(g.WaveQueue, []string{"fast", "basic", "tank"}[i%3])
		default:
			g.WaveQueue = append(g.WaveQueue, []string{"basic", "fast"}[i%2])
		}
	}
	g.Resources["gemini"] -= waveCost
	return true
}

// Note: min and max helpers are defined in core.go to avoid duplication.

// UpdateGameState advances the simulation by one tick. It performs four main duties:
// 1. Spawn any queued enemies (from Gemini's previously launched wave).
// 2. Move existing enemies along the pre-generated path and resolve leaks/damage to lives.
// 3. Let towers reduce their cooldowns and attack enemies in range, rewarding ChatGPT on kills.
// 4. Detect end-of-game conditions and update scores.
//
// This is intentionally lightweight – it is called ~10× per second by the Bubble-Tea TUI.
func (g *Game) UpdateGameState() {
	if g == nil || g.GameOver {
		return
	}

	// 1. Spawn queued enemies gradually – one per tick keeps things readable.
	if len(g.WaveQueue) > 0 {
		etype := g.WaveQueue[0]
		g.WaveQueue = g.WaveQueue[1:]
		if len(g.Path) > 0 {
			start := g.Path[0]
			en := NewEnemy(start.Y, start.X, etype, nil)
			g.Enemies = append(g.Enemies, &en)
		}
	}

	// 2. Towers act (cooldown & attack).
	for _, t := range g.Towers {
		if t.Cooldown > 0 {
			t.Cooldown--
		}
		if t.CanAttack() {
			killed := t.Attack(g.Enemies)
			for _, e := range killed {
				if e.Health <= 0 {
					// Reward ChatGPT and remove enemy; actual removal below
					g.Score["chatgpt"] += e.Reward
					g.Resources["chatgpt"] += e.Reward
				}
			}
		}
	}

	// 3. Move enemies & collect survivors.
	remaining := make([]*Enemy, 0, len(g.Enemies))
	for _, e := range g.Enemies {
		// Skip already-dead (health <=0) enemies – handled via tower kills.
		if e.Health <= 0 {
			continue
		}

		// Increase distance moved by speed each tick.
		e.DistanceMoved += e.Speed
		// Advance along the path while distance permits.
		for e.DistanceMoved >= 1.0 && e.PathIndex < len(g.Path)-1 {
			e.PathIndex++
			e.DistanceMoved -= 1.0
			p := g.Path[e.PathIndex]
			e.Pos = Position{Y: p.Y, X: p.X}
		}

		// Check if enemy reached the end of the path
		if e.PathIndex >= len(g.Path)-1 {
			g.Lives["chatgpt"]--
			if g.Lives["chatgpt"] <= 0 {
				g.GameOver = true
				g.Winner = "gemini"
			}
			continue // enemy removed
		}

		remaining = append(remaining, e)
	}
	g.Enemies = remaining

	// 4. Victory condition – Gemini has no enemies left and cannot launch more after max waves.
	if g.Lives["chatgpt"] > 0 && len(g.Enemies) == 0 && len(g.WaveQueue) == 0 {
		if g.Wave >= g.MaxWaves {
			g.GameOver = true
			g.Winner = "chatgpt"
		}
	}
}
