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
	"github.com/joho/godotenv"
)

type tickMsg time.Time

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// turnPauseRatio is how many tick intervals the between-turn pause
// (eng.Game.PauseDuration) spans. The defaults (tickDur=100ms,
// PauseDuration=1s) are 10 tick-intervals apart; keeping that ratio as the
// speed control changes tickDur means the whole simulation's pacing scales
// together instead of just the tick rate. At the fastest setting (20ms/tick)
// the pause becomes 200ms; at the slowest (500ms/tick) it becomes 5s.
const turnPauseRatio = 10

// syncPauseDuration scales the engine's between-turn pause to match the
// current tick interval, preserving turnPauseRatio. It is a no-op when
// m.game is nil (replay mode has no engine.Game).
func (m *model) syncPauseDuration() {
	if m.game != nil {
		m.game.PauseDuration = m.tickDur * turnPauseRatio
	}
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
	// asciiMode folds the UI's box-drawing and block characters down to
	// ASCII at the output stage (render_theme_v2.go). It is a property of
	// the terminal, not of any pane, which is why it lives here and is
	// applied once rather than threaded through the renderers.
	asciiMode bool
	// hideLegend backs the '?' key. Both the board's bottom border and the
	// legend's own title row advertise '?', and before this field existed
	// neither did anything.
	//
	// It is phrased NEGATIVELY so the zero value is the default behaviour --
	// legend shown. The first version was `showLegend bool` set true in
	// initialModel, which meant every model literal in a test silently got
	// the opposite default; the golden snapshot caught it immediately, but
	// only because a golden existed. Same reasoning as ArenaRuleset's
	// DisableAssists, where the zero value keeps assists on.
	hideLegend bool
	replay     []eng.ReplayEvent
	replayIdx  int
	seed       int64
	ruleset    eng.ArenaRuleset
}

func initialModel() model {
	swap := flag.Bool("swap", false, "swap defender/attacker roles")
	defInt := flag.Int("def-int", 2, "defender decision interval (seconds)")
	attInt := flag.Int("att-int", 2, "attacker decision interval (seconds)")
	headless := flag.Bool("headless", false, "run simulation without TUI")
	maxTicks := flag.Int("max-ticks", 3000, "maximum ticks to run in headless mode")
	seed := flag.Int64("seed", 0, "deterministic random seed (0 uses time-based seed)")
	maxWaves := flag.Int("max-waves", 0, "override max waves (0 keeps default)")
	mapType := flag.String("map-type", "", "map archetype: straight, forked, choke, zigzag, open-field, switchback, perimeter")
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
	ascii := flag.Bool("ascii", false, "draw the UI with ASCII characters only (no box-drawing or block glyphs)")
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
	var rulesets []eng.ArenaRuleset
	if *rulesetPreset != "" {
		ruleset, err := eng.PresetArenaRuleset(*rulesetPreset)
		if err != nil {
			log.Fatal(err)
		}
		rulesets = append(rulesets, ruleset)
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
		rulesets = append(rulesets, ruleset)
	}
	appliedRuleset := applyGameConfiguration(g, *mapType, *maxWaves, rulesets, *seed)
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
	m := model{game: g, tickDur: 100 * time.Millisecond, headless: *headless, maxTicks: *maxTicks, resultJSON: *resultJSON, replayJSON: *replayJSON, manifestJSON: *manifestJSON, reportMD: *reportMD, tournament: *tournament, tournamentCSV: *tournamentCSV, tournamentMD: *tournamentMD, balanceSweep: *balanceSweep, ratingsJSON: *ratingsJSON, replayIn: *replayIn, seed: *seed, ruleset: appliedRuleset, asciiMode: *ascii}
	m.syncPauseDuration()
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
			m.syncPauseDuration()
		case "-":
			if m.tickDur < 500*time.Millisecond {
				m.tickDur = time.Duration(float64(m.tickDur) * 1.25)
			}
			m.syncPauseDuration()
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
		case "?":
			m.hideLegend = !m.hideLegend
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

	// The redesign. ViewV2 (main_view_v2.go) owns the live and game-over
	// screens for every terminal size; the old computeLayout path this
	// function used to take is gone with the renderers that fed it.
	return m.ViewV2()
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

// replayView renders the replay inspector. It now delegates to
// ReplayViewV2 (render_replay_v2.go), which uses computeLayoutV2's six-mode
// panes -- the same layout, glyph vocabulary, palette and ASCII fold the
// live view uses.
//
// The version this replaces built its panes on computeLayout and carried
// two things that are gone with it: a truncation banner row stolen out of
// the board's own budget with per-mode arithmetic to keep the total row
// count intact, and a right-hand panel that was a json.MarshalIndent dump
// with no move feed at all.
func (m model) replayView() string {
	return m.ReplayViewV2()
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
	finalModel, err := p.Run()
	if err != nil {
		log.Fatal(err)
	}

	fm, ok := finalModel.(model)
	if !ok || fm.game == nil || fm.replayIn != "" {
		return
	}

	// This print and the artifact writes happen after the alt-screen is torn
	// down (p.Run() has already returned), so they land on stdout normally
	// instead of being drawn over by the TUI -- a live TUI match against
	// real models must leave behind the same provenance record a headless
	// run does; see the block comment on writeMatchArtifacts.
	matchResult := fm.game.BuildMatchResult()
	ticks := int(fm.game.TickCount)
	printMatchSummary(fm.game, matchResult, "interactive", &ticks)
	writeMatchArtifacts(fm, matchResult, "interactive", 0)
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

	ticks := runHeadlessSimulation(m.game, limit)
	m.game.ResolveTimeout()

	matchResult := m.game.BuildMatchResult()
	printMatchSummary(m.game, matchResult, "headless", &ticks)

	writeMatchArtifacts(m, matchResult, "headless", limit)
}

// printMatchSummary prints the one-line provenance summary shared by the
// headless and interactive run paths. ticks is optional (nil omits the
// ticks field) because the interactive model does not track a local tick
// counter the way runHeadlessSimulation does.
func printMatchSummary(g *eng.Game, matchResult eng.MatchResult, mode string, ticks *int) {
	result := "incomplete"
	if g.GameOver {
		result = "completed"
	}
	ticksField := ""
	if ticks != nil {
		ticksField = fmt.Sprintf("ticks=%d | ", *ticks)
	}
	fmt.Printf("%s run %s | %swave=%d | winner=%s | defender_lives=%d | logs=%d | rejected_def=%d | rejected_att=%d | provider_err_def=%d | provider_err_att=%d | authored_def=%s | authored_att=%s\n",
		mode,
		result,
		ticksField,
		g.Wave,
		g.ModelNames[g.Winner],
		g.Lives[g.Defender],
		len(g.Logs),
		g.TotalRejectedActionsForPlayer(g.Defender),
		g.TotalRejectedActionsForPlayer(g.Attacker),
		g.TotalProviderErrorsForPlayer(g.Defender),
		g.TotalProviderErrorsForPlayer(g.Attacker),
		formatModelAuthoredShare(matchResult, g.Defender),
		formatModelAuthoredShare(matchResult, g.Attacker),
	)
}

// writeMatchArtifacts writes the four optional match-provenance artifacts
// (-result-json, -replay-json, -manifest-json, -report-md) if their
// corresponding flags were set on m. It is shared by the headless and
// interactive run paths so that a live TUI match against real models
// produces the same provenance record a headless run does.
func writeMatchArtifacts(m model, matchResult eng.MatchResult, mode string, limit int) {
	if m.resultJSON != "" {
		if err := writeJSONFile(m.resultJSON, matchResult); err != nil {
			log.Printf("write result json: %v", err)
		}
	}
	if m.replayJSON != "" {
		if err := writeJSONFile(m.replayJSON, m.game.ReplayEvents); err != nil {
			log.Printf("write replay json: %v", err)
		}
	}
	if m.manifestJSON != "" {
		manifest := eng.BuildRunManifest(mode, m.game, m.seed, false, limit, m.ruleset, os.Getenv("GIT_COMMIT"))
		if err := writeJSONFile(m.manifestJSON, manifest); err != nil {
			log.Printf("write manifest json: %v", err)
		}
	}
	if m.reportMD != "" {
		if err := os.WriteFile(m.reportMD, []byte(matchResult.MarkdownReport()), 0600); err != nil {
			log.Printf("write report markdown: %v", err)
		}
	}
}

// formatModelAuthoredShare renders MatchResult.ModelAuthored for the headless
// summary line, reusing the same "not measured" convention as
// engine.formatModelAuthored (engine/report_markdown.go): ModelAuthored
// returns (0, false) when provenance was not recorded, and that must never
// print as "0%".
func formatModelAuthoredShare(r eng.MatchResult, playerID string) string {
	share, ok := r.ModelAuthored(playerID)
	if !ok {
		return "not measured"
	}
	return fmt.Sprintf("%.0f%%", share*100)
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

// applyGameConfiguration applies map-type, max-waves, and ruleset
// configuration to g, then applies the random seed as the final step. Every
// entry point that builds a game (the TUI/headless path in initialModel and
// the tournament path in runTournamentMatch) must configure the game
// through this single function rather than calling SetMapType/ApplyRuleset/
// SetRandomSeed independently in its own order.
//
// This matters because SetMapType and ApplyRuleset (when the ruleset sets a
// map type) both regenerate the map from whatever RNG state exists at the
// time they're called, and SetRandomSeed replaces the RNG outright and also
// regenerates the map from it. If the seed is applied before those calls,
// or after a different number of them, the resulting map depends on the
// order and count of configuration calls instead of depending only on the
// seed. Applying the seed last, always, after every map-type/ruleset call,
// guarantees the final map depends only on (seed, final map type, final
// ruleset) — never on which entry point built the game or how many prior
// calls it took to get there.
//
// mapType is the explicit -map-type flag value (may be empty). rulesets are
// applied via ApplyRuleset in order (e.g. a preset ruleset followed by a
// ruleset file override); the last one with a non-empty MapType wins, same
// as before. seed of 0 means "no explicit seed" and leaves the game's
// existing (non-deterministic) RNG state untouched, as before.
func applyGameConfiguration(g *eng.Game, mapType string, maxWaves int, rulesets []eng.ArenaRuleset, seed int64) eng.ArenaRuleset {
	appliedRuleset := eng.DefaultArenaRuleset()
	if mapType != "" {
		g.SetMapType(mapType)
		appliedRuleset.MapType = mapType
	}
	if maxWaves > 0 {
		g.MaxWaves = maxWaves
		appliedRuleset.MaxWaves = maxWaves
	}
	for _, ruleset := range rulesets {
		g.ApplyRuleset(ruleset)
		appliedRuleset = ruleset
	}
	if seed != 0 {
		g.SetRandomSeed(seed)
	}
	return appliedRuleset
}

func runTournamentMatch(matchup eng.TournamentMatchup, seed int64, config eng.TournamentConfig, swapped bool) (eng.TournamentMatchResult, eng.ArenaRunManifest, error) {
	matchConfig := eng.MatchConfig{Player1: matchup.Player1, Player2: matchup.Player2}
	resolved, err := eng.ResolveMatchConfig(matchConfig)
	if err != nil {
		return eng.TournamentMatchResult{}, eng.ArenaRunManifest{}, err
	}
	g := eng.NewGameFromResolvedConfig(resolved)
	g.PauseBetweenTurns = false
	g.AIDecisionInterval[g.Defender] = 0
	g.AIDecisionInterval[g.Attacker] = 0
	var rulesets []eng.ArenaRuleset
	if config.Ruleset != nil {
		rulesets = append(rulesets, *config.Ruleset)
	}
	appliedRuleset := applyGameConfiguration(g, "", config.MaxWaves, rulesets, seed)
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
