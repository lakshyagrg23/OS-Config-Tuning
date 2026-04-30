package main

import (
	"testing"
)

func TestEvaluateDecision_AllowedProcess(t *testing.T) {
	// Allowed process should always result in "allow" action
	ctx := Context{
		Param:            "net.ipv4.ip_forward",
		Expected:         "0",
		Actual:           "1",
		Category:         "security",
		Criticality:      "high",
		Process:          "kube-proxy",
		IsTrustedProcess: false,
		IsAllowedProcess: true, // <-- allowed by policy
	}

	policyEntry := SysctlPolicy{
		Expected:    "0",
		Category:    "security",
		Criticality: "high",
		Remediation: "auto",
	}

	global := GlobalConfig{
		RemediateThreshold: 8,
		AlertThreshold:     4,
	}

	decision := EvaluateDecision(ctx, policyEntry, global)

	if decision.Action != "allow" {
		t.Errorf("expected action 'allow', got '%s'", decision.Action)
	}
	if decision.Score != 0 {
		t.Errorf("expected score 0, got %d", decision.Score)
	}
}

func TestEvaluateDecision_CriticalSecurityViolation(t *testing.T) {
	// Untrusted process modifying high-critical security param = immediate remediation
	ctx := Context{
		Param:            "kernel.randomize_va_space",
		Expected:         "2",
		Actual:           "0",
		Category:         "security",
		Criticality:      "high",
		Process:          "unknown-app",
		IsTrustedProcess: false, // <-- untrusted
		IsAllowedProcess: false,
	}

	policyEntry := SysctlPolicy{
		Expected:    "2",
		Category:    "security",
		Criticality: "high",
		Remediation: "auto",
	}

	global := GlobalConfig{
		RemediateThreshold: 8,
		AlertThreshold:     4,
	}

	decision := EvaluateDecision(ctx, policyEntry, global)

	if decision.Action != "remediate" {
		t.Errorf("expected action 'remediate', got '%s'", decision.Action)
	}
	if decision.Score != 10 {
		t.Errorf("expected score 10 (hard rule), got %d", decision.Score)
	}
}

func TestEvaluateDecision_RiskScoring(t *testing.T) {
	tests := []struct {
		name           string
		trusted        bool
		criticality    string
		category       string
		expectedScore  int
		expectedAction string
	}{
		{
			name:           "untrusted + high critical + security",
			trusted:        false,
			criticality:    "high",
			category:       "security",
			expectedScore:  10, // 3 + 5 + 2
			expectedAction: "remediate",
		},
		{
			name:           "untrusted + high critical + performance",
			trusted:        false,
			criticality:    "high",
			category:       "performance",
			expectedScore:  8, // 3 + 5
			expectedAction: "remediate",
		},
		{
			name:           "untrusted + low criticality + security",
			trusted:        false,
			criticality:    "low",
			category:       "security",
			expectedScore:  5, // 3 + 2
			expectedAction: "alert",
		},
		{
			name:           "trusted + high critical + security",
			trusted:        true,
			criticality:    "high",
			category:       "security",
			expectedScore:  5, // -2 + 5 + 2
			expectedAction: "alert",
		},
		{
			name:           "trusted + low criticality + performance",
			trusted:        true,
			criticality:    "low",
			category:       "performance",
			expectedScore:  0, // -2 (clamped to 0)
			expectedAction: "allow",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := Context{
				Param:            "test.param",
				Expected:         "expected",
				Actual:           "actual",
				Category:         test.category,
				Criticality:      test.criticality,
				Process:          "test-process",
				IsTrustedProcess: test.trusted,
				IsAllowedProcess: false,
			}

			policyEntry := SysctlPolicy{
				Expected:    "expected",
				Remediation: "auto",
			}

			global := GlobalConfig{
				RemediateThreshold: 8,
				AlertThreshold:     4,
			}

			decision := EvaluateDecision(ctx, policyEntry, global)

			if decision.Score != test.expectedScore {
				t.Errorf("score: expected %d, got %d", test.expectedScore, decision.Score)
			}
			if decision.Action != test.expectedAction {
				t.Errorf("action: expected '%s', got '%s'", test.expectedAction, decision.Action)
			}
		})
	}
}

func TestEvaluateDecision_PolicyRemediationMode(t *testing.T) {
	// If policy says "alert", remediate decisions should be downgraded to "alert"
	ctx := Context{
		Param:            "kernel.randomize_va_space",
		Expected:         "2",
		Actual:           "0",
		Category:         "security",
		Criticality:      "high",
		Process:          "unknown",
		IsTrustedProcess: false,
		IsAllowedProcess: false,
	}

	policyEntry := SysctlPolicy{
		Expected:    "2",
		Category:    "security",
		Criticality: "high",
		Remediation: "alert", // <-- forbid auto-remediation
	}

	global := GlobalConfig{
		RemediateThreshold: 8,
		AlertThreshold:     4,
	}

	decision := EvaluateDecision(ctx, policyEntry, global)

	// Even though hard rule would say "remediate", downgrades to "alert"
	if decision.Action != "alert" {
		t.Errorf("expected action 'alert' (downgraded from remediate), got '%s'", decision.Action)
	}
	if decision.Score != 10 {
		t.Errorf("score should remain 10, got %d", decision.Score)
	}

	// Should include the downgrade reason
	foundReason := false
	for _, reason := range decision.Reasons {
		if reason == "policy forbids auto-remediation (downgraded to alert)" {
			foundReason = true
			break
		}
	}
	if !foundReason {
		t.Errorf("expected downgrade reason in decision, got: %v", decision.Reasons)
	}
}

func TestEvaluateDecision_DefaultThresholds(t *testing.T) {
	// When thresholds are not set (0), should use sensible defaults
	ctx := Context{
		Param:            "test.param",
		Expected:         "1",
		Actual:           "0",
		Category:         "performance",
		Criticality:      "medium",
		Process:          "unknown",
		IsTrustedProcess: false,
		IsAllowedProcess: false,
	}

	policyEntry := SysctlPolicy{Expected: "1"}

	global := GlobalConfig{
		RemediateThreshold: 0, // <-- not set
		AlertThreshold:     0, // <-- not set
	}

	decision := EvaluateDecision(ctx, policyEntry, global)

	// Score = 3 (untrusted), should use default remediate=8, alert=4
	// 3 < 4 < 8, so should be "allow"
	if decision.Action != "allow" {
		t.Errorf("with defaults and score 3, expected 'allow', got '%s'", decision.Action)
	}
}

func TestEvaluateDecision_HasReasons(t *testing.T) {
	// Decision should include reasons for auditability
	ctx := Context{
		Param:            "net.ipv4.ip_forward",
		Expected:         "0",
		Actual:           "1",
		Category:         "security",
		Criticality:      "high",
		Process:          "unknown",
		IsTrustedProcess: false,
		IsAllowedProcess: false,
	}

	policyEntry := SysctlPolicy{
		Expected:    "0",
		Remediation: "auto",
	}

	global := GlobalConfig{
		RemediateThreshold: 8,
		AlertThreshold:     4,
	}

	decision := EvaluateDecision(ctx, policyEntry, global)

	if len(decision.Reasons) == 0 {
		t.Errorf("expected reasons in decision, got empty list")
	}

	// Should have multiple reasons for audit trail
	if decision.Action == "remediate" && len(decision.Reasons) < 1 {
		t.Errorf("remediation decision should have at least 1 reason, got %d", len(decision.Reasons))
	}
}
