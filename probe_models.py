#!/usr/bin/env python3
"""Which NVIDIA free-endpoint models can actually play a match TODAY.

Untracked helper, same class as check_api.go. Run it before any live run:
availability changes without notice (qwen3-next-80b-a3b-instruct was this
project's recommended defender and now returns HTTP 410 Gone), and the
/v1/models listing is not a guide -- plenty of listed models 404 on
completion or never respond.

Two things this checks that a toy prompt cannot:

  * It sends a REAL game prompt (~3.8KB), dumped from the engine. Reasoning
    models return clean JSON for a one-line prompt and an EMPTY content field
    for this one, because they spend the whole token budget reasoning. That is
    the difference between openai/gpt-oss-120b looking perfect and being
    unusable.
  * It reports completion_tokens, so a model that needs a bigger max_tokens is
    distinguishable from one that is simply broken.

What it still CANNOT tell you: sustained throughput. The free tier is much
slower under a match's back-to-back requests than for a single probe --
meta/llama-3.2-3b-instruct answers in 1.0s here and timed out on 4 of 4 calls
inside a real match. Treat these numbers as a floor.

The sample prompt is EMBEDDED below so this script always works on its own.
Point PROMPT_FILE at a freshly dumped one if the prompt builder changes enough
to matter; the embedded copy is a snapshot, not a live read.

Usage:
    set -a; source .env; set +a
    python3 probe_models.py                 # the full default sweep
    python3 probe_models.py meta/llama-3.1-8b-instruct
    MAX_TOKENS=3000 python3 probe_models.py openai/gpt-oss-120b
"""
import json, os, sys, time, urllib.request

KEY = os.environ.get("NVIDIA_API_KEY")
if not KEY:
    sys.exit("NVIDIA_API_KEY is not set (try: set -a; source .env; set +a)")

URL = "https://integrate.api.nvidia.com/v1/chat/completions"
# A real defender prompt, captured from the engine at seed 7 / tick 40. Sending
# something this size is the whole point: reasoning models answer a one-line
# prompt with clean JSON and answer THIS one with an empty content field,
# because the token budget goes to reasoning instead. A toy prompt reports them
# as working when they are not.
EMBEDDED_PROMPT = """\
You are the Defender in a Tower Defense Battleground. Goal: Stop enemies from reaching the end.
Current Resources: map[p1:320 p2:320], Base Income: map[p1:5 p2:5], Wave: 0, Paths: 2
You can currently afford ONLY these actions: save, place:basic, place:splash, place:sniper, place:buffer, place_slow_zone, research:economy, research:range, research:control, invest; valid tower cells include [3 0], [5 0], [3 1]

Current objective: maximize survival and defend lives while keeping future economy viable.
Legal action schema: exactly one JSON object with keys action, reason, taunt and action-specific fields.
Your available tools this turn:
Choose EXACTLY ONE of these currently affordable actions:
1. {"action": "place", "tower_type": "basic|splash|sniper|buffer", "position": [y, x], "reason": "...", "taunt": "..."}
   Costs: basic (100), splash (200), sniper (100), buffer (300). Position must be inside map, not on path, obstacle, or another tower.
2. {"action": "place_slow_zone", "position": [y, x], "reason": "...", "taunt": "..."}
   Cost: 150. Reduces enemy speed by 50%. MUST be on a path.
3. {"action": "research", "tech": "economy|range|control", "reason": "...", "taunt": "..."}
   Costs: economy (180), range (160), control (140). Unlocks persistent defender bonuses.
4. {"action": "invest", "reason": "...", "taunt": "..."}
   Cost: 150. Permanently increases passive income.
5. {"action": "save", "reason": "...", "taunt": "..."}

State summary:
- active_enemies: 0
- wave_queue: 0
- paths_count: 2
- resources: map[p1:320 p2:320]
- income: map[p1:5 p2:5]
- lives: map[p1:20 p2:20]
- towers_count: 0
- enemies_count: 0
- valid_tower_candidates: [[3 0] [5 0] [3 1] [5 1] [3 2] [5 2] [3 3] [5 3] [3 4] [5 4] [3 5] [5 5]]
- pressure: map[active_enemies:0 attacker_resources:320 defender_lives:20 defender_towers:0 queued_enemies:0]
- visibility: map[base_vision:6 fog_enabled:true hidden_enemies:0 visible_enemies:0 vision_range:8]
- research: map[control:0 economy:0 range:0]
- attacker_abilities: [map[cooldown:12 cost:80 current_cooldown:0 description:Increase active enemy speed by 50% name:surge] map[cooldown:14 cost:90 current_cooldown:0 description:Add temporary shield to active enemies name:shield_burst] map[cooldown:10 cost:70 current_cooldown:0 description:Add extra enemies to the queue name:reinforce_wave]]
- director: map[attacker_noop:0 pressure_level:0 pressure_triggers:0 quiet_board:true]
- last_rejected_reason: map[p1: p2:]

Tower stats (damage / range / cooldown / cost):
- basic  34 / 5 / 2 / 100. Your workhorse: cheap and fires often.
- sniper 50 / 12 / 15 / 250. Long reach and hard hitting, but fires once every 15 ticks.
- splash 10 / 3 / 3 / 200. Low damage, hits groups.
- buffer 0 / 2 / n/a / 300. Deals no damage at all.

Strategic Advice:
- A buffer tower adds +50% damage to every OTHER tower within 2 tiles, and the bonus
  stacks if several buffers overlap. It never attacks, so it is only worth its 300 cost
  when it sits beside two or more damage towers. Placing buffers next to each other
  achieves nothing, because a buffer has no damage to boost.
- Shielded enemies (S) carry shield 2, which divides ALL incoming damage by 3: a sniper
  hit drops from 50 to 16. Volume of fire beats single big hits against them.
- Healer enemies (H) restore health to nearby enemies, so kill them first when both are
  in range.
- Enemy health for reference: basic 100, fast 50, tank 300, shielded 150, healer 80.
- Invest early if you can afford to, but don't let your lives drop too low.
- Only choose an action listed as affordable above. Anything else is rejected and you
  lose the turn.
- If last_rejected_reason is non-empty, change the action AND the position, not just one.
- You may include a taunt for your opponent.

Respond with exactly one JSON object only."""

PROMPT_FILE = os.environ.get("PROMPT_FILE")
if PROMPT_FILE:
    PROMPT = open(PROMPT_FILE).read()
else:
    PROMPT = EMBEDDED_PROMPT

DEFAULT = [
    "google/diffusiongemma-26b-a4b-it", "meta/llama-3.1-8b-instruct",
    "meta/llama-3.2-11b-vision-instruct", "meta/llama-3.2-90b-vision-instruct",
    "nvidia/llama-3.1-nemotron-nano-vl-8b-v1", "nvidia/nemotron-nano-12b-v2-vl",
    "openai/gpt-oss-20b", "openai/gpt-oss-120b",
    "nvidia/nemotron-3-nano-30b-a3b", "nvidia/nemotron-3-nano-omni-30b-a3b-reasoning",
    "nvidia/nemotron-3-super-120b-a12b", "nvidia/llama-3.3-nemotron-super-49b-v1",
    "nvidia/nvidia-nemotron-nano-9b-v2", "minimaxai/minimax-m3",
    "meta/llama-3.3-70b-instruct", "meta/llama-3.2-3b-instruct",
    "meta/llama-3.2-1b-instruct", "meta/llama-3.1-70b-instruct",
    "nvidia/nemotron-3-ultra-550b-a55b", "nvidia/llama-3.3-nemotron-super-49b-v1.5",
    "nvidia/llama-3.1-nemotron-nano-8b-v1", "nvidia/nemotron-mini-4b-instruct",
    "google/gemma-4-31b-it", "mistralai/mistral-nemotron",
    "stepfun-ai/step-3.7-flash", "thinkingmachines/inkling",
    "poolside/laguna-xs-2.1", "z-ai/glm-5.2",
    "qwen/qwen3-next-80b-a3b-instruct", "deepseek-ai/deepseek-v4-pro",
    "deepseek-ai/deepseek-v4-flash", "mistralai/mistral-medium-3.5-128b",
    "bytedance/seed-oss-36b-instruct", "sarvamai/sarvam-m",
    "stepfun-ai/step-3.5-flash", "upstage/solar-10.7b-instruct",
    "mistralai/mixtral-8x7b-instruct-v0.1",
]
MAX_TOKENS = int(os.environ.get("MAX_TOKENS", "400"))


def probe(model):
    body = json.dumps({"model": model, "max_tokens": MAX_TOKENS,
                       "messages": [{"role": "user", "content": PROMPT}]}).encode()
    req = urllib.request.Request(URL, data=body, headers={
        "Authorization": "Bearer " + KEY, "Content-Type": "application/json"})
    start = time.time()
    try:
        with urllib.request.urlopen(req, timeout=90) as resp:
            data = json.load(resp)
    except Exception as exc:
        return time.time() - start, "FAIL", str(exc)[:48], None
    content = (data["choices"][0]["message"].get("content") or "").strip()
    tokens = data.get("usage", {}).get("completion_tokens")
    if not content:
        return time.time() - start, "EMPTY", "raise MAX_TOKENS and retry", tokens
    if '"action"' not in content:
        return time.time() - start, "NO-JSON", content[:44].replace("\n", " "), tokens
    return time.time() - start, "JSON-OK", content[:44].replace("\n", " "), tokens


print(f"prompt: {len(PROMPT)} bytes   max_tokens: {MAX_TOKENS}   (serial, one at a time)\n")
for model in (sys.argv[1:] or DEFAULT):
    secs, status, note, tokens = probe(model)
    tok = f"{tokens:>5}" if tokens is not None else "    -"
    print(f"{secs:6.1f}s  {status:8} tok={tok}  {model:44} {note}")
