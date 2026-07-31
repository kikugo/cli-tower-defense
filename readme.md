# LLM vs LLM Tower Defense

A terminal-based tower defense game written in Go where any two configured LLMs can compete in real-time.

![Two models playing a match](docs/demo.gif)

## Game Overview

In this tower defense game, two configured models battle it out:

- **Defender**: Places towers and slow zones, upgrades defenses, invests in economy
- **Attacker**: Spawns enemies, launches waves, and invests in economy

The game runs in your terminal with a text-based interface, while the AI models make strategic decisions through API calls.

## Screenshots

```
ChatGPT vs Gemini Tower Defense
                                                   
Path: ···········································
Tower: ^                                     Enemy: o
                                                   
                                                   
                                                   
                                                   
                                                   
                                                   
                                                   
                                                   
ChatGPT: Lives: 20 | Resources: 250 | Score: 150    🤔
Gemini:  Resources: 200 | Score: 50 | Wave: 2        

ChatGPT decision: Placed basic tower
Gemini decision: Spawned fast enemy

[Q]uit [A]I: ON [+/-] Speed: 1.0x
Tower types: basic (^) sniper (⌖) splash (⊕) | Enemy types: basic (o) fast (>) tank (□)
```

## Features

- **Pure Go implementation** with no external language dependencies
- **Terminal-based interface** with simple text output
- **Real-time battles** between actual AI models via API calls
- **Multiple tower types**:
  - Basic Tower (^): Balanced damage and range
  - Sniper Tower (⌖): High damage, long range, slow fire rate
  - Splash Tower (⊕): Area damage affecting multiple enemies
- **Multiple enemy types**:
  - Basic Enemy (o): Balanced health and speed
  - Fast Enemy (>): Quick but fragile
  - Tank Enemy (□): Slow but high health
- **Resource management** for both AI players
- **Wave system** with increasing difficulty
- **AI decision-making** with configurable intervals
- **Game speed control** to speed up or slow down gameplay

## Installation

### Prerequisites

- Go 1.21 or higher
- API keys for the two providers/models you configure

### Steps

1. Clone the repository:
   ```bash
   git clone https://github.com/yourusername/chatgpt-vs-gemini-td.git
   cd chatgpt-vs-gemini-td
   ```

2. Install dependencies:
   ```bash
   go mod download
   go mod tidy
   ```

3. Set up model matchup:

   You can run with defaults (`OpenAI o3` vs `Gemini 2.5 Pro`) using:

   ```env
   OPENAI_API_KEY=your_openai_api_key_here
   GOOGLE_API_KEY=your_google_api_key_here
   ```

   Or define a custom matchup using either:
   - `MODEL_MATCH_CONFIG` (inline JSON)
   - `MODEL_MATCH_CONFIG_PATH` (path to JSON file)

   Example JSON:
   ```
   {
     "player1": {
       "provider": "openai_compatible",
       "model": "gpt-4.1-mini",
       "api_key_env": "OPENAI_API_KEY",
       "base_url": "https://api.openai.com/v1/chat/completions",
       "timeout_seconds": 20
     },
     "player2": {
       "provider": "openai_compatible",
       "model": "qwen/qwen3-32b",
       "api_key_env": "OPENROUTER_API_KEY",
       "base_url": "https://openrouter.ai/api/v1/chat/completions",
       "headers": {
         "HTTP-Referer": "https://example.com",
         "X-Title": "tower-defense"
       },
       "timeout_seconds": 20,
       "price_input_per_million": 0.15,
       "price_output_per_million": 0.60
     }
   }
   ```

   The optional `price_input_per_million` and `price_output_per_million` fields
   set USD pricing per one million tokens. When present, headless match results
   include an estimated cost derived from parsed provider token usage. The same
   fields work inside a `-profiles` catalog entry.

4. Run the game:

   You can either run the game directly:
   ```bash
   go run main.go
   ```

   Or build and run an executable:
   ```bash
   go build -o tower_defense
   ./tower_defense
   ```
   
   On Windows, use:
   ```
   tower_defense.exe
   ```

5. Verify API keys:

   If you're having issues with API connectivity, you can test your API keys:
   ```bash
   go run check_api.go
   ```

## Controls

- `Q`: Quit the game
- `A`: Toggle AI (on/off)
- `+` / `-`: Increase/decrease game speed
- `Space`: Pause/resume
- `R`: Toggle tower range preview
- `↑` / `↓` (or `K` / `J`): Scroll battle logs

### Replay viewer controls

When you open a replay with `-replay-input`, the timeline supports:

- `Space`: Pause/resume auto-play
- `n` / `→` and `b` / `←`: Step one event forward or back
- `]` / `[`: Jump 10 events forward or back
- `g` / `G`: Jump to the first or last event
- `e`: Jump to the game-end event

The sidebar shows the board reconstructed from the event stream at the current point: towers on the field, enemies spawned, kills, breaches, and the running result.

Replays recorded after the `map_init` event was introduced also draw the
board itself: path, obstacles, towers, breach markers, and the position of
the current event. Older replay files fall back to the text summary.

## Providers

Provider types:
- `openai_compatible`: OpenAI-style Chat Completions endpoints (OpenAI, OpenRouter, Groq-style integrations, etc.)
- `gemini_native`: Gemini `generateContent` endpoint

This lets you pitch any two configured models against each other without changing engine code.

## How It Works

1. The game creates a zigzag path across the screen
2. ChatGPT's AI makes decisions about tower placement and upgrades
3. Gemini's AI decides when to spawn enemies or launch waves
4. Both AIs receive the current game state and make decisions via API calls
5. The game continues until ChatGPT runs out of lives or you quit

## Technical Details

- Written entirely in Go with minimal dependencies
- Uses goroutines for non-blocking API calls
- Communicates with OpenAI and Google APIs for AI decisions
- Game state updates at configurable intervals

## Configuration

You can adjust game parameters from CLI flags:

- `-swap`: Swap defender/attacker roles
- `-def-int`: Defender decision interval in seconds
- `-att-int`: Attacker decision interval in seconds
- `-headless`: Run non-interactive simulation
- `-max-ticks`: Maximum ticks in headless mode
- `-seed`: Deterministic seed for reproducible runs
- `-max-waves`: Override max waves for short checks
- `-map-type`: Map archetype (`straight`, `forked`, `choke`, `zigzag`, `open-field`)
- `-result-json`: Write headless match summary JSON
- `-replay-json`: Write replay event stream JSON
- `-manifest-json`: Write run manifest JSON (reproducibility metadata)
- `-report-md`: Write the headless match summary as a markdown report
- `-replay-input`: Load replay JSON and open replay viewer mode
- `-ruleset-preset`: Arena ruleset preset (`default`, `fast`, `marathon`, `fair`)
- The `fair` preset disables engine assists (auto-wave, auto-defend, adaptive pressure) so results measure pure model decisions; any ruleset JSON can set `"disable_assists": true`
- `-ruleset`: Path to an arena ruleset JSON
- `-profiles`, `-player1-profile`, `-player2-profile`: Reusable model profile catalog + matchup selection
- `-tournament`: Run tournament config JSON (headless batch)
- `-tournament-csv`: Write ranked tournament standings as CSV
- `-tournament-md`: Write a tournament markdown report (standings + per-match results)
- `-balance-sweep`: Run a balance sweep config JSON (scripted duels across seeds) and print a defender win-rate table
- `-ratings-json`: Read/write persistent Elo-like model ratings across tournament runs

- `GameSpeed`: Controls how fast the game runs
- `AIDecisionInterval`: How often each AI makes decisions
- Tower and enemy attributes can be modified in their respective constructors

## Gameplay Check Guide

Automated smoke check:

```bash
go test ./...
go test -race ./...
go vet ./...
go run main.go -headless -seed=42 -max-ticks=2500 -max-waves=10
```

Replay + result export:

```bash
go run main.go -headless -seed=42 -max-ticks=1200 -result-json=match.json -replay-json=replay.json -report-md=match.md
go run main.go -replay-input=replay.json
```

`match.md` is a markdown summary: winner, win reason, wave progress, and a
per-player table of score, provider calls, average latency, token usage,
estimated cost, rejected actions, and errors.

Tournament run:

```bash
go run main.go -tournament=tournament.json -ratings-json=ratings.json -tournament-csv=standings.csv -tournament-md=tournament.md
```

`standings.csv` holds the standings ranked best-first (win rate, then average
normalized score), one row per model. `tournament.md` is a markdown report with
the same ranked standings plus a per-match results table. The JSON report on
stdout carries the full per-run results, manifests, and ratings.

## Arena Workflow

The end-to-end loop for comparing two models:

1. Define the matchup (env `MODEL_MATCH_CONFIG`, a `-profiles` catalog, or a
   `-tournament` config) and optionally a ruleset (`-ruleset-preset` or
   `-ruleset`).
2. Run reproducibly with a fixed `-seed`; export results with `-result-json`,
   `-replay-json`, `-manifest-json`, and `-report-md`.
3. Inspect a match after the fact with `-replay-input`, stepping and seeking
   through the timeline to see the reconstructed board at any point.
4. Run many seeds and role swaps with `-tournament`, persisting ratings with
   `-ratings-json` and exporting rankings with `-tournament-csv`.

All of these come from the same seeded run, so the result, replay, report, and
standings describe the same match. Fix the seed and you get the same outcome
every time.

Combat and economy numbers live in one config (`engine/balance.go`), and run
manifests record a `balance_version` so results from different eras aren't
compared by accident. To test a balance change, describe candidates in a sweep
JSON and run `go run . -balance-sweep=sweep.json`; each candidate plays
scripted duels across seeds and the table reports how often the baseline
defense held. A regression test pins the shipped defaults to that measurement.

Manual TUI check:

```bash
go run main.go -seed=42 -def-int=2 -att-int=2
```

During manual check, watch:
- turn cadence and queue growth
- provider error counters
- rejected action counters
- winner/wave progress

## Known Issues

- Terminals narrower than 84 columns show a resize notice; between 84 and 119 columns the sidebar stacks below the map
- Some Unicode characters might not display correctly in all terminals
- API rate limiting may affect gameplay if decision intervals are too short

## License

MIT License

## Acknowledgments

- OpenAI and Google for their AI APIs
- The Go community for excellent libraries
