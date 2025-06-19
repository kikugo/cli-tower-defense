package main

import (
	"fmt"
	"strings"
)

// Represents the game UI that renders the game state
type GameUI struct {
	game *Game
}

// Creates a new UI renderer
func NewGameUI(game *Game) *GameUI {
	return &GameUI{
		game: game,
	}
}

// Renders the current game state to the terminal
func (ui *GameUI) RenderGame() {
	// Clear screen
	fmt.Print("\033[H\033[2J")

	// Game stats header
	ui.renderHeader()

	// Game map
	ui.renderMap()

	// Game stats footer
	ui.renderFooter()
}

// Renders the game stats header
func (ui *GameUI) renderHeader() {
	width := 80
	headerLine := strings.Repeat("=", width)

	fmt.Println(headerLine)
	fmt.Printf("Tower Defense Battle - Wave: %d\n", ui.game.Wave)
	fmt.Printf("ChatGPT - Lives: %d, Resources: %d, Score: %d | ",
		ui.game.Lives["chatgpt"], ui.game.Resources["chatgpt"], ui.game.Score["chatgpt"])
	fmt.Printf("Gemini - Resources: %d, Score: %d\n",
		ui.game.Resources["gemini"], ui.game.Score["gemini"])
	fmt.Println(headerLine)
}

// Renders the game map with entities
func (ui *GameUI) renderMap() {
	// Create a grid for the map
	grid := make([][]rune, ui.game.MapHeight)
	for i := range grid {
		grid[i] = make([]rune, ui.game.MapWidth)
		// Fill with empty space
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	// First draw the path
	for _, pos := range ui.game.Path {
		if pos.Y >= 0 && pos.Y < ui.game.MapHeight && pos.X >= 0 && pos.X < ui.game.MapWidth {
			grid[pos.Y][pos.X] = '.'
		}
	}

	// Draw towers
	for _, tower := range ui.game.Towers {
		pos := tower.Pos
		if pos.Y >= 0 && pos.Y < ui.game.MapHeight && pos.X >= 0 && pos.X < ui.game.MapWidth {
			grid[pos.Y][pos.X] = tower.Char
		}
	}

	// Draw enemies
	for _, enemy := range ui.game.Enemies {
		pos := enemy.Pos
		if pos.Y >= 0 && pos.Y < ui.game.MapHeight && pos.X >= 0 && pos.X < ui.game.MapWidth {
			grid[pos.Y][pos.X] = enemy.Char
		}
	}

	// Print the grid
	for _, row := range grid {
		fmt.Println(string(row))
	}
}

// Renders game stats footer
func (ui *GameUI) renderFooter() {
	width := 80
	footerLine := strings.Repeat("-", width)

	fmt.Println(footerLine)
	fmt.Printf("Active Towers: %d | Active Enemies: %d | Wave Queue: %d enemies\n",
		len(ui.game.Towers), len(ui.game.Enemies), len(ui.game.WaveQueue))
	fmt.Printf("Current Turn: %s | Last Action: %s\n",
		ui.game.CurrentTurn, ui.game.LastDecisions[ui.game.CurrentTurn])

	// Tower types legend
	fmt.Printf("Tower Types: Basic (^): %d | Sniper (⌖): %d | Splash (⊕): %d\n",
		ui.countTowerType("basic"), ui.countTowerType("sniper"), ui.countTowerType("splash"))

	// Enemy types legend
	fmt.Printf("Enemy Types: Basic (o): %d | Fast (>): %d | Tank (□): %d\n",
		ui.countEnemyType("basic"), ui.countEnemyType("fast"), ui.countEnemyType("tank"))

	fmt.Println(footerLine)
}

// Helper to count towers by type
func (ui *GameUI) countTowerType(towerType string) int {
	count := 0
	for _, tower := range ui.game.Towers {
		if tower.TowerType == towerType {
			count++
		}
	}
	return count
}

// Helper to count enemies by type
func (ui *GameUI) countEnemyType(enemyType string) int {
	count := 0
	for _, enemy := range ui.game.Enemies {
		if enemy.EnemyType == enemyType {
			count++
		}
	}
	return count
}
