package stats

import (
	"math"
	"sync"
)

// RunningStat computes mean/variance/stddev online using Welford's algorithm.
// Thread-safe; intended for streaming updates.
type RunningStat struct {
	mu    sync.Mutex
	count int64
	mean  float64
	m2    float64
}

type RunningSnapshot struct {
	Count    int64
	Mean     float64
	Variance float64
	StdDev   float64
}

func (r *RunningStat) Update(value float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.count++
	delta := value - r.mean
	r.mean += delta / float64(r.count)
	delta2 := value - r.mean
	r.m2 += delta * delta2
}

func (r *RunningStat) Snapshot() RunningSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	variance := 0.0
	if r.count >= 2 {
		variance = r.m2 / float64(r.count-1)
	}

	return RunningSnapshot{
		Count:    r.count,
		Mean:     r.mean,
		Variance: variance,
		StdDev:   math.Sqrt(variance),
	}
}
