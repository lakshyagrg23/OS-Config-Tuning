package stats

import "sync"

const defaultEWMAAlpha = 0.3

// EWMAStat tracks an exponentially weighted moving average.
//
// EWMA_t = alpha*current + (1-alpha)*previous
//
// Thread-safe; intended for online/streaming updates.
type EWMAStat struct {
	mu          sync.Mutex
	Value       float64
	Alpha       float64
	initialized bool
}

func NewEWMAStat(alpha float64) *EWMAStat {
	return &EWMAStat{Alpha: clampAlpha(alpha)}
}

// Update incorporates a new sample and returns the updated EWMA value.
func (e *EWMAStat) Update(value float64) float64 {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.Alpha == 0 {
		e.Alpha = defaultEWMAAlpha
	}

	if !e.initialized {
		e.Value = value
		e.initialized = true
		return e.Value
	}

	e.Value = e.Alpha*value + (1.0-e.Alpha)*e.Value
	return e.Value
}

func (e *EWMAStat) Get() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.Value
}

func (e *EWMAStat) SetAlpha(alpha float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Alpha = clampAlpha(alpha)
}

func clampAlpha(alpha float64) float64 {
	if alpha <= 0 {
		return defaultEWMAAlpha
	}
	if alpha > 1 {
		return 1
	}
	return alpha
}
