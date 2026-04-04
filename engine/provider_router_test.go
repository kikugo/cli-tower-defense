package engine

import "testing"

type fakeProvider struct {
	name string
}

func (f *fakeProvider) Name() string {
	return f.name
}

func (f *fakeProvider) GetTowerDecision(map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"action": "save"}, nil
}

func (f *fakeProvider) GetEnemyDecision(map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"action": "save"}, nil
}

func TestDecisionRouterReturnsConfiguredProvider(t *testing.T) {
	router := NewDecisionRouter()
	router.SetPlayerProvider("p1", &fakeProvider{name: "provider-a"})

	provider, err := router.ProviderForPlayer("p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.Name() != "provider-a" {
		t.Fatalf("expected provider-a, got %s", provider.Name())
	}
}

func TestDecisionRouterErrorsOnMissingProvider(t *testing.T) {
	router := NewDecisionRouter()

	if _, err := router.ProviderForPlayer("missing"); err == nil {
		t.Fatalf("expected missing provider error")
	}
}

