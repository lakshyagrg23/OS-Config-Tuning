package stats

import (
	"testing"
	"time"
)

func TestReputation_DecayTowardNeutral(t *testing.T) {
	cfg := ReputationConfig{NeutralScore: 0.5, HalfLife: 10 * time.Second, MinScore: 0, MaxScore: 1, InitialScore: 1.0}
	cfg = cfg.withDefaults()

	r := Reputation{Score: 1.0, LastUpdated: time.Unix(1000, 0)}
	r.Decay(cfg, time.Unix(1010, 0)) // one half-life

	// After one half-life, distance to neutral halves: 0.5 + (1-0.5)*0.5 = 0.75
	if r.Score < 0.749 || r.Score > 0.751 {
		t.Fatalf("expected ~0.75, got %v", r.Score)
	}
}

func TestReputationManager_Update(t *testing.T) {
	m := NewReputationManager(ReputationConfig{InitialScore: 0.5, HalfLife: 0})
	now := time.Unix(1000, 0)

	snap := m.Get("proc", now)
	if snap.Score != 0.5 {
		t.Fatalf("expected initial 0.5, got %v", snap.Score)
	}

	snap2 := m.Update("proc", now.Add(1*time.Second), ReputationUpdate{WasDrift: true, FinalAction: "alert", AnomalySeverity: SeverityHigh})
	if snap2.Score >= snap.Score {
		t.Fatalf("expected score to decrease, before=%v after=%v", snap.Score, snap2.Score)
	}
}
