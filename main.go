package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	eng "tower-defense/engine"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joho/godotenv"
)

type tickMsg time.Time

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type model struct {
	game      *eng.Game
	width     int
	height    int
	paused    bool
	logScroll int // how many lines from the bottom we offset when viewing logs
}

func initialModel() model {
	_ = godotenv.Load()
	openaiKey := os.Getenv("OPENAI_API_KEY")
	googleKey := os.Getenv("GOOGLE_API_KEY")
	if openaiKey == "" || googleKey == "" {
		log.Fatal("OPENAI_API_KEY and GOOGLE_API_KEY must be set")
	}
	g := eng.NewGame(openaiKey, googleKey)
	return model{game: g}
}

func (m model) Init() tea.Cmd {
	return tickCmd(100 * time.Millisecond)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if !m.paused && m.game != nil && !m.game.GameOver {
			m.game.UpdateGameState()
			m.game.HandleAIDecisions()
		}
		return m, tickCmd(100 * time.Millisecond)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "space":
			m.paused = !m.paused
		case "up", "k":
			if m.logScroll < len(m.game.Logs)-1 {
				m.logScroll++
			}
		case "down", "j":
			if m.logScroll > 0 {
				m.logScroll--
			}
		}
	}
	return m, nil
}

// ---- lipgloss styles ----
var (
	pathStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // grey
	uiBorder     = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1)
	sidebarStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Width(26).Padding(0, 1)

	towerColor = map[string]lipgloss.Style{
		"basic":  lipgloss.NewStyle().Foreground(lipgloss.Color("219")), // magenta
		"splash": lipgloss.NewStyle().Foreground(lipgloss.Color("51")),  // cyan
		"sniper": lipgloss.NewStyle().Foreground(lipgloss.Color("45")),  // blue
	}

	enemyColorByType = map[string]lipgloss.Style{
		"basic": lipgloss.NewStyle().Foreground(lipgloss.Color("208")), // orange
		"fast":  lipgloss.NewStyle().Foreground(lipgloss.Color("226")), // yellow
		"tank":  lipgloss.NewStyle().Foreground(lipgloss.Color("201")), // magenta
	}
	enemyColorGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))  // healthy
	enemyColorYellow = lipgloss.NewStyle().Foreground(lipgloss.Color("226")) // mid
	enemyColorRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // low
)

func (m model) View() string {
	if m.game == nil {
		return "loading..."
	}
	if m.game.GameOver {
		return fmt.Sprintf("Game Over! Winner: %s\nPress q to quit.", m.game.Winner)
	}

	// --- Build rune grid ---
	grid := make([][]rune, m.game.MapHeight)
	for y := 0; y < m.game.MapHeight; y++ {
		grid[y] = make([]rune, m.game.MapWidth)
		for x := range grid[y] {
			grid[y][x] = ' '
		}
	}

	// Path glyphs
	for _, pos := range m.game.Path {
		if pos.Y >= 0 && pos.Y < len(grid) && pos.X >= 0 && pos.X < m.game.MapWidth {
			grid[pos.Y][pos.X] = '.'
		}
	}

	// Tower glyphs by type
	towerGlyph := map[string]rune{"basic": '^', "splash": '⊕', "sniper": '⌖'}
	for _, t := range m.game.Towers {
		glyph, ok := towerGlyph[t.TowerType]
		if !ok {
			glyph = '^'
		}
		y, x := t.Pos.Y, t.Pos.X
		if y >= 0 && y < len(grid) && x >= 0 && x < m.game.MapWidth {
			grid[y][x] = glyph
		}
	}

	// Pre-compute enemy position map for health colouring
	enemyAt := make(map[string]*eng.Enemy, len(m.game.Enemies))
	for _, e := range m.game.Enemies {
		key := fmt.Sprintf("%d,%d", e.Pos.Y, e.Pos.X)
		enemyAt[key] = e
	}

	rows := make([]string, m.game.MapHeight)
	for y := 0; y < m.game.MapHeight; y++ {
		var b strings.Builder
		for x, r := range grid[y] {
			switch r {
			case '.':
				b.WriteString(pathStyle.Render("."))
			case '^', '⊕', '⌖':
				// Determine tower type by glyph
				glyphType := map[rune]string{'^': "basic", '⊕': "splash", '⌖': "sniper"}[r]
				b.WriteString(towerColor[glyphType].Render(string(r)))
			case 'o', '>', '□':
				enKey := fmt.Sprintf("%d,%d", y, x)
				e := enemyAt[enKey]
				style := enemyColorByType["basic"]
				if e != nil {
					style = enemyColorByType[e.EnemyType]
					// Override with health colour
					ratio := float64(e.Health) / 100.0
					if ratio > 0.7 {
						style = enemyColorGreen
					} else if ratio > 0.3 {
						style = enemyColorYellow
					} else {
						style = enemyColorRed
					}
				}
				b.WriteString(style.Render(string(r)))
			default:
				b.WriteRune(r)
			}
		}
		rows[y] = b.String()
	}

	mapView := uiBorder.Render(strings.Join(rows, "\n"))

	// Sidebar with stats and logs
	turnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Bold(true)

	infoLines := []string{
		turnStyle.Render(fmt.Sprintf("Turn: %s", m.game.CurrentTurn)),
		fmt.Sprintf("Wave: %d", m.game.Wave),
		fmt.Sprintf("Lives: %d", m.game.Lives["chatgpt"]),
		fmt.Sprintf("Towers: %d", len(m.game.Towers)),
		fmt.Sprintf("Enemies: %d", len(m.game.Enemies)),
		fmt.Sprintf("Resources: %d", m.game.Resources["chatgpt"]),
		"",
		"Logs (↑/↓):",
	}

	// Determine how many log lines fit (sidebar width set to 26, assume ~ sidebar height = map height)
	maxLogs := 10
	if len(m.game.Logs) < maxLogs {
		maxLogs = len(m.game.Logs)
	}

	// Adjust window based on scroll offset
	start := len(m.game.Logs) - maxLogs - m.logScroll
	if start < 0 {
		start = 0
	}
	end := start + maxLogs
	if end > len(m.game.Logs) {
		end = len(m.game.Logs)
	}
	logsToShow := m.game.Logs[start:end]
	infoLines = append(infoLines, logsToShow...)

	sidebar := sidebarStyle.Render(strings.Join(infoLines, "\n"))

	// Combine map and sidebar horizontally using lipgloss
	ui := lipgloss.JoinHorizontal(lipgloss.Top, mapView, sidebar)

	// Footer
	footer := "(space) pause/resume, q quit"
	if m.paused {
		footer = "PAUSED | " + footer
	}
	return lipgloss.JoinVertical(lipgloss.Left, ui, footer)
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
