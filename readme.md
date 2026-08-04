# LLM vs LLM Tower Defense

A terminal tower defense game written in Go where any two configured LLMs compete in real time — and, more to the point, a small arena for measuring how they play.

![Two models playing a match](docs/demo.gif)

A real match, not a scripted one: `gemini-3-flash-preview` defending against `gemini-2.5-flash-lite` on seed 5. The telemetry panel shows live provider calls and token counts, and the attacker wins — the defender goes from 20 lives to 0 over six waves.

## Game Overview

Two configured models play opposite sides of the same board:

- **Defender**: places and upgrades towers, lays slow zones, researches tech, invests in economy
- **Attacker**: spawns enemies, launches waves, uses abilities, invests in economy

Turns alternate strictly. Each turn the engine renders the game state into a prompt, asks that player's model for exactly one JSON action, and applies it. Everything else — combat, pathing, economy — is deterministic given a seed.

## Features

- **Pure Go**, no external language dependencies
- **Provider-agnostic**: OpenAI-compatible endpoints, Gemini native, or scripted players for offline testing
- **Four tower types**: basic (`^`), sniper (`⌖`), splash (`⊕`), buffer (`B`)
- **Five enemy types**: basic (`o`), fast (`>`), tank (`□`), shielded (`S`), healer (`H`)
- **Bounded terminal layout** — the board, stats and move feed each get a row budget, so nothing scrolls off screen
- **Seeded, reproducible runs** with replay export and a timeline viewer
- **Tournaments** with role swap, ranked standings, and persistent Elo-style ratings
- **Balance sweeps** — scripted duels across seeds, so combat and economy changes can be measured without spending a token
- **Decision provenance**: every applied action records whether the model chose it or the engine substituted a fallback

## Installation

### Prerequisites

- Go 1.21 or higher
- API keys for the two providers you configure

### Steps

1. Clone and build:

   ```bash
   git clone https://github.com/kikugo/cli-tower-defense.git
   cd cli-tower-defense
   go build -o tower_defense
   ```

2. Configure a matchup. There is no default pairing — you must define one, via either:
   - `MODEL_MATCH_CONFIG` (inline JSON in `.env`)
   - `MODEL_MATCH_CONFIG_PATH` (path to a JSON file)

   ```json
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

   The optional `price_input_per_million` / `price_output_per_million` fields set USD pricing per million tokens. When present, match results carry an estimated cost derived from parsed provider token usage. The same fields work inside a `-profiles` catalog entry.

3. Run:

   ```bash
   ./tower_defense
   ```

   No API key is needed to try the offline paths — `provider: "scripted"` players, `-balance-sweep`, `-tournament` with scripted matchups, and the replay viewer all run without one.

## Controls

| key | action |
|---|---|
| `q` | quit |
| `space` | pause / resume |
| `+` / `-` | faster / slower |
| `a` | toggle AI |
| `r` | tower range overlay |
| `L` | toggle the raw log pane |
| `↑` `↓` (or `k` `j`) | scroll the move feed |

### Replay viewer

Open a recorded match with `-replay-input`:

| key | action |
|---|---|
| `space` | pause / resume auto-play |
| `n` `→` / `b` `←` | step one event forward / back |
| `]` / `[` | jump ±10 events |
| `g` / `G` | first / last event |
| `e` | jump to the game-end event |

The viewer reconstructs the board from the event stream: path, obstacles, towers, breach markers, and the position of the current event. Replays recorded before the `map_init` event was introduced fall back to a text summary.

### Terminal sizes

The layout is computed from the terminal, not assumed:

| width | layout |
|---|---|
| ≥ 116 | board and stats on the left, move feed in a full-height right column |
| 84–115 | board, stats and feed stacked |
| 60–83 | compact — the board renders a viewport into the fixed 80×14 map |
| < 60, or height < 15 | a size notice |

The simulation map stays 80×14 at every terminal size. Map dimensions affect path length and therefore balance, so the *viewport* is responsive and the *map* is not.

## How it works

1. The engine generates a seeded map — one or two lanes, with obstacles.
2. Each turn it builds a prompt for the player to move: current resources, the board, and the JSON templates for the actions that player can currently afford.
3. The model returns one JSON object. The engine parses, validates and applies it, or records a rejection.
4. Combat, movement and income resolve on a fixed tick.
5. The match ends when the defender's lives reach zero, the wave cap is cleared, or the tick budget runs out.

Provider calls are asynchronous — network latency never blocks the event loop, and a provider failure is recovered and logged rather than crashing the match.

## Measuring the arena

This is a benchmark before it is a game, so what follows is what has actually been measured, how, and where the numbers stop being trustworthy.

### The defender used to win 92% of the time

When I started running matches between two models, the defender won every one. Ten in a row across six seeds, with the roles swapped halfway through, so whichever model defended won. The arena was measuring which seat you sat in, not which model was better.

The cause was economic. When a tower killed an enemy the defender received that enemy's reward as **spendable resources**, while the attacker earned resources only when a unit reached the end of the path. The attacker paid 50 to send a tank, the defender collected 50 when it died, and nothing came back the other way unless something got through. That compounds: kills fund towers, towers produce kills, and eventually the attacker cannot afford to spawn at all. In one match the attacker finished 415 to 0 having never breached once, and 130 of its turns were spent proposing spawns it could not pay for.

The fix is one line. A kill now awards score and not resources, so both sides fund themselves from income alone.

Measured with the built-in balance sweep, scripted players on both sides so no model is involved, 40 seeds each:

| | defender wins | win rate | avg match length |
|---|---|---|---|
| Kills award resources | 37/40 | 92% | 1403 ticks |
| Kills award score only | 26/40 | 65% | 1065 ticks |

That comparison is sound — both arms ran on the same map generator, so everything below is common to them and cancels in the difference. The fix stays.

A later measurement put a number on how big a lever it was: kills were worth about 2.07 resources/tick against a base income of 5.00. Real, but small. It is not worth revisiting.

### The residual 62% is not a balance property

The shipped defaults now measure **25/40 = 62.5%** across the default generator (40 seeds, 400 ticks, scripted players). It is tempting to read that as "the game leans slightly toward the defender." It is not what the number means.

Stratifying the same 40 seeds by how many lanes the generator produced:

| lanes | seeds | defender held |
|---|---|---|
| 1 | 24 (60%) | **24/24 = 100%** |
| 2 | 16 (40%) | **1/16 = 6%** |
| — | 40 | 25/40 = 62.5% |

`generatePaths` picks one lane unless a random draw exceeds 0.6, so `P(1 lane) ≈ 0.60`, and `0.60 × 100% + 0.40 × 6% = 62.4%`. **The headline number is, to within rounding, the probability of the RNG rolling a single lane.** It is stable at both 400 and 3000 tick caps because the lane count decides the match long before the cap does.

Per archetype, 40 seeds each:

| map | lanes | defender held |
|---|---|---|
| straight, choke, zigzag, switchback, perimeter | 1 | 40/40 = 100% |
| open-field | 3 | 7/40 = 18% |
| forked | 2 | 0/40 = 0% |

Stratifying also overturned an earlier conclusion of my own. Aggregated, tower range looked like the one smooth tuning axis — 60%, 62%, 72%, 100% at range 4 through 7. Stratified, it is a mixture: the single-lane subset is pinned at 100% for *every* range from 1 to 7, and all the movement comes from the two-lane subset. And a 40–60% band that earlier work concluded "does not exist on any knob axis" does exist — `forked` at tower range 6 measures **45%** (18/40). It was invisible because the single-lane majority pins any aggregate above 58%.

The sweep now reports per stratum and will not print an unlabelled aggregate. The combined row is called `MIXTURE`, carries its composition inline, refuses to show a rate for a stratum with fewer than ten matches, and is suppressed entirely when one stratum exceeds 90% of the design:

```
candidate                | stratum   | n   | def wins | rate          | avg ticks | avg def score
defaults                 | lanes=1   |  24 | 24/24    | 100%          |     400.0 | 760.0
defaults                 | lanes=2   |  16 |  1/16    | 6%            |     188.4 | 498.8
defaults                 | MIXTURE   |  40 | 25/40    | 62%           |     315.4 | 655.5   [lanes=1:60% lanes=2:40%]
```

Each row also carries a second line reporting what the arena recorded during those matches — total rejected actions (split defender/attacker), total provider calls, and the model-authored share — alongside the four simulation numbers above. The two are not redundant: a change can leave win rate, ticks, and score bit-identical while altering what got recorded, as `skip_forced_save_turns` does below (same four simulation numbers, rejected actions from 4526 to 0 across the same 40 seeds). The simulation columns alone would report that change as invisible.

### What the balance harness actually exercises

Less than it looks like. Across 40 scripted duels, counting every entity the engine actually instantiated:

- **towers built: `basic` only.** `sniper`, `splash` and `buffer` never appear.
- **enemies spawned: `basic` and `fast` only.** `tank`, `shielded` and `healer` never appear.

Two independent causes. The scripted measuring-stick players hardcode `basic`, and wave composition gates the remaining enemy types behind wave 6 and wave 16 while the wave counter never exceeds 2 — raising the wave cap to 20 or 30 does not change that.

So the balance numbers describe a **one-tower, two-enemy game**. Deliberately absurd overrides confirm the harness is blind to the rest: making `sniper` cost 10, giving `splash` 200 damage at range 9, making `buffer` free, or removing `shielded`'s shield entirely all produce byte-identical results. Six of ten unit types have no regression protection at all.

Two measurements of that unexercised content follow. Both were run twice: the first attempt got a confounded answer, and the confound was in the experiment design, not the engine.

### The towers are not close

Scripted defenders that commit to a single tower type, on `forked` (two lanes), 40 seeds. The first attempt gave every non-basic tower 0% and looked conclusive, but it wasn't measuring what it claimed: with a 300 starting bank, `basic` buys three towers and `sniper` buys one, so it compared a tower against a tower-plus-idle-capital. At a matched 600 budget each script spends roughly all of it — six basics, three splash, two snipers, two buffers.

Hold rate has no resolution on `forked` (every script loses), so the metric is defender score, which on this engine is the sum of the rewards of everything it killed:

| defender builds | avg defender score, 600 budget |
|---|---|
| `basic` | **842.6** |
| `sniper` | 103.4 |
| `splash` | 0.0 |
| `buffer` | 0.0 |

Eight to one against the sniper, and the other two never register a kill. This matches the arithmetic: per 100 resources, `basic` delivers 11.3 damage/tick, `splash` 3.8, `sniper` 1.3 and `buffer` none. `basic` is not the balanced starter tower, it is the only economical one.

One caveat: starting resources are a per-ruleset value applied to both players, so the 300 and 600 blocks are not comparable *to each other* — only the four scripts within a block are.

### The shield mechanic breaks the game

The attacker side got the same treatment. Rather than add a script that spawns `shielded` — the first attempt did that, and since it never launched a wave it measured spawn tempo instead of unit quality — the unit `attacker_baseline` already spawns was restatted, leaving its wave logic untouched. 40 seeds, default generator:

| the attacker's staple unit is… | defender held | lanes=1 | lanes=2 |
|---|---|---|---|
| A — `basic` as shipped | 25/40 = 62% | 24/24 | 1/16 |
| B — given `shielded`'s full profile (150hp, 0.8 speed, shield 2, cost 40) | **0/40 = 0%** | **0/24** | 0/16 |
| C — shield 2 only, everything else unchanged | **0/40 = 0%** | **0/24** | 0/16 |
| D — `shielded`'s body and price but **no** shield | 29/40 = 72% | 24/24 | 5/16 |

Read C against D. Giving the attacker's basic unit a shield and changing nothing else — same health, same speed, same 20 cost — takes the defender from 62% to **zero**, and does it on the single-lane maps that are otherwise 100% defender wins at every tower range from 1 to 7. Giving it the shielded unit's larger body and higher price *without* the shield makes things *easier* for the defender, at 72%.

So it is not the health and not the cost. It is `damage /= (shield + 1)`, integer division: a basic tower's 34 becomes 11, and a 150-health unit that took three shots now takes fourteen.

Shielded is a dominant attacker strategy, it has never been exercised by any balance measurement, and the prompt describes the mechanic to every attacker in plain language. Nothing has been retuned on the strength of this — it is a measurement, and the fix belongs with a rethink of how shields interact with the shot-count thresholds the whole balance sits on.

### Tank is overpriced, not weak

The same substitution applied to the remaining unmeasured units. It is faithful for `tank`, whose behaviour is entirely its stat line, and only partly faithful for `healer` — the heal is keyed on the unit's type name (`actions.go`), not on a stat, so restatting gives you a healer's body without its ability.

| the attacker's staple unit is… | defender held |
|---|---|
| `basic` as shipped | 25/40 = 62% |
| `tank`'s full profile — 300hp, 0.5 speed, cost 50 | 36/40 = **90%** |
| `tank`'s health only — 300hp at basic's speed and 20 cost | **0/40 = 0%** |
| `tank`'s speed only — 0.5 speed at basic's health and cost | 26/40 = 65% |
| `healer`'s body only — 80hp, cost 30 (no heal) | 27/40 = 68% |

Tank's health is devastating and its price more than cancels it. Three hundred health at 20 resources ends every match; the same unit at its real 50 makes the attacker *worse off than spawning basics*, because half speed doubles the time spent inside tower range, so 3× the health buys only about 1.5× the survivability for 2.5× the price. Speed on its own barely moves anything. Tank is not weak content, it is mispriced content — and nothing had ever measured it either way.

The healer's ability needed a different method. It keys on the unit's *type name* rather than on a stat, so a restatted `basic` gets a healer's body and never heals — which is what the 68% above measures. A dedicated scripted attacker that spawns real `healer` units, identical to the baseline in every other respect including its wave logic, gives **26/40 (65%)** against the body-only 27/40 (68%). A one-match swing across 40 seeds: the ability is worth nothing measurable. Of the four units the harness never exercises, one is decisive, one is mispriced, and two are inert.

### Whose decision was it?

When a model's response cannot be parsed, or its provider call fails, the engine used to substitute a decision of its own — a tower at (10,10), a `basic` spawn, a `save` — and record it as though the model had chosen it. Seven of nine substitution points serialized identically to a genuine decision.

The consequence is visible in this repository's own captured reference match, `live_fair_replay.json`. It contains 90 decision events and **three distinct decision objects**, all engine literals:

```
45x  {"action":"spawn","enemy_type":"basic","reason":"Default fallback"}
37x  {"action":"place","position":[10,10],"tower_type":"basic","reason":"Default fallback"}
 8x  {"action":"save","reason":"provider request failed"}
```

Not one decision in that match came from a model, and nothing in the recorded output said so — the provider-error counter read zero despite eight known failures, because both HTTP providers returned a nil error. Thirty-two of the forty-eight rejections in that match are the parser's hardcoded `[10,10]` colliding with itself, turn after turn.

Matches now record this. Every applied decision is tagged with its source, a provider failure skips the turn instead of substituting, provider errors are counted for the first time, and `MatchResult` carries a per-player `model_authored_share`. Artifacts written before provenance existed report *"not measured"* rather than defaulting to "100% model" — the distinction is the whole point.

**No live match recorded before this change can be shown to have measured a model rather than the engine.** Conclusions drawn from those runs carry an uncertainty nobody can now quantify.

### What is not claimed

- That the balance is tuned for anything beyond `basic`-vs-`basic` play.
- That the scripted duel proxy predicts model behaviour. It does not currently agree with it: the scripted attacker launches waves, and across a 12-match live tournament no model launched a single one.
- That Elo separates models here. Under a systematic seat advantage with role swap, every pairing goes 1–1 and ratings return to where they started.
- That the shipped balance survives contact with the content it has never measured. On the evidence above, it does not.

## Arena workflow

The end-to-end loop for comparing two models:

1. Define the matchup (`MODEL_MATCH_CONFIG`, a `-profiles` catalog, or a `-tournament` config) and optionally a ruleset (`-ruleset-preset` or `-ruleset`).
2. Run with a fixed `-seed`; export with `-result-json`, `-replay-json`, `-manifest-json` and `-report-md`.
3. Inspect the match afterwards with `-replay-input`, stepping through the timeline.
4. Run many seeds and role swaps with `-tournament`, persisting ratings with `-ratings-json`.

Combat and economy numbers live in one config (`engine/balance.go`). Run manifests record a `balance_version` label and a `balance_hash` derived from the numbers themselves — the label is for humans and does not change when a sweep retunes a stat, so the hash is what actually detects drift. Manifests also record the resolved provider configuration, including the temperature that produced the result (it defaults to 0.7, so live comparisons carry sampling noise whether or not anyone chose it). API keys are never recorded.

To test a balance change, describe candidates in a sweep JSON and run `-balance-sweep`; each candidate plays scripted duels across seeds and the table reports how often the baseline defense held. A regression test pins the shipped defaults to that measurement.

```bash
# offline, no API key needed
./tower_defense -balance-sweep sweep.json
./tower_defense -headless -seed 42 -max-ticks 1200 \
    -result-json match.json -replay-json replay.json -report-md match.md
./tower_defense -replay-input replay.json

# a tournament
./tower_defense -tournament tournament.json -ratings-json ratings.json \
    -tournament-csv standings.csv -tournament-md tournament.md
```

`match.md` is a markdown summary: winner, win reason, wave progress, and a per-player table of score, provider calls, average latency, token usage, estimated cost, authored share, rejected actions and errors.

## Configuration

```
-swap                  swap defender/attacker roles
-def-int  -att-int     decision interval per side, seconds
-headless -max-ticks   non-interactive simulation and its tick budget
-seed                  deterministic seed
-max-waves             override the wave cap
-map-type              straight, forked, choke, zigzag, open-field, switchback, perimeter
-ruleset-preset        default, fast, marathon, fair
-ruleset               path to an arena ruleset JSON
-profiles -player1-profile -player2-profile
-result-json -replay-json -manifest-json -report-md
-replay-input          open a recorded replay
-tournament -tournament-csv -tournament-md -ratings-json
-balance-sweep         run a sweep config and print the win-rate table
```

The `fair` preset disables engine assists (auto-wave, auto-defend, adaptive pressure) so results measure model decisions only. Any ruleset JSON can set `"disable_assists": true`, and `"skip_forced_save_turns": true` to stop asking the model on turns where saving is the only legal move — see below for what that costs.

## Development

```bash
export GOPATH=$PWD/.gomodcache GOCACHE=$PWD/.gocache GOFLAGS=-mod=mod
go build ./...
go vet ./...
go test ./...
go test -race ./...
```

Dependencies are vendored in-repo, so the build needs no network.

## Known Issues

- **Replay event streams are capped at 10,000 events and trimmed from the front.** A run at the shipped defaults (3000 ticks, 30 waves) reaches the cap exactly — measured, 858 events discarded, including four tower placements. The data is genuinely gone; what is fixed is that it is no longer *silently* gone. The stream carries a truncation marker, `ReconstructSnapshot` flags the reconstruction as untrustworthy, and the replay viewer shows a warning above the board. A reconstruction of a window entirely before the gap is still exact and is not flagged.
- **`tank` and `healer` never appear in a scripted duel.** They are gated behind wave 6 and wave 16, and the wave counter does not get there under the measurement wave cap. Both have since been measured anyway — see above — by restatting the unit the scripted attacker spawns, and for the healer's ability, by a dedicated scripted attacker, since that ability keys on the unit's type name rather than on a stat.
- **A decision is still requested every tick by default**, whether or not anything changed — see `skip_forced_save_turns` below for why that default is deliberate.
- Some Unicode glyphs may not render in every terminal.

## Asking the model less often

With income at 5/tick and the cheapest action costing 100, a player that spends is broke for about twenty ticks afterwards. The engine asks for a decision every tick regardless, so roughly nineteen of every twenty provider calls exist only to be told that nothing is affordable. On a paid API that is twenty times the tokens and twenty times the latency for no information.

The ruleset field `"skip_forced_save_turns": true` suppresses those calls: when a player's only legal action is `save`, the engine applies the save directly instead of asking. On one seed that took provider calls from 400 to 47.

**It is off by default, and the reason is worth stating.** The turns it skips are exactly the ones where a broke model proposes something it cannot afford and gets rejected — and rejection rate is one of the metrics this arena exists to record. Measured on seed 11, same match outcome either way:

| | rejected attacker actions | match result |
|---|---|---|
| default | 157 | identical |
| `skip_forced_save_turns` | 0 | identical |

The balance gate is unmoved in both modes, because a rejected action and a skipped one touch no shared game state. So the flag is free for pacing and costly for measurement, and which of those you want depends on whether you are running a demo or an experiment. Skipped turns are recorded with their own provenance source, excluded from both sides of the authored-share ratio, so they neither count as the model's work nor as an engine substitution.

## License

MIT

## Acknowledgments

- The Charm team for Bubble Tea and Lip Gloss
- The Go community
