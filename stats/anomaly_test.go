package stats

import "testing"

func TestClassifyZScore(t *testing.T) {
	cfg := AnomalyConfig{MinSamples: 5, MediumZ: 2.0, HighZ: 3.0}

	res := ClassifyZScore(3.1, cfg, 10)
	if !res.IsAnomalous || res.Severity != SeverityHigh {
		t.Fatalf("expected high anomaly, got %+v", res)
	}

	res = ClassifyZScore(2.2, cfg, 10)
	if !res.IsAnomalous || res.Severity != SeverityMedium {
		t.Fatalf("expected medium anomaly, got %+v", res)
	}

	res = ClassifyZScore(2.2, cfg, 1)
	if res.IsAnomalous || res.Severity != SeverityUnknown {
		t.Fatalf("expected unknown due to low samples, got %+v", res)
	}
}
