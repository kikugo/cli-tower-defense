package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
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
	game       *eng.Game
	width      int
	height     int
	paused     bool
	logScroll  int // how many lines from the bottom we offset when viewing logs
	tickDur    time.Duration
	showRange  bool
	headless   bool
	maxTicks   int
	resultJSON string
	replayJSON string
	tournament string
	replayIn   string
	replayMode bool
	replay     []eng.ReplayEvent
	replayIdx  int
}

func initialModel() model {
	swap := flag.Bool("swap", false, "swap defender/attacker roles")
	defInt := flag.Int("def-int", 2, "defender decision interval (seconds)")
	attInt := flag.Int("att-int", 2, "attacker decision interval (seconds)")
	headless := flag.Bool("headless", false, "run simulation without TUI")
	maxTicks := flag.Int("max-ticks", 3000, "maximum ticks to run in headless mode")
	seed := flag.Int64("seed", 0, "deterministic random seed (0 uses time-based seed)")
	maxWaves := flag.Int("max-waves", 0, "override max waves (0 keeps default)")
	mapType := flag.String("map-type", "", "map archetype: straight, forked, choke, zigzag, open-field")
	rulesetPreset := flag.String("ruleset-preset", "", "arena ruleset preset: default, fast, marathon")
	rulesetPath := flag.String("ruleset", "", "path to arena ruleset JSON")
	profilesPath := flag.String("profiles", "", "path to model profile catalog JSON")
	player1Profile := flag.String("player1-profile", "", "profile name for player1")
	player2Profile := flag.String("player2-profile", "", "profile name for player2")
	resultJSON := flag.String("result-json", "", "write headless match summary JSON to this path")
	replayJSON := flag.String("replay-json", "", "write headless replay event JSON to this path")
	replayIn := flag.String("replay-input", "", "load replay JSON and view in replay mode")
	tournament := flag.String("tournament", "", "run tournament config JSON instead of a single TUI match")
	flag.Parse()
	_ = godotenv.Load()
	var g *eng.Game
	if *profilesPath != "" || *player1Profile != "" || *player2Profile != "" {
		if *profilesPath == "" || *player1Profile == "" || *player2Profile == "" {
			log.Fatal("profiles mode requires -profiles, -player1-profile, and -player2-profile")
		}
		catalog, err := eng.LoadModelProfileCatalog(*profilesPath)
		if err != nil {
			log.Fatal(err)
		}
		matchCfg, err := eng.BuildMatchConfigFromProfiles(catalog, *player1Profile, *player2Profile)
		if err != nil {
			log.Fatal(err)
		}
		resolved, err := eng.ResolveMatchConfig(matchCfg)
		if err != nil {
			log.Fatal(err)
		}
		g = eng.NewGameFromResolvedConfig(resolved)
	} else {
		ng, err := eng.NewGameFromEnv()
		if err != nil {
			log.Fatal(err)
		}
		g = ng
	}
	if *seed != 0 {
		g.SetRandomSeed(*seed)
	}
	if *mapType != "" {
		g.SetMapType(*mapType)
	}
	if *maxWaves > 0 {
		g.MaxWaves = *maxWaves
	}
	if *rulesetPreset != "" {
		ruleset, err := eng.PresetArenaRuleset(*rulesetPreset)
		if err != nil {
			log.Fatal(err)
		}
		g.ApplyRuleset(ruleset)
	}
	if *rulesetPath != "" {
		var ruleset eng.ArenaRuleset
		raw, err := os.ReadFile(*rulesetPath)
		if err != nil {
			log.Fatalf("read ruleset: %v", err)
		}
		if err := json.Unmarshal(raw, &ruleset); err != nil {
			log.Fatalf("parse ruleset: %v", err)
		}
		g.ApplyRuleset(ruleset)
	}
	if *swap {
		g.Defender, g.Attacker = g.Player2, g.Player1
		g.CurrentTurn = g.Defender
	}
	g.AIDecisionInterval[g.Defender] = *defInt
	g.AIDecisionInterval[g.Attacker] = *attInt
	if *headless {
		g.PauseBetweenTurns = false
		// In headless mode, default intervals can make progress appear stalled.
		// If caller kept defaults, switch to immediate decisions.
		if *defInt == 2 {
			g.AIDecisionInterval[g.Defender] = 0
		}
		if *attInt == 2 {
			g.AIDecisionInterval[g.Attacker] = 0
		}
	}
	m := model{game: g, tickDur: 100 * time.Millisecond, headless: *headless, maxTicks: *maxTicks, resultJSON: *resultJSON, replayJSON: *replayJSON, tournament: *tournament, replayIn: *replayIn}
	if *replayIn != "" {
		var events []eng.ReplayEvent
		raw, err := os.ReadFile(*replayIn)
		if err != nil {
			log.Fatalf("read replay input: %v", err)
		}
		if err := json.Unmarshal(raw, &events); err != nil {
			log.Fatalf("parse replay input: %v", err)
		}
		m.replayMode = true
		m.replay = events
	}
	return m
}

func (m model) Init() tea.Cmd {
	return tickCmd(m.tickDur)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if m.replayMode {
			if !m.paused && m.replayIdx < len(m.replay)-1 {
				m.replayIdx++
			}
			return m, tickCmd(m.tickDur)
		}
		if !m.paused && m.game != nil && !m.game.GameOver {
			m.game.UpdateGameState()
			m.game.HandleAIDecisions()
		}
		return m, tickCmd(m.tickDur)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "space":
			m.paused = !m.paused
		case "+":
			if m.tickDur > 20*time.Millisecond {
				m.tickDur = time.Duration(float64(m.tickDur) * 0.8)
			}
		case "-":
			if m.tickDur < 500*time.Millisecond {
				m.tickDur = time.Duration(float64(m.tickDur) * 1.25)
			}
		case "a":
			if m.game != nil {
				m.game.AIEnabled = !m.game.AIEnabled
			}
		case "up", "k":
			if m.logScroll < len(m.game.Logs)-1 {
				m.logScroll++
			}
		case "down", "j":
			if m.logScroll > 0 {
				m.logScroll--
			}
		case "r":
			m.showRange = !m.showRange
		case "n", "right", "l":
			if m.replayMode && m.replayIdx < len(m.replay)-1 {
				m.replayIdx++
			}
		case "b", "left", "h":
			if m.replayMode && m.replayIdx > 0 {
				m.replayIdx--
			}
		}
	}
	return m, nil
}

// ---- lipgloss styles ----
var (
	pathStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // grey
	uiBorder     = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1)
	sidebarStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Width(35).Padding(0, 1)

	towerColor = map[string]lipgloss.Style{
		"basic":  lipgloss.NewStyle().Foreground(lipgloss.Color("219")), // magenta
		"splash": lipgloss.NewStyle().Foreground(lipgloss.Color("51")),  // cyan
		"sniper": lipgloss.NewStyle().Foreground(lipgloss.Color("45")),  // blue
		"buffer": lipgloss.NewStyle().Foreground(lipgloss.Color("202")), // orange/red
	}

	enemyColorByType = map[string]lipgloss.Style{
		"basic":    lipgloss.NewStyle().Foreground(lipgloss.Color("208")), // orange
		"fast":     lipgloss.NewStyle().Foreground(lipgloss.Color("226")), // yellow
		"tank":     lipgloss.NewStyle().Foreground(lipgloss.Color("201")), // magenta
		"shielded": lipgloss.NewStyle().Foreground(lipgloss.Color("46")),  // green/lime
		"healer":   lipgloss.NewStyle().Foreground(lipgloss.Color("123")), // light blue
	}
	enemyColorGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))  // healthy
	enemyColorYellow = lipgloss.NewStyle().Foreground(lipgloss.Color("226")) // mid
	enemyColorRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // low
	particleStyle    = map[string]lipgloss.Style{
		"red":   lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		"green": lipgloss.NewStyle().Foreground(lipgloss.Color("46")),
		"blue":  lipgloss.NewStyle().Foreground(lipgloss.Color("21")),
	}
)

// wrapText is a simple helper to wrap text to a certain width
func wrapText(text string, width int) string {
	if len(text) <= width {
		return text
	}
	return text[:width-3] + "..."
}

func (m model) View() string {
	if m.replayMode {
		return m.replayView()
	}
	if m.game == nil {
		return "loading..."
	}
	if m.game.GameOver {
		winnerName := m.game.ModelNames[m.game.Winner]
		if m.game.Winner == "none" {
			winnerName = "No one"
		}
		return fmt.Sprintf("Game Over! Winner: %s\nPress q to quit.", winnerName)
	}

	// --- Build rune grid ---
	grid := make([][]rune, m.game.MapHeight)
	for y := 0; y < m.game.MapHeight; y++ {
		grid[y] = make([]rune, m.game.MapWidth)
		for x := range grid[y] {
			grid[y][x] = ' '
		}
	}

	// Path glyphs with box drawing
	for _, path := range m.game.Paths {
		for i, pos := range path {
			if pos.Y >= 0 && pos.Y < len(grid) && pos.X >= 0 && pos.X < m.game.MapWidth {
				char := '·'
				if i > 0 && i < len(path)-1 {
					prev := path[i-1]
					next := path[i+1]
					if prev.Y == next.Y {
						char = '─'
					} else if prev.X == next.X {
						char = '│'
					} else {
						char = '┼'
					}
				}
				// Check for slow zone
				for _, sz := range m.game.SlowZones {
					if sz.Pos.Y == pos.Y && sz.Pos.X == pos.X {
						char = '≋'
						break
					}
				}
				grid[pos.Y][pos.X] = char
			}
		}
	}

	// Obstacles
	for _, obs := range m.game.Obstacles {
		if obs.Y >= 0 && obs.Y < len(grid) && obs.X >= 0 && obs.X < m.game.MapWidth {
			grid[obs.Y][obs.X] = '⬡'
		}
	}

	// Tower glyphs by type
	towerGlyph := map[string]rune{"basic": '^', "splash": '⊕', "sniper": '⌖', "buffer": 'B'}
	towerAt := make(map[string]*eng.Tower)
	for _, t := range m.game.Towers {
		glyph, ok := towerGlyph[t.TowerType]
		if !ok {
			glyph = '^'
		}
		y, x := t.Pos.Y, t.Pos.X
		if y >= 0 && y < len(grid) && x >= 0 && x < m.game.MapWidth {
			grid[y][x] = glyph
			key := fmt.Sprintf("%d,%d", y, x)
			towerAt[key] = t
		}
	}

	// Pre-compute enemy position map for health colouring
	enemyAt := make(map[string]*eng.Enemy, len(m.game.Enemies))
	for _, e := range m.game.Enemies {
		key := fmt.Sprintf("%d,%d", e.Pos.Y, e.Pos.X)
		enemyAt[key] = e
		if e.Pos.Y >= 0 && e.Pos.Y < len(grid) && e.Pos.X >= 0 && e.Pos.X < m.game.MapWidth {
			grid[e.Pos.Y][e.Pos.X] = e.Char
		}
	}

	// Particles
	for _, p := range m.game.Particles {
		if p.Pos.Y >= 0 && p.Pos.Y < len(grid) && p.Pos.X >= 0 && p.Pos.X < m.game.MapWidth {
			grid[p.Pos.Y][p.Pos.X] = p.Char
		}
	}

	// If range preview enabled, overlay range markers
	if m.showRange {
		for _, t := range m.game.Towers {
			for y2 := 0; y2 < m.game.MapHeight; y2++ {
				for x2 := 0; x2 < m.game.MapWidth; x2++ {
					dy := y2 - t.Pos.Y
					dx := x2 - t.Pos.X
					if dx*dx+dy*dy <= t.Range*t.Range {
						if grid[y2][x2] == ' ' {
							grid[y2][x2] = '•'
						}
					}
				}
			}
		}
	}

	// Pre-compute particle map
	particleAt := make(map[string]*eng.Particle)
	for _, p := range m.game.Particles {
		key := fmt.Sprintf("%d,%d", p.Pos.Y, p.Pos.X)
		particleAt[key] = p
	}

	rows := make([]string, m.game.MapHeight)
	for y := 0; y < m.game.MapHeight; y++ {
		var b strings.Builder
		for x, r := range grid[y] {
			// Check for particle first to render on top
			key := fmt.Sprintf("%d,%d", y, x)
			if p, ok := particleAt[key]; ok {
				b.WriteString(particleStyle[p.Color].Render(string(p.Char)))
				continue
			}

			switch r {
			case '·', '─', '│', '┼':
				b.WriteString(pathStyle.Render(string(r)))
			case '≋':
				b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(string(r)))
			case '⬡':
				b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(string(r)))
			case '^', '⊕', '⌖', 'B':
				glyphType := map[rune]string{'^': "basic", '⊕': "splash", '⌖': "sniper", 'B': "buffer"}[r]
				style := towerColor[glyphType]
				twKey := fmt.Sprintf("%d,%d", y, x)
				if t, ok := towerAt[twKey]; ok && t.Level > 0 {
					style = style.Copy().Bold(true).Underline(true)
				}
				b.WriteString(style.Render(string(r)))
			case 'o', '>', '□', 'S', 'H':
				enKey := fmt.Sprintf("%d,%d", y, x)
				e := enemyAt[enKey]
				style := enemyColorByType["basic"]
				if e != nil {
					style = enemyColorByType[e.EnemyType]
					// Health ratio using MaxHealth
					ratio := 1.0
					if e.MaxHealth > 0 {
						ratio = float64(e.Health) / float64(e.MaxHealth)
					}
					if ratio > 0.7 {
						style = enemyColorGreen
					} else if ratio > 0.3 {
						style = enemyColorYellow
					} else {
						style = enemyColorRed
					}
				}
				b.WriteString(style.Render(string(r)))
			case '•':
				b.WriteString(pathStyle.Render("•"))
			default:
				b.WriteRune(r)
			}
		}
		rows[y] = b.String()
	}

	mapView := uiBorder.Render(strings.Join(rows, "\n"))

	// Sidebar with stats and logs
	turnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Bold(true)

	defID := m.game.Defender
	attID := m.game.Attacker
	defName := m.game.ModelNames[defID]
	attName := m.game.ModelNames[attID]
	curName := m.game.ModelNames[m.game.CurrentTurn]

	p1ID := m.game.Player1
	p2ID := m.game.Player2
	p1Name := m.game.ModelNames[p1ID]
	p2Name := m.game.ModelNames[p2ID]
	p1Reason := wrapText(m.game.LastReasoning[p1ID], 30)
	p2Reason := wrapText(m.game.LastReasoning[p2ID], 30)
	p1Taunt := wrapText(m.game.LastTaunt[p1ID], 30)
	p2Taunt := wrapText(m.game.LastTaunt[p2ID], 30)

	infoLines := []string{
		turnStyle.Render(fmt.Sprintf("Turn: %s", curName)),
		fmt.Sprintf("Wave: %d", m.game.Wave),
		fmt.Sprintf("Tick: %d", m.game.TickCount),
		fmt.Sprintf("Queue: %d | Enemies: %d | Towers: %d", len(m.game.WaveQueue), len(m.game.Enemies), len(m.game.Towers)),
		fmt.Sprintf("Lives (%s): %d", defName, m.game.Lives[defID]),
		fmt.Sprintf("Resources (%s): %d (inc: %d)", defName, m.game.Resources[defID], m.game.Income[defID]),
		fmt.Sprintf("Resources (%s): %d (inc: %d)", attName, m.game.Resources[attID], m.game.Income[attID]),
		fmt.Sprintf("Pressure: active=%d queued=%d", len(m.game.Enemies), len(m.game.WaveQueue)),
		fmt.Sprintf("Provider errors: %s=%d %s=%d", p1Name, m.game.TotalProviderErrorsForPlayer(p1ID), p2Name, m.game.TotalProviderErrorsForPlayer(p2ID)),
		fmt.Sprintf("Rejected actions: %s=%d %s=%d", p1Name, m.game.TotalRejectedActionsForPlayer(p1ID), p2Name, m.game.TotalRejectedActionsForPlayer(p2ID)),
		fmt.Sprintf("Noop streak: %s=%d %s=%d", p1Name, m.game.NoopStreak[p1ID], p2Name, m.game.NoopStreak[p2ID]),
		fmt.Sprintf("Last status: %s=%s", p1Name, m.game.LastActionStatus[p1ID]),
		fmt.Sprintf("Last status: %s=%s", p2Name, m.game.LastActionStatus[p2ID]),
		fmt.Sprintf("Last reject: %s=%s", p1Name, wrapText(m.game.LastRejectedReason[p1ID], 20)),
		fmt.Sprintf("Last reject: %s=%s", p2Name, wrapText(m.game.LastRejectedReason[p2ID], 20)),
		"",
		"Strategy Reasoning:",
		fmt.Sprintf("%s: %s", p1Name, p1Reason),
		fmt.Sprintf("%s: %s", p2Name, p2Reason),
		"",
		"Battle Taunts:",
		fmt.Sprintf("%s: \"%s\"", p1Name, p1Taunt),
		fmt.Sprintf("%s: \"%s\"", p2Name, p2Taunt),
		"",
		"Logs (↑/↓):",
	}

	maxLogs := 10
	if len(m.game.Logs) < maxLogs {
		maxLogs = len(m.game.Logs)
	}

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

	ui := lipgloss.JoinHorizontal(lipgloss.Top, mapView, sidebar)

	speed := math.Round((100.0/float64(m.tickDur/time.Millisecond))*10) / 10
	aiStatus := "on"
	if !m.game.AIEnabled {
		aiStatus = "off"
	}
	footer := fmt.Sprintf("speed %.1fx | ai %s | (space) pause/resume, +/- adjust, a toggle ai, q quit", speed, aiStatus)
	if m.paused {
		footer = "PAUSED | " + footer
	}
	return lipgloss.JoinVertical(lipgloss.Left, ui, footer)
}

func (m model) replayView() string {
	total := len(m.replay)
	if total == 0 {
		return "Replay mode: no events loaded\nPress q to quit."
	}
	if m.replayIdx < 0 {
		m.replayIdx = 0
	}
	if m.replayIdx >= total {
		m.replayIdx = total - 1
	}

	ev := m.replay[m.replayIdx]
	left := uiBorder.Render(strings.Join([]string{
		"Replay Timeline",
		fmt.Sprintf("Event %d/%d", m.replayIdx+1, total),
		fmt.Sprintf("Type: %s", ev.Type),
		fmt.Sprintf("Player: %s", ev.PlayerID),
		fmt.Sprintf("Role: %s", ev.Role),
		fmt.Sprintf("Action: %s", ev.Action),
		fmt.Sprintf("Reason: %s", wrapText(ev.Reason, 58)),
		"",
		"Controls:",
		"space pause/resume",
		"n/right next event",
		"b/left previous event",
		"q quit",
	}, "\n"))

	details := "{}"
	if len(ev.Details) > 0 {
		if data, err := json.MarshalIndent(ev.Details, "", "  "); err == nil {
			details = string(data)
		}
	}
	right := sidebarStyle.Render(strings.Join([]string{
		"Arena Metrics",
		fmt.Sprintf("replay events: %d", total),
		fmt.Sprintf("at index: %d", m.replayIdx),
		fmt.Sprintf("timestamp: %s", ev.Time.Format(time.RFC3339)),
		"",
		"Event Details:",
		details,
	}, "\n"))

	ui := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	status := "running"
	if m.paused {
		status = "paused"
	}
	return lipgloss.JoinVertical(lipgloss.Left, ui, fmt.Sprintf("replay %s | +/- speed | q quit", status))
}

func main() {
	m := initialModel()
	if m.tournament != "" {
		if err := runTournament(m.tournament); err != nil {
			log.Fatal(err)
		}
		return
	}
	if m.headless {
		runHeadless(m)
		return
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

func runHeadless(m model) {
	if m.game == nil {
		fmt.Println("headless run failed: game is nil")
		return
	}
	limit := m.maxTicks
	if limit <= 0 {
		limit = 3000
	}

	ticks := 0
	ticks = runHeadlessSimulation(m.game, limit)

	result := "incomplete"
	if m.game.GameOver {
		result = "completed"
	}
	fmt.Printf("headless run %s | ticks=%d | wave=%d | winner=%s | defender_lives=%d | logs=%d | rejected_def=%d | rejected_att=%d | provider_err_def=%d | provider_err_att=%d\n",
		result,
		ticks,
		m.game.Wave,
		m.game.ModelNames[m.game.Winner],
		m.game.Lives[m.game.Defender],
		len(m.game.Logs),
		m.game.TotalRejectedActionsForPlayer(m.game.Defender),
		m.game.TotalRejectedActionsForPlayer(m.game.Attacker),
		m.game.TotalProviderErrorsForPlayer(m.game.Defender),
		m.game.TotalProviderErrorsForPlayer(m.game.Attacker),
	)

	if m.resultJSON != "" {
		if err := writeJSONFile(m.resultJSON, m.game.BuildMatchResult()); err != nil {
			log.Printf("write result json: %v", err)
		}
	}
	if m.replayJSON != "" {
		if err := writeJSONFile(m.replayJSON, m.game.ReplayEvents); err != nil {
			log.Printf("write replay json: %v", err)
		}
	}
}

func runHeadlessSimulation(g *eng.Game, limit int) int {
	ticks := 0
	for ticks < limit && !g.GameOver {
		if g.AIThinking[g.Player1] || g.AIThinking[g.Player2] {
			g.HandleAIDecisions()
			time.Sleep(10 * time.Millisecond)
			continue
		}
		g.UpdateGameState()
		g.HandleAIDecisions()
		ticks++
	}

	for i := 0; i < 200 && !g.GameOver; i++ {
		if !g.AIThinking[g.Player1] && !g.AIThinking[g.Player2] {
			break
		}
		g.HandleAIDecisions()
		time.Sleep(10 * time.Millisecond)
	}
	return ticks
}

func runTournament(path string) error {
	var config eng.TournamentConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return err
	}

	report := eng.TournamentReport{Name: config.Name}
	for _, matchup := range config.Matchups {
		for _, scheduled := range eng.BuildTournamentSchedule(config) {
			result, err := runTournamentMatch(matchup, scheduled.Seed, config, scheduled.Swapped)
			if err != nil {
				return err
			}
			report.Results = append(report.Results, result)
		}
	}
	report.Standings = eng.BuildTournamentStandings(report.Results)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func runTournamentMatch(matchup eng.TournamentMatchup, seed int64, config eng.TournamentConfig, swapped bool) (eng.TournamentMatchResult, error) {
	matchConfig := eng.MatchConfig{Player1: matchup.Player1, Player2: matchup.Player2}
	resolved, err := eng.ResolveMatchConfig(matchConfig)
	if err != nil {
		return eng.TournamentMatchResult{}, err
	}
	g := eng.NewGameFromResolvedConfig(resolved)
	g.PauseBetweenTurns = false
	g.AIDecisionInterval[g.Defender] = 0
	g.AIDecisionInterval[g.Attacker] = 0
	if config.MaxWaves > 0 {
		g.MaxWaves = config.MaxWaves
	}
	if config.Ruleset != nil {
		g.ApplyRuleset(*config.Ruleset)
	}
	if seed != 0 {
		g.SetRandomSeed(seed)
	}
	if swapped {
		g.Defender, g.Attacker = g.Player2, g.Player1
		g.CurrentTurn = g.Defender
	}
	runHeadlessSimulation(g, config.NormalizedMaxTicksForMain())
	return eng.TournamentMatchResult{
		Matchup: matchup.Name,
		Seed:    seed,
		Swapped: swapped,
		Result:  g.BuildMatchResult(),
	}, nil
}

func writeJSONFile(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
