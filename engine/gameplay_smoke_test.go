package engine

import "testing"

type scriptedProvider struct {
	defenderAction map[string]interface{}
	attackerAction map[string]interface{}
}

func (s *scriptedProvider) Name() string {
	return "scripted"
}

func (s *scriptedProvider) GetTowerDecision(map[string]interface{}) (map[string]interface{}, error) {
	return s.defenderAction, nil
}

func (s *scriptedProvider) GetEnemyDecision(map[string]interface{}) (map[string]interface{}, error) {
	return s.attackerAction, nil
}

func TestGameplaySmokeRunsWithoutPanic(t *testing.T) {
	g := NewGame("test", "test")
	g.SetRandomSeed(123)
	g.PauseBetweenTurns = false
	g.AIDecisionInterval[g.Player1] = 0
	g.AIDecisionInterval[g.Player2] = 0

	def := &scriptedProvider{
		defenderAction: map[string]interface{}{
			"action":     "place",
			"tower_type": "basic",
			"position":   []interface{}{float64(2), float64(2)},
		},
		attackerAction: map[string]interface{}{"action": "save"},
	}
	att := &scriptedProvider{
		defenderAction: map[string]interface{}{"action": "save"},
		attackerAction: map[string]interface{}{"action": "spawn", "enemy_type": "basic"},
	}
	g.DecisionRouter.SetPlayerProvider(g.Player1, def)
	g.DecisionRouter.SetPlayerProvider(g.Player2, att)

	for i := 0; i < 400; i++ {
		g.UpdateGameState()
		g.HandleAIDecisions()
		if g.GameOver {
			break
		}
	}

	if len(g.Logs) == 0 {
		t.Fatalf("expected gameplay logs to be generated")
	}
	if len(g.Enemies) == 0 && len(g.WaveQueue) == 0 && g.Wave == 0 {
		t.Fatalf("expected smoke run to exercise gameplay progression")
	}
}
