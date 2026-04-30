package main

import (
	"sync"
	"time"
)

// ConflictManager detects repeated drift events for the same parameter.
// It tracks when remediation attempts occur and identifies conflicts
// (e.g., another system overriding changes) by detecting rapid repeats.
type ConflictManager struct {
	mu     sync.Mutex
	events map[string][]time.Time
}

// NewConflictManager creates a new ConflictManager instance.
func NewConflictManager() *ConflictManager {
	return &ConflictManager{
		events: make(map[string][]time.Time),
	}
}

// Record appends a new event timestamp for the given parameter.
// Thread-safe.
func (c *ConflictManager) Record(param string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events[param] = append(c.events[param], time.Now())
}

// IsConflicting checks if a parameter has experienced repeated drift
// within a time window, indicating a conflict.
//
// Parameters:
//   - param: The sysctl parameter to check
//   - window: Time window to consider for conflict detection
//   - threshold: Number of events required to declare a conflict
//
// Returns true if the parameter has >= threshold events within the window.
// Automatically prunes old timestamps from tracking.
// Thread-safe.
func (c *ConflictManager) IsConflicting(param string, window time.Duration, threshold int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	timestamps, exists := c.events[param]
	if !exists {
		return false
	}

	// Calculate cutoff time: timestamps older than this are outside the window
	cutoff := time.Now().Add(-window)

	// Prune old timestamps and count valid ones
	var validTimestamps []time.Time
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			validTimestamps = append(validTimestamps, ts)
		}
	}

	// Update map with pruned timestamps (cleanup old data)
	if len(validTimestamps) > 0 {
		c.events[param] = validTimestamps
	} else {
		delete(c.events, param)
	}

	// Return true if conflict threshold is met
	return len(validTimestamps) >= threshold
}

// LastEvent returns the timestamp of the most recent event for a parameter,
// or zero time if no events exist. Thread-safe.
func (c *ConflictManager) LastEvent(param string) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	timestamps, exists := c.events[param]
	if !exists || len(timestamps) == 0 {
		return time.Time{}
	}

	return timestamps[len(timestamps)-1]
}

// EventCount returns the number of events for a parameter within the given window.
// Thread-safe.
func (c *ConflictManager) EventCount(param string, window time.Duration) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	timestamps, exists := c.events[param]
	if !exists {
		return 0
	}

	cutoff := time.Now().Add(-window)
	count := 0
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			count++
		}
	}

	return count
}

// Clear removes all tracking data for a parameter. Thread-safe.
func (c *ConflictManager) Clear(param string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.events, param)
}

// ClearAll removes all tracking data. Useful for resetting state. Thread-safe.
func (c *ConflictManager) ClearAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = make(map[string][]time.Time)
}
