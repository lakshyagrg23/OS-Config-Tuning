package main

import (
	"sync"
	"testing"
	"time"
)

func TestNewConflictManager(t *testing.T) {
	cm := NewConflictManager()
	if cm == nil {
		t.Error("NewConflictManager returned nil")
	}
	if cm.events == nil {
		t.Error("events map not initialized")
	}
	if len(cm.events) != 0 {
		t.Error("events map should be empty initially")
	}
}

func TestRecord_SingleEvent(t *testing.T) {
	cm := NewConflictManager()
	before := time.Now()
	cm.Record("vm.swappiness")
	after := time.Now()

	timestamps := cm.events["vm.swappiness"]
	if len(timestamps) != 1 {
		t.Errorf("expected 1 event, got %d", len(timestamps))
	}

	recorded := timestamps[0]
	if recorded.Before(before) || recorded.After(after.Add(1*time.Millisecond)) {
		t.Errorf("recorded timestamp outside expected window")
	}
}

func TestRecord_MultipleEvents(t *testing.T) {
	cm := NewConflictManager()
	param := "kernel.sched_migration_cost_ns"

	for i := 0; i < 5; i++ {
		cm.Record(param)
	}

	if len(cm.events[param]) != 5 {
		t.Errorf("expected 5 events, got %d", len(cm.events[param]))
	}
}

func TestRecord_MultipleParameters(t *testing.T) {
	cm := NewConflictManager()

	cm.Record("param1")
	cm.Record("param2")
	cm.Record("param1")
	cm.Record("param3")

	if len(cm.events["param1"]) != 2 {
		t.Errorf("param1: expected 2 events, got %d", len(cm.events["param1"]))
	}
	if len(cm.events["param2"]) != 1 {
		t.Errorf("param2: expected 1 event, got %d", len(cm.events["param2"]))
	}
	if len(cm.events["param3"]) != 1 {
		t.Errorf("param3: expected 1 event, got %d", len(cm.events["param3"]))
	}
}

func TestIsConflicting_NoEvents(t *testing.T) {
	cm := NewConflictManager()

	if cm.IsConflicting("vm.swappiness", 10*time.Second, 3) {
		t.Error("expected no conflict when no events recorded")
	}
}

func TestIsConflicting_BelowThreshold(t *testing.T) {
	cm := NewConflictManager()

	cm.Record("vm.swappiness")
	cm.Record("vm.swappiness")

	if cm.IsConflicting("vm.swappiness", 10*time.Second, 3) {
		t.Error("expected no conflict when below threshold")
	}
}

func TestIsConflicting_AtThreshold(t *testing.T) {
	cm := NewConflictManager()

	cm.Record("vm.swappiness")
	cm.Record("vm.swappiness")
	cm.Record("vm.swappiness")

	if !cm.IsConflicting("vm.swappiness", 10*time.Second, 3) {
		t.Error("expected conflict when at threshold")
	}
}

func TestIsConflicting_AboveThreshold(t *testing.T) {
	cm := NewConflictManager()

	for i := 0; i < 5; i++ {
		cm.Record("vm.swappiness")
	}

	if !cm.IsConflicting("vm.swappiness", 10*time.Second, 3) {
		t.Error("expected conflict when above threshold")
	}
}

func TestIsConflicting_OldEventsExpired(t *testing.T) {
	cm := NewConflictManager()

	// Manually inject old timestamps
	cm.mu.Lock()
	cm.events["vm.swappiness"] = []time.Time{
		time.Now().Add(-20 * time.Second),
		time.Now().Add(-15 * time.Second),
		time.Now().Add(-10 * time.Second),
	}
	cm.mu.Unlock()

	// All events are older than 5-second window, should not conflict
	if cm.IsConflicting("vm.swappiness", 5*time.Second, 3) {
		t.Error("expected no conflict when all events are outside window")
	}
}

func TestIsConflicting_MixedOldAndNew(t *testing.T) {
	cm := NewConflictManager()

	// Manually inject mixed timestamps
	cm.mu.Lock()
	cm.events["vm.swappiness"] = []time.Time{
		time.Now().Add(-20 * time.Second), // Too old
		time.Now().Add(-3 * time.Second),  // Fresh
		time.Now().Add(-2 * time.Second),  // Fresh
		time.Now().Add(-1 * time.Second),  // Fresh
	}
	cm.mu.Unlock()

	// 3 recent events within 5-second window, should conflict
	if !cm.IsConflicting("vm.swappiness", 5*time.Second, 3) {
		t.Error("expected conflict with 3 recent events")
	}

	// After call, old event should be pruned
	cm.mu.Lock()
	if len(cm.events["vm.swappiness"]) != 3 {
		t.Errorf("expected 3 events after pruning, got %d", len(cm.events["vm.swappiness"]))
	}
	cm.mu.Unlock()
}

func TestIsConflicting_WindowBoundary(t *testing.T) {
	cm := NewConflictManager()

	// Inject timestamps at exact window boundary
	cm.mu.Lock()
	now := time.Now()
	cm.events["vm.swappiness"] = []time.Time{
		now.Add(-10 * time.Second),       // At boundary
		now.Add(-10*time.Second - 1*time.Millisecond), // Just outside
	}
	cm.mu.Unlock()

	// Only 1 event within 10-second window (the one at boundary)
	if cm.IsConflicting("vm.swappiness", 10*time.Second, 2) {
		t.Error("expected no conflict with only 1 event in window")
	}
}

func TestIsConflicting_PrunsOldData(t *testing.T) {
	cm := NewConflictManager()

	// Inject 10 old events
	cm.mu.Lock()
	for i := 0; i < 10; i++ {
		cm.events["vm.swappiness"] = append(cm.events["vm.swappiness"],
			time.Now().Add(-30*time.Second))
	}
	cm.mu.Unlock()

	// Check conflict with 5-second window
	conflicting := cm.IsConflicting("vm.swappiness", 5*time.Second, 1)

	if conflicting {
		t.Error("expected no conflict with all old events")
	}

	// Verify data was pruned
	cm.mu.Lock()
	if _, exists := cm.events["vm.swappiness"]; exists {
		t.Error("expected old parameter entry to be deleted after pruning")
	}
	cm.mu.Unlock()
}

func TestLastEvent_NoEvents(t *testing.T) {
	cm := NewConflictManager()

	lastTime := cm.LastEvent("vm.swappiness")
	if !lastTime.IsZero() {
		t.Error("expected zero time for non-existent parameter")
	}
}

func TestLastEvent_WithEvents(t *testing.T) {
	cm := NewConflictManager()

	cm.Record("vm.swappiness")
	time.Sleep(10 * time.Millisecond)
	cm.Record("vm.swappiness")

	lastTime := cm.LastEvent("vm.swappiness")
	if lastTime.IsZero() {
		t.Error("expected non-zero time for recorded events")
	}
}

func TestEventCount_WithinWindow(t *testing.T) {
	cm := NewConflictManager()

	cm.Record("vm.swappiness")
	cm.Record("vm.swappiness")
	cm.Record("vm.swappiness")

	count := cm.EventCount("vm.swappiness", 10*time.Second)
	if count != 3 {
		t.Errorf("expected 3 events in window, got %d", count)
	}
}

func TestEventCount_OutsideWindow(t *testing.T) {
	cm := NewConflictManager()

	cm.mu.Lock()
	cm.events["vm.swappiness"] = []time.Time{
		time.Now().Add(-30 * time.Second),
		time.Now().Add(-20 * time.Second),
		time.Now().Add(-10 * time.Second),
	}
	cm.mu.Unlock()

	count := cm.EventCount("vm.swappiness", 5*time.Second)
	if count != 0 {
		t.Errorf("expected 0 events in window, got %d", count)
	}
}

func TestClear_SingleParameter(t *testing.T) {
	cm := NewConflictManager()

	cm.Record("param1")
	cm.Record("param1")
	cm.Record("param2")

	cm.Clear("param1")

	if len(cm.events["param1"]) != 0 {
		t.Error("expected param1 to be cleared")
	}
	if len(cm.events["param2"]) != 1 {
		t.Error("expected param2 to remain unchanged")
	}
}

func TestClear_NonExistentParameter(t *testing.T) {
	cm := NewConflictManager()

	// Should not panic
	cm.Clear("non-existent")
}

func TestClearAll(t *testing.T) {
	cm := NewConflictManager()

	cm.Record("param1")
	cm.Record("param2")
	cm.Record("param3")

	cm.ClearAll()

	if len(cm.events) != 0 {
		t.Errorf("expected 0 events after ClearAll, got %d", len(cm.events))
	}
}

func TestConflictManager_ThreadSafety_ConcurrentRecords(t *testing.T) {
	cm := NewConflictManager()
	numGoroutines := 100
	eventsPerGoroutine := 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				cm.Record("vm.swappiness")
			}
		}()
	}

	wg.Wait()

	expected := numGoroutines * eventsPerGoroutine
	if len(cm.events["vm.swappiness"]) != expected {
		t.Errorf("expected %d events, got %d", expected, len(cm.events["vm.swappiness"]))
	}
}

func TestConflictManager_ThreadSafety_MixedReadWrite(t *testing.T) {
	cm := NewConflictManager()

	// Add initial events
	for i := 0; i < 10; i++ {
		cm.Record("vm.swappiness")
	}

	var wg sync.WaitGroup
	numWriters := 5
	numReaders := 10

	// Start writers
	wg.Add(numWriters)
	for w := 0; w < numWriters; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				cm.Record("vm.swappiness")
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	// Start readers
	wg.Add(numReaders)
	for r := 0; r < numReaders; r++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				cm.IsConflicting("vm.swappiness", 10*time.Second, 5)
				cm.EventCount("vm.swappiness", 10*time.Second)
				cm.LastEvent("vm.swappiness")
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// Verify we have all recorded events
	expected := 10 + (numWriters * 20)
	if len(cm.events["vm.swappiness"]) != expected {
		t.Errorf("expected %d events, got %d", expected, len(cm.events["vm.swappiness"]))
	}
}

func TestTypicalConflictScenario(t *testing.T) {
	cm := NewConflictManager()
	param := "vm.swappiness"
	window := 5 * time.Second
	threshold := 3

	// Scenario: Remediation keeps getting overridden
	// Attempt 1: Record remediation
	cm.Record(param)
	if cm.IsConflicting(param, window, threshold) {
		t.Error("step 1: premature conflict detection")
	}

	// Attempt 2: Drift recurs 50ms later
	time.Sleep(50 * time.Millisecond)
	cm.Record(param)
	if cm.IsConflicting(param, window, threshold) {
		t.Error("step 2: premature conflict detection")
	}

	// Attempt 3: Drift recurs again
	time.Sleep(50 * time.Millisecond)
	cm.Record(param)
	if !cm.IsConflicting(param, window, threshold) {
		t.Error("step 3: conflict should be detected")
	}

	// Wait for window to expire
	time.Sleep(6 * time.Second)
	if cm.IsConflicting(param, window, threshold) {
		t.Error("step 4: conflict should have expired")
	}
}

func BenchmarkConflictManager_Record(b *testing.B) {
	cm := NewConflictManager()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cm.Record("vm.swappiness")
	}
}

func BenchmarkConflictManager_IsConflicting(b *testing.B) {
	cm := NewConflictManager()

	// Pre-populate with events
	for i := 0; i < 100; i++ {
		cm.Record("vm.swappiness")
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cm.IsConflicting("vm.swappiness", 10*time.Second, 50)
	}
}

func BenchmarkConflictManager_EventCount(b *testing.B) {
	cm := NewConflictManager()

	// Pre-populate with events
	for i := 0; i < 100; i++ {
		cm.Record("vm.swappiness")
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cm.EventCount("vm.swappiness", 10*time.Second)
	}
}

func BenchmarkConflictManager_LastEvent(b *testing.B) {
	cm := NewConflictManager()

	// Pre-populate with events
	for i := 0; i < 100; i++ {
		cm.Record("vm.swappiness")
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cm.LastEvent("vm.swappiness")
	}
}
