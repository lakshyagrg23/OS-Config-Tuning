package main

import (
	"testing"
	"time"
)

func TestSimulateEvent_CriticalSecurityViolation(t *testing.T) {
	// Scenario: Untrusted process modifying high-critical security parameter
	// Expected: Immediate remediation (hard rule)

	event := WorkEvent{
		Pid:      9999,
		Process:  "malicious-app",
		Access:   "WRITE",
		FilePath: "/proc/sys/kernel/randomize_va_space",
	}

	policyEntry := SysctlPolicy{
		Expected:    "2",
		Category:    "security",
		Criticality: "high",
		Remediation: "auto",
		Cooldown:    30 * time.Second,
	}

	policy := &Policy{
		Global:           GlobalConfig{RemediateThreshold: 8, AlertThreshold: 4},
		TrustedProcesses: []string{"systemd", "kubelet"},
		Sysctl:           map[string]SysctlPolicy{},
	}

	cm := NewCooldownManager()

	decision := simulateEvent(
		event,
		"kernel.randomize_va_space",
		policyEntry,
		policy,
		cm,
		"0", // actual value differs from expected
	)

	if decision.Action != "remediate" {
		t.Errorf("expected 'remediate' for critical security violation, got '%s'", decision.Action)
	}
	if decision.Score != 10 {
		t.Errorf("expected score 10 (hard rule), got %d", decision.Score)
	}

	// Should be recorded in cooldown
	if !cm.InCooldown("kernel.randomize_va_space", 30*time.Second) {
		t.Error("expected parameter to be in cooldown after remediation")
	}
}

func TestSimulateEvent_AllowedProcessOverride(t *testing.T) {
	// Scenario: kube-proxy modifying net.ipv4.ip_forward (explicitly allowed)
	// Expected: Allow (exception granted)

	event := WorkEvent{
		Pid:      5000,
		Process:  "kube-proxy",
		Access:   "WRITE",
		FilePath: "/proc/sys/net/ipv4/ip_forward",
	}

	policyEntry := SysctlPolicy{
		Expected:       "0",
		Category:       "security",
		Criticality:    "high",
		Remediation:    "auto",
		Cooldown:       30 * time.Second,
		AllowProcesses: []string{"kube-proxy"},
	}

	policy := &Policy{
		Global:           GlobalConfig{RemediateThreshold: 8, AlertThreshold: 4},
		TrustedProcesses: []string{},
		Sysctl:           map[string]SysctlPolicy{},
	}

	cm := NewCooldownManager()

	decision := simulateEvent(
		event,
		"net.ipv4.ip_forward",
		policyEntry,
		policy,
		cm,
		"1", // differs from expected "0"
	)

	if decision.Action != "allow" {
		t.Errorf("expected 'allow' for whitelisted process, got '%s'", decision.Action)
	}
	if decision.Score != 0 {
		t.Errorf("expected score 0, got %d", decision.Score)
	}

	// Should NOT be recorded in cooldown (was allowed, not remediated)
	if cm.InCooldown("net.ipv4.ip_forward", 30*time.Second) {
		t.Error("expected parameter to NOT be in cooldown when allowed")
	}
}

func TestSimulateEvent_CooldownBlocking(t *testing.T) {
	// Scenario: Parameter flagged for remediation but in active cooldown
	// Expected: Decision downgraded to "alert"

	event := WorkEvent{
		Pid:      1000,
		Process:  "unknown-daemon",
		Access:   "WRITE",
		FilePath: "/proc/sys/kernel/dmesg_restrict",
	}

	policyEntry := SysctlPolicy{
		Expected:    "1",
		Category:    "security",
		Criticality: "high",
		Remediation: "auto",
		Cooldown:    30 * time.Second,
	}

	policy := &Policy{
		Global:           GlobalConfig{RemediateThreshold: 8, AlertThreshold: 4},
		TrustedProcesses: []string{},
		Sysctl:           map[string]SysctlPolicy{},
	}

	cm := NewCooldownManager()

	// Pre-populate cooldown (simulate previous remediation)
	cm.Record("kernel.dmesg_restrict")

	decision := simulateEvent(
		event,
		"kernel.dmesg_restrict",
		policyEntry,
		policy,
		cm,
		"0", // differs from expected
	)

	// Decision engine would say "remediate" (score 10, hard rule)
	// But cooldown manager blocks it → downgrade to "alert"
	if decision.Action != "alert" {
		t.Errorf("expected 'alert' when cooldown blocks remediation, got '%s'", decision.Action)
	}

	// Should find cooldown reason in the decision
	foundCooldownReason := false
	for _, reason := range decision.Reasons {
		if reason == "remediation blocked by cooldown" {
			foundCooldownReason = true
			break
		}
	}
	if !foundCooldownReason {
		t.Errorf("expected cooldown reason in decision, got: %v", decision.Reasons)
	}
}

func TestSimulateEvent_CooldownExpiry(t *testing.T) {
	// Scenario: Cooldown has expired, remediation should be allowed again
	// Expected: Remediate

	event := WorkEvent{
		Pid:      2000,
		Process:  "rogue-app",
		Access:   "WRITE",
		FilePath: "/proc/sys/fs/suid_dumpable",
	}

	policyEntry := SysctlPolicy{
		Expected:    "0",
		Category:    "security",
		Criticality: "high",
		Remediation: "auto",
		Cooldown:    50 * time.Millisecond, // Short cooldown for testing
	}

	policy := &Policy{
		Global:           GlobalConfig{RemediateThreshold: 8, AlertThreshold: 4},
		TrustedProcesses: []string{},
		Sysctl:           map[string]SysctlPolicy{},
	}

	cm := NewCooldownManager()

	// Record an old timestamp (expired cooldown)
	cm.mu.Lock()
	cm.lastRemediated["fs.suid_dumpable"] = time.Now().Add(-100 * time.Millisecond)
	cm.mu.Unlock()

	decision := simulateEvent(
		event,
		"fs.suid_dumpable",
		policyEntry,
		policy,
		cm,
		"1", // differs from expected
	)

	// Cooldown expired, so remediation should proceed
	if decision.Action != "remediate" {
		t.Errorf("expected 'remediate' after cooldown expiry, got '%s'", decision.Action)
	}
}

func TestSimulateEvent_RepeatedEvents_CooldownEnforced(t *testing.T) {
	// Scenario: Same parameter mutated multiple times rapidly
	// Expected: First remediation, subsequent attempts blocked by cooldown

	event := WorkEvent{
		Pid:      3000,
		Process:  "spammy-process",
		Access:   "WRITE",
		FilePath: "/proc/sys/net/ipv4/tcp_syncookies",
	}

	policyEntry := SysctlPolicy{
		Expected:    "1",
		Category:    "security",
		Criticality: "high",
		Remediation: "auto",
		Cooldown:    100 * time.Millisecond, // Short for testing
	}

	policy := &Policy{
		Global:           GlobalConfig{RemediateThreshold: 8, AlertThreshold: 4},
		TrustedProcesses: []string{},
		Sysctl:           map[string]SysctlPolicy{},
	}

	cm := NewCooldownManager()
	param := "net.ipv4.tcp_syncookies"

	// Event 1: Should remediate (not in cooldown yet)
	decision1 := simulateEvent(event, param, policyEntry, policy, cm, "0")
	if decision1.Action != "remediate" {
		t.Errorf("first attempt: expected 'remediate', got '%s'", decision1.Action)
	}

	// Event 2: Immediately after (should be blocked by cooldown)
	decision2 := simulateEvent(event, param, policyEntry, policy, cm, "0")
	if decision2.Action != "alert" {
		t.Errorf("second attempt during cooldown: expected 'alert', got '%s'", decision2.Action)
	}

	// Event 3: After cooldown expires
	time.Sleep(150 * time.Millisecond)
	decision3 := simulateEvent(event, param, policyEntry, policy, cm, "0")
	if decision3.Action != "remediate" {
		t.Errorf("third attempt after cooldown: expected 'remediate', got '%s'", decision3.Action)
	}
}

func TestSimulateEvent_TrustedProcessLowRisk(t *testing.T) {
	// Scenario: Trusted process modifying low-risk parameter
	// Expected: Allow (risk score below threshold)

	event := WorkEvent{
		Pid:      100,
		Process:  "systemd",
		Access:   "WRITE",
		FilePath: "/proc/sys/vm/swappiness",
	}

	policyEntry := SysctlPolicy{
		Expected:    "10",
		Category:    "performance",
		Criticality: "low",
		Remediation: "auto",
		Cooldown:    30 * time.Second,
	}

	policy := &Policy{
		Global:           GlobalConfig{RemediateThreshold: 8, AlertThreshold: 4},
		TrustedProcesses: []string{"systemd", "kubelet"},
		Sysctl:           map[string]SysctlPolicy{},
	}

	cm := NewCooldownManager()

	decision := simulateEvent(
		event,
		"vm.swappiness",
		policyEntry,
		policy,
		cm,
		"20", // differs from expected
	)

	if decision.Action != "allow" {
		t.Errorf("expected 'allow' for trusted process + low-risk param, got '%s'", decision.Action)
	}

	// Should NOT be in cooldown (was allowed, not remediated)
	if cm.InCooldown("vm.swappiness", 30*time.Second) {
		t.Error("expected parameter to NOT be in cooldown when allowed")
	}
}

func TestSimulateEvent_PolicyForbidsAutoRemediation(t *testing.T) {
	// Scenario: High-risk drift detected, but policy forbids auto-remediation
	// Expected: Alert (downgraded from remediate by policy)

	event := WorkEvent{
		Pid:      1111,
		Process:  "untrustworthy",
		Access:   "WRITE",
		FilePath: "/proc/sys/kernel/kptr_restrict",
	}

	policyEntry := SysctlPolicy{
		Expected:    "2",
		Category:    "security",
		Criticality: "high",
		Remediation: "alert", // Policy forbids auto-remediation
		Cooldown:    30 * time.Second,
	}

	policy := &Policy{
		Global:           GlobalConfig{RemediateThreshold: 8, AlertThreshold: 4},
		TrustedProcesses: []string{},
		Sysctl:           map[string]SysctlPolicy{},
	}

	cm := NewCooldownManager()

	decision := simulateEvent(
		event,
		"kernel.kptr_restrict",
		policyEntry,
		policy,
		cm,
		"1", // differs from expected
	)

	// Even though hard rule would say "remediate", policy downgrade to "alert"
	if decision.Action != "alert" {
		t.Errorf("expected 'alert' when policy forbids auto-remediation, got '%s'", decision.Action)
	}

	// Should NOT be recorded in cooldown (was alerted, not remediated)
	if cm.InCooldown("kernel.kptr_restrict", 30*time.Second) {
		t.Error("expected parameter to NOT be in cooldown when only alerted")
	}
}

func TestSimulateEvent_NoConfiguredThresholds(t *testing.T) {
	// Scenario: Global config has no thresholds (uses defaults)
	// Expected: Defaults applied correctly

	event := WorkEvent{
		Pid:      2222,
		Process:  "unknown",
		Access:   "WRITE",
		FilePath: "/proc/sys/net/ipv4/ip_forward",
	}

	policyEntry := SysctlPolicy{
		Expected:    "0",
		Category:    "security",
		Criticality: "medium",
		Remediation: "auto",
		Cooldown:    30 * time.Second,
	}

	policy := &Policy{
		Global: GlobalConfig{
			RemediateThreshold: 0, // Not configured, will use default
			AlertThreshold:     0, // Not configured, will use default
		},
		TrustedProcesses: []string{},
		Sysctl:           map[string]SysctlPolicy{},
	}

	cm := NewCooldownManager()

	decision := simulateEvent(
		event,
		"net.ipv4.ip_forward",
		policyEntry,
		policy,
		cm,
		"1", // differs from expected
	)

	// Score = 3 (untrusted) + 2 (security) = 5
	// Default remediate threshold = 8, alert threshold = 4
	// 5 >= 4 && 5 < 8 → "alert"
	if decision.Action != "alert" {
		t.Errorf("expected 'alert' with default thresholds, got '%s'", decision.Action)
	}
}

func TestSimulateEvent_DeterministicBehavior(t *testing.T) {
	// Scenario: Same event run twice should produce identical results
	// Expected: Deterministic decision (same action, score, reasons)

	event := WorkEvent{
		Pid:      5555,
		Process:  "consistent-process",
		Access:   "WRITE",
		FilePath: "/proc/sys/kernel/dmesg_restrict",
	}

	policyEntry := SysctlPolicy{
		Expected:    "1",
		Category:    "security",
		Criticality: "high",
		Remediation: "auto",
		Cooldown:    30 * time.Second,
	}

	policy := &Policy{
		Global:           GlobalConfig{RemediateThreshold: 8, AlertThreshold: 4},
		TrustedProcesses: []string{},
		Sysctl:           map[string]SysctlPolicy{},
	}

	// Run 1: Fresh cooldown manager
	cm1 := NewCooldownManager()
	decision1 := simulateEvent(event, "kernel.dmesg_restrict", policyEntry, policy, cm1, "0")

	// Run 2: Fresh cooldown manager (independent)
	cm2 := NewCooldownManager()
	decision2 := simulateEvent(event, "kernel.dmesg_restrict", policyEntry, policy, cm2, "0")

	if decision1.Action != decision2.Action {
		t.Errorf("non-deterministic action: run1=%s, run2=%s", decision1.Action, decision2.Action)
	}
	if decision1.Score != decision2.Score {
		t.Errorf("non-deterministic score: run1=%d, run2=%d", decision1.Score, decision2.Score)
	}
	if len(decision1.Reasons) != len(decision2.Reasons) {
		t.Errorf("non-deterministic reasons: run1 has %d, run2 has %d", len(decision1.Reasons), len(decision2.Reasons))
	}
}

func TestSimulateEvent_FullIntegration(t *testing.T) {
	// Scenario: Complete integration test of full pipeline
	// - Build policy
	// - Process multiple events
	// - Verify state transitions

	policy := &Policy{
		Global: GlobalConfig{
			RemediateThreshold: 8,
			AlertThreshold:     4,
		},
		TrustedProcesses: []string{"kubelet", "systemd"},
		Sysctl: map[string]SysctlPolicy{
			"kernel.randomize_va_space": {
				Expected:    "2",
				Category:    "security",
				Criticality: "high",
				Remediation: "auto",
				Cooldown:    50 * time.Millisecond,
			},
		},
	}

	cm := NewCooldownManager()
	param := "kernel.randomize_va_space"
	policyEntry := policy.Sysctl[param]

	// Step 1: Malicious app triggers remediation
	event1 := WorkEvent{
		Pid:      9999,
		Process:  "malicious-app",
		Access:   "WRITE",
		FilePath: "/proc/sys/kernel/randomize_va_space",
	}
	decision1 := simulateEvent(event1, param, policyEntry, policy, cm, "0")
	if decision1.Action != "remediate" {
		t.Errorf("step 1: expected 'remediate', got '%s'", decision1.Action)
	}

	// Step 2: Immediate retry should be blocked
	event2 := WorkEvent{
		Pid:      9999,
		Process:  "malicious-app",
		Access:   "WRITE",
		FilePath: "/proc/sys/kernel/randomize_va_space",
	}
	decision2 := simulateEvent(event2, param, policyEntry, policy, cm, "0")
	if decision2.Action != "alert" {
		t.Errorf("step 2: expected 'alert' (blocked by cooldown), got '%s'", decision2.Action)
	}

	// Step 3: After cooldown expires, remediation allowed
	time.Sleep(75 * time.Millisecond)
	event3 := WorkEvent{
		Pid:      9999,
		Process:  "malicious-app",
		Access:   "WRITE",
		FilePath: "/proc/sys/kernel/randomize_va_space",
	}
	decision3 := simulateEvent(event3, param, policyEntry, policy, cm, "0")
	if decision3.Action != "remediate" {
		t.Errorf("step 3: expected 'remediate' (cooldown expired), got '%s'", decision3.Action)
	}

	// Step 4: Trusted process should never be blocked
	event4 := WorkEvent{
		Pid:      100,
		Process:  "kubelet",
		Access:   "WRITE",
		FilePath: "/proc/sys/kernel/randomize_va_space",
	}
	decision4 := simulateEvent(event4, param, policyEntry, policy, cm, "0")
	// Score for trusted process modifying security/high:
	// +5 (high criticality) +2 (security category) -2 (trusted) = 5
	// 5 >= alertThreshold (4), so action = "alert" (not remediate)
	// Trusted processes still alert on critical changes, just don't auto-remediate
	if decision4.Action != "alert" {
		t.Errorf("step 4: expected 'alert' for trusted process, got '%s'", decision4.Action)
	}
}
