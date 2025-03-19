import logging
from typing import Dict, List, Tuple
import openai
from .base_ai import BaseAI
from ..game.game_state import GameState
from ..game.tower import Tower
from ..game.enemy import Enemy
from ..game.tower_type import TowerType
from ..game.enemy_type import EnemyType

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

class OpenAIAI(BaseAI):
    def __init__(self, api_key: str):
        super().__init__()
        openai.api_key = api_key
        self.model = "gpt-4-turbo-preview"
        logger.info(f"Initialized OpenAI AI with model: {self.model}")

    def _create_prompt(self, game_state: GameState) -> str:
        # Create a detailed prompt with game state information
        prompt = f"""You are playing a tower defense game. Your goal is to defend your base by placing towers strategically.

Current Game State:
- Money: {game_state.money}
- Lives: {game_state.lives}
- Wave: {game_state.current_wave}
- Time: {game_state.time}

Map Information:
- Width: {game_state.map_width}
- Height: {game_state.map_height}
- Path: {game_state.path}

Your Towers:
"""
        for tower in game_state.towers:
            prompt += f"- {tower.tower_type.value} at ({tower.x}, {tower.y}) with level {tower.level}\n"

        prompt += "\nActive Enemies:\n"
        for enemy in game_state.enemies:
            prompt += f"- {enemy.enemy_type.value} at ({enemy.x}, {enemy.y}) with {enemy.health} health\n"

        prompt += """
Based on this information, what should be your next action? Choose one of:
1. Place a new tower (specify type and position)
2. Upgrade an existing tower (specify tower position)
3. Do nothing

Respond in JSON format with:
{
    "action": "place" | "upgrade" | "nothing",
    "tower_type": "archer" | "wizard" | "cannon" | null,
    "x": number | null,
    "y": number | null
}
"""
        return prompt

    def get_next_action(self, game_state: GameState) -> Tuple[str, Dict]:
        logger.info("Getting next action from OpenAI AI")
        logger.info(f"Current game state: Money={game_state.money}, Lives={game_state.lives}, Wave={game_state.current_wave}")
        
        try:
            prompt = self._create_prompt(game_state)
            logger.debug(f"Created prompt: {prompt}")
            
            response = openai.chat.completions.create(
                model=self.model,
                messages=[
                    {"role": "system", "content": "You are a tower defense game AI. Make strategic decisions based on the game state."},
                    {"role": "user", "content": prompt}
                ],
                temperature=0.7,
                max_tokens=500
            )
            
            logger.info(f"Received response from OpenAI: {response.choices[0].message.content}")
            
            # Parse the response
            import json
            try:
                action_data = json.loads(response.choices[0].message.content)
                logger.info(f"Parsed action: {action_data}")
                
                if action_data["action"] == "place":
                    logger.info(f"AI decided to place {action_data['tower_type']} at ({action_data['x']}, {action_data['y']})")
                elif action_data["action"] == "upgrade":
                    logger.info(f"AI decided to upgrade tower at ({action_data['x']}, {action_data['y']})")
                else:
                    logger.info("AI decided to do nothing")
                
                return action_data["action"], action_data
            except json.JSONDecodeError as e:
                logger.error(f"Failed to parse OpenAI response as JSON: {e}")
                return "nothing", {}
                
        except Exception as e:
            logger.error(f"Error getting action from OpenAI: {e}")
            return "nothing", {} 