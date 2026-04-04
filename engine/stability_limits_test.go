package engine

import "testing"

func TestLogfRespectsMaxLogs(t *testing.T) {
	g := NewGame("test", "test")
	g.MaxLogs = 3

	g.logf("a")
	g.logf("b")
	g.logf("c")
	g.logf("d")

	if len(g.Logs) != 3 {
		t.Fatalf("expected 3 logs after cap, got %d", len(g.Logs))
	}
	if g.Logs[0] != "b" || g.Logs[2] != "d" {
		t.Fatalf("expected oldest logs trimmed, got %#v", g.Logs)
	}
}

