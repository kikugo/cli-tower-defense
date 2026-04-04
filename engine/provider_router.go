package engine

import "fmt"

type DecisionProvider interface {
	Name() string
	GetTowerDecision(gameState map[string]interface{}) (map[string]interface{}, error)
	GetEnemyDecision(gameState map[string]interface{}) (map[string]interface{}, error)
}

type DecisionRouter struct {
	providers map[string]DecisionProvider
}

func NewDecisionRouter() *DecisionRouter {
	return &DecisionRouter{providers: map[string]DecisionProvider{}}
}

func (r *DecisionRouter) SetPlayerProvider(playerID string, provider DecisionProvider) {
	r.providers[playerID] = provider
}

func (r *DecisionRouter) ProviderForPlayer(playerID string) (DecisionProvider, error) {
	provider, ok := r.providers[playerID]
	if !ok {
		return nil, fmt.Errorf("no provider configured for player %q", playerID)
	}
	return provider, nil
}

