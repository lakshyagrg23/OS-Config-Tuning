package stats

import "math"

type Severity string

const (
	SeverityUnknown Severity = "unknown"
	SeverityNone    Severity = "none"
	SeverityLow     Severity = "low"
	SeverityMedium  Severity = "medium"
	SeverityHigh    Severity = "high"
)

type AnomalyConfig struct {
	MinSamples int64   `yaml:"min_samples"`
	MediumZ    float64 `yaml:"medium_z"`
	HighZ      float64 `yaml:"high_z"`
}

type AnomalyResult struct {
	ZScore      float64
	IsAnomalous bool
	Severity    Severity
	Score       float64 // normalized 0..1
}

func (c AnomalyConfig) withDefaults() AnomalyConfig {
	out := c
	if out.MinSamples <= 0 {
		out.MinSamples = 10
	}
	if out.MediumZ <= 0 {
		out.MediumZ = 2.0
	}
	if out.HighZ <= 0 {
		out.HighZ = 3.0
	}
	if out.HighZ < out.MediumZ {
		out.HighZ = out.MediumZ
	}
	return out
}

func ComputeZScore(value, mean, stddev float64) float64 {
	if stddev <= 0 {
		return 0
	}
	return (value - mean) / stddev
}

func ClassifyZScore(z float64, cfg AnomalyConfig, sampleCount int64) AnomalyResult {
	cfg = cfg.withDefaults()
	if sampleCount < cfg.MinSamples {
		return AnomalyResult{ZScore: z, IsAnomalous: false, Severity: SeverityUnknown, Score: 0}
	}

	absZ := math.Abs(z)
	switch {
	case absZ >= cfg.HighZ:
		return AnomalyResult{ZScore: z, IsAnomalous: true, Severity: SeverityHigh, Score: 1.0}
	case absZ >= cfg.MediumZ:
		return AnomalyResult{ZScore: z, IsAnomalous: true, Severity: SeverityMedium, Score: 0.7}
	case absZ > 0:
		return AnomalyResult{ZScore: z, IsAnomalous: false, Severity: SeverityLow, Score: 0.3}
	default:
		return AnomalyResult{ZScore: z, IsAnomalous: false, Severity: SeverityNone, Score: 0}
	}
}

func maxSeverity(a, b Severity) Severity {
	order := func(s Severity) int {
		switch s {
		case SeverityHigh:
			return 4
		case SeverityMedium:
			return 3
		case SeverityLow:
			return 2
		case SeverityNone:
			return 1
		default:
			return 0
		}
	}

	if order(a) >= order(b) {
		return a
	}
	return b
}
