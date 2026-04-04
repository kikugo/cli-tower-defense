# LLM vs LLM Tower Defense

A terminal-based tower defense game written in Go where any two configured LLMs can compete in real-time.

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
       "timeout_seconds": 20
     }
   }
   ```

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

- Terminal resize handling is limited
- Some Unicode characters might not display correctly in all terminals
- API rate limiting may affect gameplay if decision intervals are too short

## License

MIT License

## Acknowledgments

- OpenAI and Google for their AI APIs
- The Go community for excellent libraries
