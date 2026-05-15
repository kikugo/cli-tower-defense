package engine

import "testing"

func TestSplashUpgradeScalesMoreGentlyThanBasic(t *testing.T) {
	basic := NewTower(0, 0, "basic", nil)
	splash := NewTower(0, 0, "splash", nil)

	basic.Upgrade()
	splash.Upgrade()

	if basic.Damage <= splash.Damage {
		t.Fatalf("expected basic upgraded damage to stay above splash upgraded damage")
	}
	if splash.Damage != 12 {
		t.Fatalf("expected splash damage to scale gently to 12, got %d", splash.Damage)
	}
}

func TestBufferUpgradeIncreasesRangeOnly(t *testing.T) {
	buffer := NewTower(0, 0, "buffer", nil)
	startRange := buffer.Range

	buffer.Upgrade()

	if buffer.Range != startRange+1 {
		t.Fatalf("expected buffer range upgrade")
	}
	if buffer.Damage != 0 {
		t.Fatalf("expected buffer damage to remain 0, got %d", buffer.Damage)
	}
}
