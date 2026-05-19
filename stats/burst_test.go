package stats

import (
	"testing"
	"time"
)

func TestBurstDetector(t *testing.T) {
	b := NewBurstDetector(BurstConfig{Window: 1 * time.Second, Threshold: 3, MaxSamples: 10})
	start := time.Unix(1000, 0)

	if snap := b.Add(start); snap.Count != 1 || snap.Severity != SeverityNone {
		t.Fatalf("unexpected snap1: %+v", snap)
	}
	_ = b.Add(start.Add(100 * time.Millisecond))
	snap := b.Add(start.Add(200 * time.Millisecond))
	if snap.Count != 3 || snap.Severity != SeverityMedium {
		t.Fatalf("expected medium burst at threshold, got %+v", snap)
	}

	// After window passes, old events should be pruned.
	snap = b.Add(start.Add(2 * time.Second))
	if snap.Count != 1 {
		t.Fatalf("expected pruning, got %+v", snap)
	}
}
