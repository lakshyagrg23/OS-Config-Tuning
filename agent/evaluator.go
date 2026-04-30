package main

// Decision represents the outcome of a policy evaluation for a sysctl drift event.
// It determines whether the agent should allow, alert, or remediate.
type Decision struct {
	Action  string   `json:"action"`  // "allow", "alert", or "remediate"
	Score   int      `json:"score"`   // Risk score from 0-10+
	Reasons []string `json:"reasons"` // Explanation of the decision
}

// EvaluateDecision is the core decision engine that determines how to respond to
// a sysctl drift event. It follows a deterministic policy-driven pipeline with
// hard rules and risk scoring.
//
// Decision pipeline:
//  1. Hard rule: allowed process → "allow"
//  2. Hard rule: critical security violation from untrusted process → "remediate"
//  3. Risk scoring based on trust, criticality, and category
//  4. Threshold-based decision: remediate > alert > allow
//  5. Respect policy remediation mode ("alert" downgrade)
func EvaluateDecision(
	ctx Context,
	policyEntry SysctlPolicy,
	global GlobalConfig,
) Decision {
	// Hard Rule 1: Allowed process override
	if ctx.IsAllowedProcess {
		return Decision{
			Action:  "allow",
			Score:   0,
			Reasons: []string{"allowed process override"},
		}
	}

	// Risk Scoring Phase
	score := 0
	reasons := []string{}

	// Hard Rule 2: Critical security violation from untrusted process
	if ctx.Category == "security" && ctx.Criticality == "high" && !ctx.IsTrustedProcess {
		score = 10
		reasons = []string{"untrusted process modifying high-critical security parameter"}
		// Don't return yet - apply policy remediation mode check below
	} else {
		// Scoring rule: untrusted process
		if !ctx.IsTrustedProcess {
			score += 3
			reasons = append(reasons, "untrusted process")
		}

		// Scoring rule: high criticality
		if ctx.Criticality == "high" {
			score += 5
			reasons = append(reasons, "high criticality parameter")
		}

		// Scoring rule: security category
		if ctx.Category == "security" {
			score += 2
			reasons = append(reasons, "security-related parameter")
		}

		// Scoring rule: trusted process (negative score)
		if ctx.IsTrustedProcess {
			score -= 2
			reasons = append(reasons, "trusted process (reduced risk)")
		}

		// Ensure score doesn't go below 0
		if score < 0 {
			score = 0
		}
	}

	// Threshold-based decision
	var action string
	remediateThreshold := global.RemediateThreshold
	alertThreshold := global.AlertThreshold

	// Set sensible defaults if not configured
	if remediateThreshold <= 0 {
		remediateThreshold = 8
	}
	if alertThreshold <= 0 {
		alertThreshold = 4
	}

	if score >= remediateThreshold {
		action = "remediate"
	} else if score >= alertThreshold {
		action = "alert"
	} else {
		action = "allow"
	}

	// Respect policy remediation mode: downgrade "remediate" to "alert" if policy forbids auto-remediation
	if action == "remediate" && policyEntry.Remediation == "alert" {
		action = "alert"
		reasons = append(reasons, "policy forbids auto-remediation (downgraded to alert)")
	}

	return Decision{
		Action:  action,
		Score:   score,
		Reasons: reasons,
	}
}
