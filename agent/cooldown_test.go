package main

import (
	"sync"
	"testing"
	"time"
)

func TestNewCooldownManager(t *testing.T) {
	cm := NewCooldownManager()

	if cm == nil {
		t.Fatal("NewCooldownManager returned nil")
	}

	if cm.lastRemediated == nil {
		t.Fatal("lastRemediated map not initialized")
	}

	if len(cm.lastRemediated) != 0 {
		t.Errorf("expected empty map, got %d entries", len(cm.lastRemediated))
	}
}

func TestInCooldown_NotRecorded(t *testing.T) {
	cm := NewCooldownManager()

	// Parameter not recorded yet should never be in cooldown
	inCooldown := cm.InCooldown("vm.swappiness", 30*time.Second)

	if inCooldown {
		t.Errorf("expected InCooldown to return false for unrecorded parameter, got true")
	}
}

func TestRecord_SingleParameter(t *testing.T) {
	cm := NewCooldownManager()

	cm.Record("vm.swappiness")

	// Should now show it was recorded
	lastTime := cm.LastRemediation("vm.swappiness")
	if lastTime.IsZero() {
		t.Error("expected LastRemediation to return non-zero time after Record")
	}
}

func TestInCooldown_WithinCooldown(t *testing.T) {
	cm := NewCooldownManager()

	cm.Record("vm.swappiness")

	// Immediately after recording, should be in cooldown (elapsed ≈ 0)
	inCooldown := cm.InCooldown("vm.swappiness", 30*time.Second)

	if !inCooldown {
		t.Errorf("expected InCooldown to return true within cooldown period, got false")
	}
}

func TestInCooldown_ExpiredCooldown(t *testing.T) {
	cm := NewCooldownManager()

	// Record time in the past by directly manipulating the map (for testing)
	cm.mu.Lock()
	cm.lastRemediated["test.param"] = time.Now().Add(-2 * time.Second)
	cm.mu.Unlock()

	// With a 1 second cooldown, the 2 second old entry should be expired
	inCooldown := cm.InCooldown("test.param", 1*time.Second)

	if inCooldown {
		t.Errorf("expected InCooldown to return false when cooldown expired, got true")
	}
}

func TestMultipleParameters(t *testing.T) {
	cm := NewCooldownManager()

	cm.Record("param1")
	cm.Record("param2")
	cm.Record("param3")

	// All should be in cooldown
	if !cm.InCooldown("param1", 30*time.Second) || !cm.InCooldown("param2", 30*time.Second) || !cm.InCooldown("param3", 30*time.Second) {
		t.Error("expected all parameters to be in cooldown")
	}

	// Expire param2 artificially
	cm.mu.Lock()
	cm.lastRemediated["param2"] = time.Now().Add(-2 * time.Second)
	cm.mu.Unlock()

	// param1 and param3 still in cooldown, param2 expired
	if !cm.InCooldown("param1", 1*time.Second) {
		t.Error("param1 should still be in cooldown")
	}
	if cm.InCooldown("param2", 1*time.Second) {
		t.Error("param2 should be expired from cooldown")
	}
	if !cm.InCooldown("param3", 1*time.Second) {
		t.Error("param3 should still be in cooldown")
	}
}

func TestLastRemediation_NotRecorded(t *testing.T) {
	cm := NewCooldownManager()

	lastTime := cm.LastRemediation("vm.swappiness")

	if !lastTime.IsZero() {
		t.Errorf("expected zero time for unrecorded parameter, got %v", lastTime)
	}
}

func TestLastRemediation_Recorded(t *testing.T) {
	cm := NewCooldownManager()

	before := time.Now()
	cm.Record("vm.swappiness")
	after := time.Now()

	lastTime := cm.LastRemediation("vm.swappiness")

	if lastTime.Before(before) || lastTime.After(after.Add(1*time.Millisecond)) {
		t.Errorf("expected LastRemediation to return recent time, got %v", lastTime)
	}
}

func TestClear(t *testing.T) {
	cm := NewCooldownManager()

	cm.Record("param1")
	cm.Record("param2")
	cm.Record("param3")

	// Should have 3 entries
	if len(cm.lastRemediated) != 3 {
		t.Errorf("expected 3 entries, got %d", len(cm.lastRemediated))
	}

	cm.Clear()

	// Should be empty
	if len(cm.lastRemediated) != 0 {
		t.Errorf("expected empty map after Clear, got %d entries", len(cm.lastRemediated))
	}

	// No parameters should be in cooldown
	if cm.InCooldown("param1", 30*time.Second) {
		t.Error("expected param1 to not be in cooldown after Clear")
	}
}

func TestThreadSafety_ConcurrentRecords(t *testing.T) {
	cm := NewCooldownManager()

	var wg sync.WaitGroup
	numGoroutines := 100
	numIterations := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				cm.Record("param1")
				cm.Record("param2")
				cm.Record("param3")
			}
		}(i)
	}

	wg.Wait()

	// After all goroutines finish, should have exactly 3 entries (one per parameter)
	if len(cm.lastRemediated) != 3 {
		t.Errorf("expected 3 entries after concurrent Records, got %d", len(cm.lastRemediated))
	}

	// All should still be in cooldown
	if !cm.InCooldown("param1", 30*time.Second) || !cm.InCooldown("param2", 30*time.Second) || !cm.InCooldown("param3", 30*time.Second) {
		t.Error("expected all parameters to be in cooldown after concurrent work")
	}
}

func TestThreadSafety_MixedReadWrite(t *testing.T) {
	cm := NewCooldownManager()

	cm.Record("param1")
	cm.Record("param2")

	var wg sync.WaitGroup
	numReaders := 50
	numWriters := 10

	// Concurrent readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = cm.InCooldown("param1", 30*time.Second)
			_ = cm.InCooldown("param2", 30*time.Second)
			_ = cm.LastRemediation("param1")
		}(i)
	}

	// Concurrent writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cm.Record("param1")
			cm.Record("param2")
		}(i)
	}

	wg.Wait()

	// Should still have exactly 2 entries
	if len(cm.lastRemediated) != 2 {
		t.Errorf("expected 2 entries after concurrent mixed operations, got %d", len(cm.lastRemediated))
	}
}

func TestTypicalUsage(t *testing.T) {
	// Simulate typical agent usage pattern
	cm := NewCooldownManager()
	cooldown := 100 * time.Millisecond

	param := "vm.swappiness"

	// First remediation should be allowed
	if cm.InCooldown(param, cooldown) {
		t.Error("first remediation should not be blocked")
	}
	cm.Record(param)

	// Immediate retry should be blocked
	if !cm.InCooldown(param, cooldown) {
		t.Error("second remediation should be blocked during cooldown")
	}

	// Wait for cooldown to expire
	time.Sleep(cooldown + 10*time.Millisecond)

	// Next remediation should be allowed
	if cm.InCooldown(param, cooldown) {
		t.Error("remediation should be allowed after cooldown expired")
	}
	cm.Record(param)

	// New spam attempt should be blocked
	if !cm.InCooldown(param, cooldown) {
		t.Error("remediation spam should be blocked")
	}
}

func BenchmarkInCooldown(b *testing.B) {
	cm := NewCooldownManager()
	cm.Record("param")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cm.InCooldown("param", 30*time.Second)
	}
}

func BenchmarkRecord(b *testing.B) {
	cm := NewCooldownManager()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cm.Record("param")
	}
}
