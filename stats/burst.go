package stats

import (
	"sync"
	"time"
)

type BurstConfig struct {
	Window     time.Duration `yaml:"window"`
	Threshold  int           `yaml:"threshold"`
	MaxSamples int           `yaml:"max_samples"`
}

type BurstSnapshot struct {
	Count     int
	Window    time.Duration
	Threshold int
	Score     float64 // normalized 0..1
	Severity  Severity
}

func (c BurstConfig) withDefaults() BurstConfig {
	out := c
	if out.Window <= 0 {
		out.Window = 2 * time.Second
	}
	if out.Threshold <= 0 {
		out.Threshold = 10
	}
	if out.MaxSamples <= 0 {
		out.MaxSamples = 256
	}
	if out.MaxSamples < out.Threshold {
		out.MaxSamples = out.Threshold
	}
	return out
}

// BurstDetector tracks event density in a sliding time window.
//
// It uses a fixed-size ring buffer to keep memory bounded.
type BurstDetector struct {
	mu  sync.Mutex
	cfg BurstConfig

	buf        []time.Time
	head, size int
}

func NewBurstDetector(cfg BurstConfig) *BurstDetector {
	cfg = cfg.withDefaults()
	return &BurstDetector{
		cfg: cfg,
		buf: make([]time.Time, cfg.MaxSamples),
	}
}

func (b *BurstDetector) Add(now time.Time) BurstSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.buf) == 0 {
		return BurstSnapshot{Window: b.cfg.Window, Threshold: b.cfg.Threshold, Severity: SeverityNone}
	}

	idx := (b.head + b.size) % len(b.buf)
	if b.size == len(b.buf) {
		b.head = (b.head + 1) % len(b.buf)
		idx = (b.head + b.size - 1) % len(b.buf)
	} else {
		b.size++
	}
	b.buf[idx] = now

	cutoff := now.Add(-b.cfg.Window)
	for b.size > 0 {
		oldest := b.buf[b.head]
		if oldest.After(cutoff) {
			break
		}
		b.head = (b.head + 1) % len(b.buf)
		b.size--
	}

	count := b.size
	severity := SeverityNone
	if count >= 2*b.cfg.Threshold {
		severity = SeverityHigh
	} else if count >= b.cfg.Threshold {
		severity = SeverityMedium
	}

	score := 0.0
	if b.cfg.Threshold > 0 {
		score = float64(count) / float64(b.cfg.Threshold)
		if score > 1.0 {
			score = 1.0
		}
	}

	return BurstSnapshot{
		Count:     count,
		Window:    b.cfg.Window,
		Threshold: b.cfg.Threshold,
		Score:     score,
		Severity:  severity,
	}
}
