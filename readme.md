# ChatGPT vs Gemini Tower Defense

A terminal-based tower defense game where OpenAI's ChatGPT and Google's Gemini AI models compete against each other in real-time.

## Game Overview

In this unique tower defense game, two AI models battle it out:

- **ChatGPT (Defender)**: Places towers to defend against incoming waves of enemies
- **Gemini (Attacker)**: Spawns enemies and launches waves to break through the defenses

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

- **Terminal-based interface** using tcell library
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

- Go 1.16 or higher
- OpenAI API key
- Google API key (for Gemini)

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

3. Set up API keys:
   
   Create a `.env` file in the project root:
   ```
   OPENAI_API_KEY=your_openai_api_key_here
   GOOGLE_API_KEY=your_google_api_key_here
   ```
   
   To get API keys:
   - OpenAI API key: Sign up at [OpenAI Platform](https://platform.openai.com/) and generate an API key
   - Google API key: Visit [Google AI Studio](https://makersuite.google.com/app/apikey) to get a Gemini API key
   
   **Important**: The `.env` file contains sensitive API keys. Add it to your `.gitignore` file to prevent accidentally pushing it to GitHub:
   ```
   # Add this to your .gitignore file
   .env
   ```

4. Build the game:
   ```bash
   go build -o tower-defense
   ```

5. Run the game:
   ```bash
   ./tower-defense
   ```
   
   On Windows, use:
   ```
   tower-defense.exe
   ```

## Controls

- `Q`: Quit the game
- `A`: Toggle AI (on/off)
- `+` / `-`: Increase/decrease game speed

## How It Works

1. The game creates a zigzag path across the screen
2. ChatGPT's AI makes decisions about tower placement and upgrades
3. Gemini's AI decides when to spawn enemies or launch waves
4. Both AIs receive the current game state and make decisions via API calls
5. The game continues until ChatGPT runs out of lives or you quit

## Technical Details

- Written in Go using the tcell library for terminal UI
- Uses goroutines for non-blocking API calls
- Communicates with OpenAI and Google APIs for AI decisions
- Game state updates at configurable intervals

## Configuration

You can adjust game parameters in the code:

- `GameSpeed`: Controls how fast the game runs
- `AIDecisionInterval`: How often each AI makes decisions
- Tower and enemy attributes can be modified in their respective constructors

## Known Issues

- Terminal resize handling is limited
- Some Unicode characters might not display correctly in all terminals
- API rate limiting may affect gameplay if decision intervals are too short

## License

MIT License

## Acknowledgments

- [tcell](https://github.com/gdamore/tcell) for the terminal UI library
- OpenAI and Google for their AI APIs