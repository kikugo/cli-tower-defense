package main

import (
	"encoding/json"
	"flag"
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
	game          *eng.Game
	width         int
	height        int
	paused        bool
	logScroll     int // how many lines from the bottom we offset when viewing logs
	showLogs      bool
	tickDur       time.Duration
	showRange     bool
	headless      bool
	maxTicks      int
	resultJSON    string
	replayJSON    string
	manifestJSON  string
	reportMD      string
	tournament    string
	tournamentCSV string
	tournamentMD  string
	balanceSweep  string
	ratingsJSON   string
	replayIn      string
	replayMode    bool
	replay        []eng.ReplayEvent
	replayIdx     int
	seed          int64
	ruleset       eng.ArenaRuleset
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
	manifestJSON := flag.String("manifest-json", "", "write run manifest JSON to this path")
	reportMD := flag.String("report-md", "", "write headless match report as markdown to this path")
	replayIn := flag.String("replay-input", "", "load replay JSON and view in replay mode")
	tournament := flag.String("tournament", "", "run tournament config JSON instead of a single TUI match")
	tournamentCSV := flag.String("tournament-csv", "", "write tournament standings CSV to this path")
	tournamentMD := flag.String("tournament-md", "", "write tournament markdown report to this path")
	balanceSweep := flag.String("balance-sweep", "", "run balance sweep config JSON and print win-rate table")
	ratingsJSON := flag.String("ratings-json", "", "path to read/write persistent model ratings for tournaments")
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
	appliedRuleset := eng.DefaultArenaRuleset()
	if *mapType != "" {
		g.SetMapType(*mapType)
		appliedRuleset.MapType = *mapType
	}
	if *maxWaves > 0 {
		g.MaxWaves = *maxWaves
		appliedRuleset.MaxWaves = *maxWaves
	}
	if *rulesetPreset != "" {
		ruleset, err := eng.PresetArenaRuleset(*rulesetPreset)
		if err != nil {
			log.Fatal(err)
		}
		g.ApplyRuleset(ruleset)
		appliedRuleset = ruleset
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
		appliedRuleset = ruleset
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
	m := model{game: g, tickDur: 100 * time.Millisecond, headless: *headless, maxTicks: *maxTicks, resultJSON: *resultJSON, replayJSON: *replayJSON, manifestJSON: *manifestJSON, reportMD: *reportMD, tournament: *tournament, tournamentCSV: *tournamentCSV, tournamentMD: *tournamentMD, balanceSweep: *balanceSweep, ratingsJSON: *ratingsJSON, replayIn: *replayIn, seed: *seed, ruleset: appliedRuleset}
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
		case "L":
			m.showLogs = !m.showLogs
		case "n", "right", "l":
			if m.replayMode {
				m.replayIdx = clampReplayIdx(m.replayIdx+1, len(m.replay))
			}
		case "b", "left", "h":
			if m.replayMode {
				m.replayIdx = clampReplayIdx(m.replayIdx-1, len(m.replay))
			}
		case "]":
			if m.replayMode {
				m.replayIdx = clampReplayIdx(m.replayIdx+10, len(m.replay))
			}
		case "[":
			if m.replayMode {
				m.replayIdx = clampReplayIdx(m.replayIdx-10, len(m.replay))
			}
		case "g", "home":
			if m.replayMode {
				m.replayIdx = 0
			}
		case "G", "end":
			if m.replayMode {
				m.replayIdx = clampReplayIdx(len(m.replay)-1, len(m.replay))
			}
		case "e":
			if m.replayMode {
				m.replayIdx = replayGameEndIndex(m.replay, m.replayIdx)
			}
		}
	}
	return m, nil
}

// ---- lipgloss styles ----
var (
	pathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // grey
	uiBorder  = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1)

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

// selectLogWindow picks the slice of raw g.Logs entries the 'L' debug pane
// shows: the most recent `budget` entries, offset by scroll (the up/down
// keys' existing m.logScroll semantics, unchanged from before this rewrite).
// This is the ONLY place raw log entries reach the screen now -- the move
// feed (renderMoveFeed) replaced them as the default pane, so a single
// multi-line g.Logs entry like the "=== Game State ===" block can no longer
// enter the layout uncounted: it goes through fitLines just like everything
// else, which forces it to exactly `budget` rows.
func selectLogWindow(logs []string, budget, scroll int) []string {
	if budget <= 0 || len(logs) == 0 {
		return nil
	}
	n := budget
	if n > len(logs) {
		n = len(logs)
	}
	start := len(logs) - n - scroll
	if start < 0 {
		start = 0
	}
	end := start + n
	if end > len(logs) {
		end = len(logs)
	}
	return logs[start:end]
}

// View renders the live match onto computeLayout's pane rects. Every pane is
// built to its exact rect (renderBoard/renderStats/renderMoveFeed all
// guarantee exact row counts; padCells guarantees exact column counts), then
// composed by plain []string concatenation (vstack) or side-by-side merge
// (hjoin) -- never by handing an unbounded string to Bubble Tea and hoping
// it fits. That is the direct fix for the defect main_view_test.go
// documents: Bubble Tea 0.26.1 clips to the last `height` lines rather than
// scrolling, so anything this function doesn't already fit is gone, not
// just off-screen.
func (m model) View() string {
	if m.replayMode {
		return m.replayView()
	}
	if m.game == nil {
		return "loading..."
	}

	lyt := computeLayout(m.width, m.height)
	if lyt.mode == layoutTooSmall {
		return tooSmallNotice(m.width, m.height)
	}

	if m.game.GameOver {
		return m.gameOverView(lyt)
	}

	statusRow := padCells(renderStatusText(m.game, m.tickDur, m.paused), lyt.w)
	keybarRow := padCells(renderKeyText(m.paused, m.game.AIEnabled, m.showLogs), lyt.w)

	viewportW := boardViewportWidth(m.game.MapWidth, lyt.board.w)
	panX := autoFollowPanX(m.game, viewportW)
	boardRows := renderBoard(m.game, lyt.board, panX, m.showRange)
	statsRows := renderStats(m.game, lyt.stats)
	sideRows := m.buildSideRows(lyt)

	body := composeBody(lyt, boardRows, statsRows, sideRows)
	all := vstack([]string{statusRow}, body, []string{keybarRow})
	return strings.Join(all, "\n")
}

// gameOverView renders the match-over screen: the frozen final board (the
// same state the live view would already be showing, since Update() stops
// calling UpdateGameState once GameOver is true -- nothing here recomputes
// anything) with a centered MATCH OVER result card spliced into it, plus the
// same stats pane and move feed/log pane the live view renders. This
// replaces the old bare "Game Over! Winner: %s\nPress q to quit." string,
// which discarded the board, score, wave, and cost at the exact moment they
// matter most (see main_view_test.go's TestGameOverContentPresence for the
// regression test that catches a return to that behavior -- the fit
// invariant alone cannot, since a two-line string fits every terminal).
func (m model) gameOverView(lyt layout) string {
	statusRow := padCells(renderGameOverStatusText(m.game), lyt.w)
	keybarRow := padCells(renderGameOverKeyText(m.showLogs), lyt.w)

	viewportW := boardViewportWidth(m.game.MapWidth, lyt.board.w)
	panX := autoFollowPanX(m.game, viewportW)
	cardMaxW, cardMaxH := boardInteriorSize(m.game, lyt.board)
	card := buildGameOverCard(m.game, cardMaxW, cardMaxH)
	boardRows := renderBoardWithCard(m.game, lyt.board, panX, m.showRange, card)
	statsRows := renderStats(m.game, lyt.stats)
	sideRows := m.buildSideRows(lyt)

	body := composeBody(lyt, boardRows, statsRows, sideRows)
	all := vstack([]string{statusRow}, body, []string{keybarRow})
	return strings.Join(all, "\n")
}

// buildSideRows selects the live view's right-hand/lower pane content: the
// move feed by default, or the raw log window when 'L' has toggled it on.
// Shared by View() and gameOverView() so the two paths can't drift in how
// they build this pane -- only the board and status/key text differ between
// them.
func (m model) buildSideRows(lyt layout) []string {
	if m.showLogs {
		return fitLines(selectLogWindow(m.game.Logs, lyt.moves.h, m.logScroll), lyt.moves.w, lyt.moves.h)
	}
	return renderMoveFeed(buildMoveFeed(m.game.ReplayEvents), lyt.moves.w, lyt.moves.h)
}

// composeBody assembles the board/stats/move-feed panes into the final body
// rows for a given layout: wide mode joins the board+stats left column
// beside the full-height move feed; compact/stacked modes stack all three
// vertically. Shared by the live view and the game-over view.
func composeBody(lyt layout, boardRows, statsRows, sideRows []string) []string {
	if lyt.mode == layoutWide {
		left := vstack(boardRows, statsRows)
		return hjoin(left, lyt.board.w, sideRows, lyt.moves.w)
	}
	return vstack(boardRows, statsRows, sideRows)
}

// waveProgressBar renders wave progress as a compact bar for the sidebar.
func waveProgressBar(wave, maxWaves, width int) string {
	if maxWaves <= 0 || width <= 0 {
		return fmt.Sprintf("Wave %d", wave)
	}
	filled := wave * width / maxWaves
	if filled > width {
		filled = width
	}
	return fmt.Sprintf("Wave %d/%d [%s%s]", wave, maxWaves,
		strings.Repeat("█", filled), strings.Repeat("─", width-filled))
}

// fmtCostMicros renders a USD-millionths cost as a dollar figure, or "-" when
// no cost has accrued (for example when no pricing is configured).
func fmtCostMicros(micros int64) string {
	if micros <= 0 {
		return "-"
	}
	return fmt.Sprintf("$%.4f", float64(micros)/1_000_000)
}

// clampReplayIdx keeps a replay index within [0, total-1].
func clampReplayIdx(idx, total int) int {
	if total <= 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx > total-1 {
		return total - 1
	}
	return idx
}

// replayGameEndIndex returns the index of the first game_end event, or the
// current index if none is present.
func replayGameEndIndex(events []eng.ReplayEvent, fallback int) int {
	for i, ev := range events {
		if ev.Type == eng.ReplayGameEnd {
			return i
		}
	}
	return clampReplayIdx(fallback, len(events))
}

// progressBar renders a simple textual timeline position indicator.
func progressBar(idx, total, width int) string {
	if total <= 1 || width <= 0 {
		return ""
	}
	pos := (idx * (width - 1)) / (total - 1)
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < width; i++ {
		if i == pos {
			b.WriteByte('|')
		} else {
			b.WriteByte('-')
		}
	}
	b.WriteByte(']')
	return b.String()
}

// replayView renders the replay viewer onto the same computeLayout budgets
// the live view uses. It replaces the pre-rewrite version, which built its
// right-hand panel by json.MarshalIndent-ing an event's ENTIRE Details map
// with no size bound -- for the map_init event that's the full paths array,
// measured at 396 rendered rows regardless of terminal size (finding 1.4).
// Here the same detail text is run through fitLines like everything else,
// so it is capped to its pane's budget no matter how large the underlying
// JSON is; nothing bypasses the layout.
func (m model) replayView() string {
	w, h := m.width, m.height
	if w == 0 || h == 0 {
		w, h = 80, 24
	}
	if w < 60 || h < 15 {
		return tooSmallNotice(m.width, m.height)
	}

	total := len(m.replay)
	if total == 0 {
		rows := fitLines([]string{"Replay mode: no events loaded", "Press q to quit."}, w, 2)
		return strings.Join(rows, "\n")
	}
	idx := clampReplayIdx(m.replayIdx, total)
	ev := m.replay[idx]

	lyt := computeLayout(w, h)

	statusText := fmt.Sprintf("Replay %d/%d %s · %s", idx+1, total, progressBar(idx, total, 24), ev.Type)
	statusRow := padCells(statusText, lyt.w)

	keyText := "space pause/resume · n/b step · [/] ±10 · g/G start/end · e game end · q quit"
	if m.paused {
		keyText = "PAUSED · " + keyText
	}
	keybarRow := padCells(keyText, lyt.w)

	snap := eng.ReconstructSnapshot(m.replay, idx+1)
	boardRows := renderReplayBoard(snap, ev.Position, lyt.board)

	detailWidth := lyt.w
	detailBudget := lyt.stats.h + lyt.moves.h
	if lyt.mode == layoutWide {
		detailWidth = lyt.moves.w
		detailBudget = lyt.moves.h
	}

	details := "{}"
	if len(ev.Details) > 0 {
		if data, err := json.MarshalIndent(ev.Details, "", "  "); err == nil {
			details = string(data)
		}
	}
	detailLines := []string{
		fmt.Sprintf("Player: %s  Role: %s", ev.PlayerID, ev.Role),
		fmt.Sprintf("Action: %s", ev.Action),
		fmt.Sprintf("Reason: %s", ev.Reason),
		"",
		"Board state (reconstructed)",
	}
	detailLines = append(detailLines, snap.SummaryLines()...)
	detailLines = append(detailLines, "", "Event details:")
	detailLines = append(detailLines, strings.Split(details, "\n")...)
	detailRows := fitLinesWithMoreIndicator(detailLines, detailWidth, detailBudget)

	var body []string
	if lyt.mode == layoutWide {
		body = hjoin(boardRows, lyt.board.w, detailRows, lyt.moves.w)
	} else {
		body = vstack(boardRows, detailRows)
	}

	all := vstack([]string{statusRow}, body, []string{keybarRow})
	return strings.Join(all, "\n")
}

func main() {
	m := initialModel()
	if m.balanceSweep != "" {
		if err := runBalanceSweep(m.balanceSweep); err != nil {
			log.Fatal(err)
		}
		return
	}
	if m.tournament != "" {
		if err := runTournament(m.tournament, m.ratingsJSON, m.tournamentCSV, m.tournamentMD); err != nil {
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
	m.game.ResolveTimeout()

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
	if m.manifestJSON != "" {
		manifest := eng.BuildRunManifest("headless", m.game, m.seed, false, limit, m.ruleset, os.Getenv("GIT_COMMIT"))
		if err := writeJSONFile(m.manifestJSON, manifest); err != nil {
			log.Printf("write manifest json: %v", err)
		}
	}
	if m.reportMD != "" {
		if err := os.WriteFile(m.reportMD, []byte(m.game.BuildMatchResult().MarkdownReport()), 0600); err != nil {
			log.Printf("write report markdown: %v", err)
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

func runTournament(path, ratingsPath, csvPath, mdPath string) error {
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
			result, manifest, err := runTournamentMatch(matchup, scheduled.Seed, config, scheduled.Swapped)
			if err != nil {
				return err
			}
			report.Results = append(report.Results, result)
			report.Manifests = append(report.Manifests, manifest)
		}
	}
	report.Standings = eng.SortStandings(eng.BuildTournamentStandings(report.Results))
	ratings := eng.DefaultModelRatings()
	if ratingsPath != "" {
		if raw, err := os.ReadFile(ratingsPath); err == nil {
			_ = json.Unmarshal(raw, &ratings)
		}
	}
	ratings.ApplyTournamentResults(report.Results)
	report.Ratings = ratings.Ratings
	if ratingsPath != "" {
		if err := writeJSONFile(ratingsPath, ratings); err != nil {
			return err
		}
	}

	if csvPath != "" {
		if err := os.WriteFile(csvPath, []byte(eng.StandingsCSV(report.Standings)), 0600); err != nil {
			return err
		}
	}
	if mdPath != "" {
		if err := os.WriteFile(mdPath, []byte(report.MarkdownReport()), 0600); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func runTournamentMatch(matchup eng.TournamentMatchup, seed int64, config eng.TournamentConfig, swapped bool) (eng.TournamentMatchResult, eng.ArenaRunManifest, error) {
	matchConfig := eng.MatchConfig{Player1: matchup.Player1, Player2: matchup.Player2}
	resolved, err := eng.ResolveMatchConfig(matchConfig)
	if err != nil {
		return eng.TournamentMatchResult{}, eng.ArenaRunManifest{}, err
	}
	g := eng.NewGameFromResolvedConfig(resolved)
	appliedRuleset := eng.DefaultArenaRuleset()
	g.PauseBetweenTurns = false
	g.AIDecisionInterval[g.Defender] = 0
	g.AIDecisionInterval[g.Attacker] = 0
	if config.MaxWaves > 0 {
		g.MaxWaves = config.MaxWaves
		appliedRuleset.MaxWaves = config.MaxWaves
	}
	if config.Ruleset != nil {
		g.ApplyRuleset(*config.Ruleset)
		appliedRuleset = *config.Ruleset
	}
	if seed != 0 {
		g.SetRandomSeed(seed)
	}
	if swapped {
		g.Defender, g.Attacker = g.Player2, g.Player1
		g.CurrentTurn = g.Defender
	}
	maxTicks := config.NormalizedMaxTicksForMain()
	runHeadlessSimulation(g, maxTicks)
	g.ResolveTimeout()
	result := eng.TournamentMatchResult{
		Matchup: matchup.Name,
		Seed:    seed,
		Swapped: swapped,
		Result:  g.BuildMatchResult(),
	}
	manifest := eng.BuildRunManifest("tournament", g, seed, swapped, maxTicks, appliedRuleset, os.Getenv("GIT_COMMIT"))
	return result, manifest, nil
}

func writeJSONFile(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
