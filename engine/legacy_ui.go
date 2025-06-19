//go:build legacy_ui

package engine

import (
	"fmt"
	"strings"
	"time"
)

// Simple UI function to display the game state in a cleaner way
func (g *Game) displayGameUI() {
	// Clear screen
	fmt.Print("\033[H\033[2J")

	// Header
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("ChatGPT vs Gemini Tower Defense - Wave: %d\n", g.Wave)
	fmt.Printf("ChatGPT - Lives: %d, Resources: %d, Score: %d | ",
		g.Lives["chatgpt"], g.Resources["chatgpt"], g.Score["chatgpt"])
	fmt.Printf("Gemini - Resources: %d, Score: %d\n",
		g.Resources["gemini"], g.Score["gemini"])
	fmt.Println(strings.Repeat("=", 80))

	// Create a grid for the map
	grid := make([][]rune, g.MapHeight)
	for i := range grid {
		grid[i] = make([]rune, g.MapWidth)
		// Fill with empty space
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	// Draw the path
	for _, pos := range g.Path {
		if pos.Y >= 0 && pos.Y < g.MapHeight && pos.X >= 0 && pos.X < g.MapWidth {
			grid[pos.Y][pos.X] = '.'
		}
	}

	// Draw towers
	for _, tower := range g.Towers {
		pos := tower.Pos
		if pos.Y >= 0 && pos.Y < g.MapHeight && pos.X >= 0 && pos.X < g.MapWidth {
			grid[pos.Y][pos.X] = tower.Char
		}
	}

	// Draw enemies
	for _, enemy := range g.Enemies {
		pos := enemy.Pos
		if pos.Y >= 0 && pos.Y < g.MapHeight && pos.X >= 0 && pos.X < g.MapWidth {
			grid[pos.Y][pos.X] = enemy.Char
		}
	}

	// Print the grid
	for _, row := range grid {
		fmt.Println(string(row))
	}

	// Footer
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("Active Towers: %d | Active Enemies: %d | Wave Queue: %d enemies\n",
		len(g.Towers), len(g.Enemies), len(g.WaveQueue))
	fmt.Printf("Current Turn: %s | Last Action: %s\n",
		g.CurrentTurn, g.LastDecisions[g.CurrentTurn])
	fmt.Println("Tower Types: Basic (^) | Sniper (⌖) | Splash (⊕)")
	fmt.Println("Enemy Types: Basic (o) | Fast (>) | Tank (□)")
	fmt.Println(strings.Repeat("-", 80))
}

func (g *Game) printGameState() {
	currentTime := time.Now()

	// Only print game state in these conditions:
	// 1. We haven't printed game state in at least 3 seconds
	// 2. The number of enemies or towers has changed
	// 3. Every 5th state change (to reduce output frequency)

	enemyCountChanged := g.lastEnemyCount != len(g.Enemies)
	towerCountChanged := g.lastTowerCount != len(g.Towers)
	timePassed := currentTime.Sub(g.lastStatePrintTime) > 3*time.Second

	if enemyCountChanged || towerCountChanged {
		g.stateChangeCounter++
	}

	shouldPrint := timePassed ||
		(enemyCountChanged && g.stateChangeCounter%5 == 0) ||
		(towerCountChanged && g.stateChangeCounter%5 == 0)

	if shouldPrint {
		g.logf("\n=== Game State ===\n")
		g.logf("Wave: %d", g.Wave)
		g.logf("Current Turn: %s", g.CurrentTurn)
		g.logf("ChatGPT - Lives: %d, Resources: %d, Score: %d",
			g.Lives["chatgpt"], g.Resources["chatgpt"], g.Score["chatgpt"])
		g.logf("Gemini - Resources: %d, Score: %d",
			g.Resources["gemini"], g.Score["gemini"])
		g.logf("Active Towers: %d, Active Enemies: %d",
			len(g.Towers), len(g.Enemies))
		g.logf("Wave Queue: %d enemies", len(g.WaveQueue))
		g.logf("==================\n")

		g.lastStatePrintTime = currentTime
		g.lastEnemyCount = len(g.Enemies)
		g.lastTowerCount = len(g.Towers)
	}
}
