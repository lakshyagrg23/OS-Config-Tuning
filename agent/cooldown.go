package main

import (
	"sync"
	"time"
)

// CooldownManager tracks remediation cooldowns for sysctl parameters to prevent
// infinite remediation loops. It is thread-safe and handles concurrent access
// using read and write locks.
type CooldownManager struct {
	mu             sync.RWMutex
	lastRemediated map[string]time.Time
}

// NewCooldownManager creates a new CooldownManager with an empty cooldown map.
func NewCooldownManager() *CooldownManager {
	return &CooldownManager{
		lastRemediated: make(map[string]time.Time),
	}
}

// InCooldown returns true if the given parameter is still within its cooldown period.
// A parameter is in cooldown if the elapsed time since its last remediation is less
// than the specified cooldown duration. Returns false if the parameter has never been
// remediated or if the cooldown has expired.
//
// This method uses a read lock and is safe for concurrent access.
func (c *CooldownManager) InCooldown(param string, cooldown time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	lastTime, exists := c.lastRemediated[param]
	if !exists {
		return false
	}

	elapsed := time.Since(lastTime)
	return elapsed < cooldown
}

// Record marks the current time as the last remediation timestamp for the parameter.
// This method updates or creates an entry for the parameter with the current time.
//
// This method uses a write lock and is safe for concurrent access.
func (c *CooldownManager) Record(param string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastRemediated[param] = time.Now()
}

// LastRemediation returns the time of the last remediation for a parameter,
// or zero time if the parameter has never been remediated.
//
// This method uses a read lock and is safe for concurrent access.
func (c *CooldownManager) LastRemediation(param string) time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if lastTime, exists := c.lastRemediated[param]; exists {
		return lastTime
	}
	return time.Time{}
}

// Clear removes all cooldown records, effectively resetting the manager.
//
// This method uses a write lock and is safe for concurrent access.
func (c *CooldownManager) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastRemediated = make(map[string]time.Time)
}
