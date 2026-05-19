package engine

import "testing"

func TestBuildRunManifestIncludesCoreFields(t *testing.T) {
	g := NewGame("k1", "k2")
	r := DefaultArenaRuleset()
	m := BuildRunManifest("headless", g, 42, true, 1200, r, "abc123")
	if m.RunType != "headless" || m.Seed != 42 || !m.Swapped || m.MaxTicks != 1200 {
		t.Fatalf("unexpected manifest metadata: %#v", m)
	}
	if m.Models[g.Player1] == "" || m.Models[g.Player2] == "" {
		t.Fatalf("expected model names in manifest")
	}
	if m.GitCommit != "abc123" {
		t.Fatalf("expected git commit in manifest")
	}
}
