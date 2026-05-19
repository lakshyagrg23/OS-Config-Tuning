package stats

import "testing"

func TestEWMAStat_Update(t *testing.T) {
	e := NewEWMAStat(0.5)

	if got := e.Update(10); got != 10 {
		t.Fatalf("expected first update to set value, got %v", got)
	}
	if got := e.Update(20); got != 15 {
		t.Fatalf("expected ewma=15, got %v", got)
	}
}
