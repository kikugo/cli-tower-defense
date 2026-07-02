package engine

import (
	"encoding/json"
	"testing"
)

func TestMapInitRecordedOnceAtFirstTick(t *testing.T) {
	g := NewGame("test", "test")
	g.SetMapType("straight")
	g.UpdateGameState()
	g.UpdateGameState()

	inits := 0
	for _, ev := range g.ReplayEvents {
		if ev.Type == ReplayMapInit {
			inits++
		}
	}
	if inits != 1 {
		t.Fatalf("expected exactly one map_init, got %d", inits)
	}
	if g.ReplayEvents[0].Type != ReplayMapInit {
		t.Fatalf("expected map_init first, got %s", g.ReplayEvents[0].Type)
	}
}

func TestReplayTrimPreservesMapInit(t *testing.T) {
	g := NewGame("test", "test")
	g.SetMapType("straight")
	g.MaxReplayEvents = 5
	for i := 0; i < 20; i++ {
		g.UpdateGameState()
	}
	if len(g.ReplayEvents) != 5 {
		t.Fatalf("expected trim to 5 events, got %d", len(g.ReplayEvents))
	}
	if g.ReplayEvents[0].Type != ReplayMapInit {
		t.Fatalf("expected map_init preserved at index 0, got %s", g.ReplayEvents[0].Type)
	}
}

func TestSnapshotSurfacesMapAfterJSONRoundTrip(t *testing.T) {
	g := NewGame("test", "test")
	g.SetMapType("straight")
	g.UpdateGameState()

	raw, err := json.Marshal(g.ReplayEvents)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var loaded []ReplayEvent
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	snap := ReconstructSnapshot(loaded, len(loaded))
	if !snap.HasMap {
		t.Fatalf("expected snapshot to carry map data")
	}
	if snap.MapWidth != g.MapWidth || snap.MapHeight != g.MapHeight {
		t.Fatalf("map dims mismatch: %dx%d vs %dx%d", snap.MapWidth, snap.MapHeight, g.MapWidth, g.MapHeight)
	}
	if len(snap.MapPaths) == 0 || len(snap.MapPaths[0]) == 0 {
		t.Fatalf("expected path points in snapshot")
	}
}

func TestSnapshotCollectsBreachPoints(t *testing.T) {
	events := []ReplayEvent{
		{Type: ReplayBreach, Position: &Position{Y: 3, X: 7}, Details: map[string]interface{}{"defender_lives": 5}},
	}
	snap := ReconstructSnapshot(events, 1)
	if len(snap.BreachPoints) != 1 || snap.BreachPoints[0] != (Position{Y: 3, X: 7}) {
		t.Fatalf("expected breach point recorded, got %v", snap.BreachPoints)
	}
}
