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
		"towers":                 towers,
		"enemies":                enemies,
		"slow_zones":             slowZones,
		"obstacles":              obstacles,
		"valid_tower_candidates": g.validTowerCandidates(12),
		"pressure":               g.attackPressureSummary(),
		"visibility":             g.visibilitySummary(),
		"research":               g.researchSummary(),
		"attacker_abilities":     g.attackerAbilitySummary(),
		"director":               g.directorSummary(),
		"last_action_status":     copyStringMap(g.LastActionStatus),
		"last_rejected_reason":   copyStringMap(g.LastRejectedReason),
		"resources":              resourcesIface,
		"income":                 incomeIface,
		"lives":                  livesIface,
		"wave":                   g.Wave,
		"score":                  g.Score,
		"paths_count":            len(g.Paths),
		"wave_queue":             len(g.WaveQueue),
		"active_enemies":         len(g.Enemies),
	}
}

func (g *Game) getPlayerGameState(playerID, role string) map[string]interface{} {
	state := g.getGameState()
	if role != "defender" || !g.FogOfWar {
		return state
	}

	visibleEnemies := g.visibleEnemiesForDefender()
	filtered := make([]interface{}, len(visibleEnemies))
	for i, e := range visibleEnemies {
		pathLen := 1
		if e.PathID < len(g.Paths) {
			pathLen = len(g.Paths[e.PathID])
		}
		progress := float64(e.PathIndex) / float64(pathLen)
		filtered[i] = map[string]interface{}{
			"type":     e.EnemyType,
			"position": []int{e.Pos.Y, e.Pos.X},
			"health":   e.Health,
			"speed":    e.Speed,
			"shield":   e.Shield,
			"progress": progress,
			"path_id":  e.PathID,
		}
	}
	state["enemies"] = filtered
	state["active_enemies"] = len(filtered)
	state["visibility"] = g.visibilitySummary()
	_ = playerID
	return state
}

func (g *Game) validTowerCandidates(limit int) [][]int {
	candidates := make([][]int, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, path := range g.Paths {
		for _, pos := range path {
			for _, offset := range []Position{{Y: -1, X: 0}, {Y: 1, X: 0}, {Y: 0, X: -1}, {Y: 0, X: 1}} {
				y := pos.Y + offset.Y
				x := pos.X + offset.X
				key := tileKey(y, x)
				if _, ok := seen[key]; ok {
					continue
				}
				if ok, _ := g.canPlaceTowerAt(y, x); ok {
					seen[key] = struct{}{}
					candidates = append(candidates, []int{y, x})
					if len(candidates) >= limit {
						return candidates
					}
				}
			}
		}
	}
	return candidates
}

func (g *Game) attackPressureSummary() map[string]interface{} {
	return map[string]interface{}{
		"active_enemies":     len(g.Enemies),
		"queued_enemies":     len(g.WaveQueue),
		"defender_lives":     g.Lives[g.Defender],
		"defender_towers":    len(g.Towers),
		"attacker_resources": g.Resources[g.Attacker],
	}
}

func (g *Game) visibilitySummary() map[string]interface{} {
	visible := len(g.Enemies)
	if g.FogOfWar {
		visible = len(g.visibleEnemiesForDefender())
	}
	return map[string]interface{}{
		"fog_enabled":     g.FogOfWar,
		"vision_range":    g.DefenderVisionRange,
		"base_vision":     g.BaseVisionRange,
		"visible_enemies": visible,
		"hidden_enemies":  max(0, len(g.Enemies)-visible),
	}
}

func (g *Game) researchSummary() map[string]interface{} {
	summary := make(map[string]interface{}, len(g.ResearchLevels))
	for tech, level := range g.ResearchLevels {
		summary[tech] = level
	}
	return summary
}

func (g *Game) attackerAbilitySummary() []interface{} {
	abilities := availableAttackerAbilities()
	summary := make([]interface{}, 0, len(abilities))
	for _, ability := range abilities {
		current := ability
		current.CurrentCD = g.AbilityCooldowns[ability.Name]
		summary = append(summary, map[string]interface{}{
			"name":             current.Name,
			"cost":             current.Cost,
			"cooldown":         current.Cooldown,
			"current_cooldown": current.CurrentCD,
			"description":      current.Description,
		})
	}
	return summary
}

func (g *Game) rebuildEnemySpatialIndex() {
	g.EnemyTileIndex = make(map[string][]*Enemy, len(g.Enemies))
	for _, enemy := range g.Enemies {
		if enemy == nil || enemy.Health <= 0 {
			continue
		}
		key := tileKey(enemy.Pos.Y, enemy.Pos.X)
		g.EnemyTileIndex[key] = append(g.EnemyTileIndex[key], enemy)
	}
}

func (g *Game) enemiesNear(pos Position, radius int) []*Enemy {
	if radius < 0 {
		return nil
	}
	seen := map[*Enemy]struct{}{}
	nearby := make([]*Enemy, 0)
	for y := pos.Y - radius; y <= pos.Y+radius; y++ {
		for x := pos.X - radius; x <= pos.X+radius; x++ {
			for _, enemy := range g.EnemyTileIndex[tileKey(y, x)] {
				if _, ok := seen[enemy]; ok {
					continue
				}
				if distance(pos, enemy.Pos) <= float64(radius) {
					seen[enemy] = struct{}{}
					nearby = append(nearby, enemy)
				}
			}
		}
	}
	return nearby
}

func (g *Game) directorSummary() map[string]interface{} {
	return map[string]interface{}{
		"pressure_level":    g.PressureLevel,
		"pressure_triggers": g.PressureTriggers,
		"attacker_noop":     g.NoopStreak[g.Attacker],
		"quiet_board":       len(g.Enemies) == 0 && len(g.WaveQueue) == 0,
	}
}

func (g *Game) visibleEnemiesForDefender() []*Enemy {
	if !g.FogOfWar {
		return append([]*Enemy(nil), g.Enemies...)
	}
	visible := make([]*Enemy, 0, len(g.Enemies))
	for _, enemy := range g.Enemies {
		if g.isEnemyVisibleToDefender(enemy) {
			visible = append(visible, enemy)
		}
	}
	return visible
}

func (g *Game) isEnemyVisibleToDefender(enemy *Enemy) bool {
	if enemy == nil {
		return false
	}
	if enemy.Pos.X <= g.BaseVisionRange {
		return true
	}
	for _, tower := range g.Towers {
		if distance(tower.Pos, enemy.Pos) <= float64(max(tower.Range, g.DefenderVisionRange)) {
			return true
		}
	}
	for _, zone := range g.SlowZones {
		if distance(zone.Pos, enemy.Pos) <= float64(g.DefenderVisionRange) {
			return true
		}
	}
	return false
}

func (g *Game) researchTech(tech string) bool {
	costs := map[string]int{"economy": 180, "range": 160, "control": 140}
	cost, ok := costs[tech]
	if !ok {
		return false
	}
	if g.ResearchLevels[tech] >= 2 || g.Resources[g.Defender] < cost {
		return false
	}
	g.Resources[g.Defender] -= cost
	g.ResearchLevels[tech]++
	switch tech {
	case "economy":
		g.Income[g.Defender] += 3
	case "range":
		for _, tower := range g.Towers {
			tower.Range++
		}
	case "control":
		g.DefenderVisionRange++
	}
	return true
}

func availableAttackerAbilities() []AbilitySpec {
	return []AbilitySpec{
		{Name: "surge", Cost: 80, Cooldown: 12, Description: "Increase active enemy speed by 50%"},
		{Name: "shield_burst", Cost: 90, Cooldown: 14, Description: "Add temporary shield to active enemies"},
		{Name: "reinforce_wave", Cost: 70, Cooldown: 10, Description: "Add extra enemies to the queue"},
	}
}

func (g *Game) useAttackerAbility(name string) bool {
	specs := map[string]AbilitySpec{}
	for _, spec := range availableAttackerAbilities() {
		specs[spec.Name] = spec
	}
	spec, ok := specs[name]
	if !ok || g.AbilityCooldowns[name] > 0 || g.Resources[g.Attacker] < spec.Cost {
		return false
	}
	g.Resources[g.Attacker] -= spec.Cost
	g.AbilityCooldowns[name] = spec.Cooldown
	switch name {
	case "surge":
		for _, enemy := range g.Enemies {
			enemy.Speed *= 1.5
		}
	case "shield_burst":
		for _, enemy := range g.Enemies {
			enemy.Shield++
		}
	case "reinforce_wave":
		for _, enemyType := range []string{"fast", "basic", "shielded"} {
			if g.MaxWaveQueue > 0 && len(g.WaveQueue) >= g.MaxWaveQueue {
				break
			}
			g.WaveQueue = append(g.WaveQueue, enemyType)
		}
	}
	return true
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
	if ok, _ := g.canPlaceTowerAt(y, x); !ok {
		return false
	}
	tw := NewTower(y, x, towerType, nil)
	g.Towers = append(g.Towers, &tw)
	g.Resources[g.Defender] -= cost
	g.recordReplayEvent(ReplayEvent{
		Type:     ReplayPlacement,
		PlayerID: g.Defender,
		Role:     "defender",
		Action:   "place",
		Position: &Position{Y: y, X: x},
		Details:  map[string]interface{}{"tower_type": towerType, "cost": cost},
	})
	return true
}

func (g *Game) canPlaceTowerAt(y, x int) (bool, string) {
	// simple bounds/path/obstacle check
	if y < 0 || y >= g.MapHeight || x < 0 || x >= g.MapWidth {
		return false, "out_of_bounds"
	}
	if _, ok := g.PathTileSet[tileKey(y, x)]; ok {
		return false, "on_path"
	}
	if _, ok := g.ObstacleTileSet[tileKey(y, x)]; ok {
		return false, "on_obstacle"
	}
	for _, t := range g.Towers {
		if t.Pos.Y == y && t.Pos.X == x {
			return false, "occupied_by_tower"
		}
	}
	return true, ""
}

func (g *Game) findNearestTowerPlacement(startY, startX int, maxRadius int) (int, int, bool) {
	for r := 1; r <= maxRadius; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if abs(dy)+abs(dx) != r {
					continue
				}
				y := startY + dy
				x := startX + dx
				ok, _ := g.canPlaceTowerAt(y, x)
				if ok {
					return y, x, true
				}
			}
		}
	}
	return 0, 0, false
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
	if _, ok := g.PathTileSet[tileKey(y, x)]; !ok {
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
	g.recordReplayEvent(ReplayEvent{
		Type:     ReplaySpawn,
		PlayerID: g.Attacker,
		Role:     "attacker",
		Action:   "spawn",
		Position: &Position{Y: start.Y, X: start.X},
		Details:  map[string]interface{}{"enemy_type": enemyType, "cost": cost, "path_id": pathIdx},
	})
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
		if g.MaxWaveQueue > 0 && len(g.WaveQueue) >= g.MaxWaveQueue {
			break
		}
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
	g.recordReplayEvent(ReplayEvent{
		Type:     ReplayWave,
		PlayerID: g.Attacker,
		Role:     "attacker",
		Action:   "wave",
		Amount:   num,
		Details:  map[string]interface{}{"cost": waveCost, "wave": g.Wave, "queue": len(g.WaveQueue)},
	})
	return true
}

// UpdateGameState advances the simulation by one tick.
func (g *Game) UpdateGameState() {
	if g == nil || g.GameOver {
		return
	}
	g.TickCount++
	g.recordReplayEvent(ReplayEvent{
		Type: ReplayTick,
		Details: map[string]interface{}{
			"wave":    g.Wave,
			"enemies": len(g.Enemies),
			"towers":  len(g.Towers),
			"queue":   len(g.WaveQueue),
		},
	})

	// Passive income
	g.stateChangeCounter++
	if g.stateChangeCounter%10 == 0 {
		for p, inc := range g.Income {
			g.Resources[p] += inc
		}
	}
	for ability, cooldown := range g.AbilityCooldowns {
		if cooldown > 0 {
			g.AbilityCooldowns[ability] = cooldown - 1
		}
	}
	g.applyAdaptivePressure()

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
	g.rebuildEnemySpatialIndex()
	g.runTowerPhase()
	rebuildAfterTowerDamage := false
	for _, enemy := range g.Enemies {
		if enemy.Health <= 0 {
			rebuildAfterTowerDamage = true
			break
		}
	}
	if rebuildAfterTowerDamage {
		g.rebuildEnemySpatialIndex()
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
				if g.ResearchLevels["control"] > 0 {
					actualSpeed *= 0.85
				}
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
			g.recordReplayEvent(ReplayEvent{
				Type:     ReplayBreach,
				PlayerID: g.Attacker,
				Role:     "attacker",
				Action:   "breach",
				Position: &Position{Y: e.Pos.Y, X: e.Pos.X},
				Details:  map[string]interface{}{"enemy_type": e.EnemyType, "defender_lives": g.Lives[g.Defender]},
			})

			if g.Lives[g.Defender] <= 0 {
				g.GameOver = true
				g.Winner = g.Attacker
				g.recordReplayEvent(ReplayEvent{
					Type:     ReplayGameEnd,
					PlayerID: g.Attacker,
					Reason:   "defender_lives_depleted",
					Details:  map[string]interface{}{"winner": g.Winner, "wave": g.Wave},
				})
			}
			continue
		}

		remaining = append(remaining, e)
	}
	g.Enemies = remaining
	g.rebuildEnemySpatialIndex()

	// 4. Victory condition
	if g.Lives[g.Defender] > 0 && len(g.Enemies) == 0 && len(g.WaveQueue) == 0 && g.Wave >= g.MaxWaves {
		g.GameOver = true
		g.Winner = g.Defender
		g.recordReplayEvent(ReplayEvent{
			Type:     ReplayGameEnd,
			PlayerID: g.Defender,
			Reason:   "max_waves_cleared",
			Details:  map[string]interface{}{"winner": g.Winner, "wave": g.Wave},
		})
	}
}

func (g *Game) runTowerPhase() {
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

			candidates := g.enemiesNear(t.Pos, t.Range)
			killed := t.Attack(candidates)
			for _, e := range killed {
				g.Particles = append(g.Particles, &Particle{Pos: e.Pos, Char: '*', Lifetime: 2, Color: "red"})
				g.recordReplayEvent(ReplayEvent{
					Type:     ReplayDamage,
					PlayerID: g.Defender,
					Role:     "defender",
					Action:   "attack",
					Position: &Position{Y: e.Pos.Y, X: e.Pos.X},
					Details:  map[string]interface{}{"tower_type": t.TowerType, "enemy_type": e.EnemyType, "enemy_health": e.Health},
				})
				if e.Health <= 0 {
					g.Score[g.Defender] += e.Reward
					g.Resources[g.Defender] += e.Reward
				}
			}
			t.Damage = originalDamage
		}
	}
}

func (g *Game) applyAdaptivePressure() {
	if g.GameOver || g.TickCount%20 != 0 {
		return
	}
	quietBoard := len(g.Enemies) == 0 && len(g.WaveQueue) == 0
	if !quietBoard || g.NoopStreak[g.Attacker] < 3 {
		return
	}
	g.PressureTriggers++
	g.PressureLevel++
	if g.Resources[g.Attacker] >= 70 && g.AbilityCooldowns["reinforce_wave"] == 0 {
		_ = g.useAttackerAbility("reinforce_wave")
		return
	}
	if g.shouldAutoLaunchWave(g.Attacker) {
		_ = g.spawnWave()
		return
	}
	if g.Resources[g.Attacker] >= 20 && (g.MaxWaveQueue == 0 || len(g.WaveQueue) < g.MaxWaveQueue) {
		g.WaveQueue = append(g.WaveQueue, "basic")
	}
}

func distance(a, b Position) float64 {
	return math.Sqrt(math.Pow(float64(a.Y-b.Y), 2) + math.Pow(float64(a.X-b.X), 2))
}
