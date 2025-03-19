import logging
from typing import List, Tuple
from .tower import Tower
from .enemy import Enemy
from .tower_type import TowerType
from .enemy_type import EnemyType

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

class GameState:
    def __init__(self, map_width: int, map_height: int, path: List[Tuple[int, int]]):
        self.map_width = map_width
        self.map_height = map_height
        self.path = path
        self.money = 100
        self.lives = 10
        self.current_wave = 1
        self.time = 0
        self.towers: List[Tower] = []
        self.enemies: List[Enemy] = []
        logger.info(f"Initialized game state with map size {map_width}x{map_height}")

    def place_tower(self, tower_type: TowerType, x: int, y: int) -> bool:
        if self.money < tower_type.cost:
            logger.warning(f"Not enough money to place {tower_type.value} tower. Required: {tower_type.cost}, Available: {self.money}")
            return False
            
        if not self._is_valid_position(x, y):
            logger.warning(f"Invalid position ({x}, {y}) for tower placement")
            return False
            
        tower = Tower(tower_type, x, y)
        self.towers.append(tower)
        self.money -= tower_type.cost
        logger.info(f"Placed {tower_type.value} tower at ({x}, {y}). Remaining money: {self.money}")
        return True

    def upgrade_tower(self, x: int, y: int) -> bool:
        tower = self._get_tower_at(x, y)
        if not tower:
            logger.warning(f"No tower found at position ({x}, {y})")
            return False
            
        if self.money < tower.upgrade_cost:
            logger.warning(f"Not enough money to upgrade tower. Required: {tower.upgrade_cost}, Available: {self.money}")
            return False
            
        tower.upgrade()
        self.money -= tower.upgrade_cost
        logger.info(f"Upgraded tower at ({x}, {y}) to level {tower.level}. Remaining money: {self.money}")
        return True

    def spawn_enemy(self, enemy_type: EnemyType) -> bool:
        if self.money < enemy_type.cost:
            logger.warning(f"Not enough money to spawn {enemy_type.value}. Required: {enemy_type.cost}, Available: {self.money}")
            return False
            
        enemy = Enemy(enemy_type, self.path[0])
        self.enemies.append(enemy)
        self.money -= enemy_type.cost
        logger.info(f"Spawned {enemy_type.value} enemy. Remaining money: {self.money}")
        return True

    def update(self, delta_time: float):
        self.time += delta_time
        
        # Update enemies
        for enemy in self.enemies[:]:
            enemy.move(delta_time)
            if enemy.reached_end:
                self.lives -= 1
                self.enemies.remove(enemy)
                logger.warning(f"Enemy reached the end! Lives remaining: {self.lives}")
                if self.lives <= 0:
                    logger.error("Game Over!")
                    return False
                    
        # Update towers
        for tower in self.towers:
            tower.update(delta_time)
            # Find and attack enemies in range
            for enemy in self.enemies:
                if tower.can_attack(enemy):
                    damage = tower.attack(enemy)
                    logger.debug(f"Tower at ({tower.x}, {tower.y}) dealt {damage} damage to enemy at ({enemy.x}, {enemy.y})")
                    if enemy.health <= 0:
                        self.enemies.remove(enemy)
                        self.money += enemy.enemy_type.reward
                        logger.info(f"Enemy defeated! Gained {enemy.enemy_type.reward} money. Total: {self.money}")
                        
        return True

    def _is_valid_position(self, x: int, y: int) -> bool:
        if x < 0 or x >= self.map_width or y < 0 or y >= self.map_height:
            return False
        if (x, y) in self.path:
            return False
        for tower in self.towers:
            if tower.x == x and tower.y == y:
                return False
        return True

    def _get_tower_at(self, x: int, y: int) -> Tower:
        for tower in self.towers:
            if tower.x == x and tower.y == y:
                return tower
        return None 