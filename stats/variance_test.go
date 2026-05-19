package stats

import "testing"

func TestRunningStat_Snapshot(t *testing.T) {
	var r RunningStat
	r.Update(1)
	r.Update(2)
	r.Update(3)

	s := r.Snapshot()
	if s.Count != 3 {
		t.Fatalf("expected count=3, got %d", s.Count)
	}
	if s.Mean != 2 {
		t.Fatalf("expected mean=2, got %v", s.Mean)
	}
	// Sample variance for [1,2,3] is 1.
	if s.Variance != 1 {
		t.Fatalf("expected variance=1, got %v", s.Variance)
	}
}
