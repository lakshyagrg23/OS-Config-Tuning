package main

import (
	"fmt"
	"math"

	"drift-agent/stats"
)

// Decision represents the outcome of a policy evaluation for a sysctl drift event.
// It determines whether the agent should allow, alert, or remediate.
type Decision struct {
	Action  string   `json:"action"`  // "allow", "alert", or "remediate"
	Score   int      `json:"score"`   // Risk score from 0-10+
	Reasons []string `json:"reasons"` // Explanation of the decision

	BaseScore            int     `json:"-"`
	RiskMultiplier       float64 `json:"-"`
	AnomalyMultiplier    float64 `json:"-"`
	BurstMultiplier      float64 `json:"-"`
	ReputationMultiplier float64 `json:"-"`
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
			Action:               "allow",
			Score:                0,
			Reasons:              []string{"allowed process override"},
			BaseScore:            0,
			RiskMultiplier:       1,
			AnomalyMultiplier:    1,
			BurstMultiplier:      1,
			ReputationMultiplier: 1,
		}
	}

	// Risk Scoring Phase
	baseScore := 0
	reasons := []string{}
	hardRule2 := false

	// Hard Rule 2: Critical security violation from untrusted process
	if ctx.Category == "security" && ctx.Criticality == "high" && !ctx.IsTrustedProcess {
		hardRule2 = true
		baseScore = 10
		reasons = []string{"untrusted process modifying high-critical security parameter"}
		// Don't return yet - apply policy remediation mode check below
	} else {
		// Scoring rule: untrusted process
		if !ctx.IsTrustedProcess {
			baseScore += 3
			reasons = append(reasons, "untrusted process")
		}

		// Scoring rule: high criticality
		if ctx.Criticality == "high" {
			baseScore += 5
			reasons = append(reasons, "high criticality parameter")
		}

		// Scoring rule: security category
		if ctx.Category == "security" {
			baseScore += 2
			reasons = append(reasons, "security-related parameter")
		}

		// Scoring rule: trusted process (negative score)
		if ctx.IsTrustedProcess {
			baseScore -= 2
			reasons = append(reasons, "trusted process (reduced risk)")
		}

		// Ensure score doesn't go below 0
		if baseScore < 0 {
			baseScore = 0
		}
	}

	// Streaming intelligence fusion (optional).
	anomalyMultiplier := 1.0
	burstMultiplier := 1.0
	reputationMultiplier := 1.0
	totalMultiplier := 1.0

	if ctx.HasIntel {
		// Prefer fused values already computed upstream; fall back to fusing locally.
		anomalySev := ctx.Intel.BehavioralAnomalySeverity
		burst := ctx.Intel.Burst
		if anomalySev == "" {
			_, anomalySev, _ = stats.FuseBehavioralSignals(ctx.Intel.ProcessWrite, ctx.Intel.ParamDrift)
		}
		if burst.Window == 0 && burst.Threshold == 0 {
			_, _, burst = stats.FuseBehavioralSignals(ctx.Intel.ProcessWrite, ctx.Intel.ParamDrift)
		}

		switch anomalySev {
		case stats.SeverityHigh:
			anomalyMultiplier = 1.30
		case stats.SeverityMedium:
			anomalyMultiplier = 1.15
		}

		// Burst impact scales with normalized score; capped at +25%.
		if burst.Score > 0 {
			burstMultiplier = 1.0 + 0.25*burst.Score
			if burstMultiplier < 1.0 {
				burstMultiplier = 1.0
			}
		}

		rep := ctx.Intel.ProcessReputationBefore.Score
		if rep < 0.5 {
			reputationMultiplier = 1.0 + (0.5-rep)*0.3
		} else {
			reputationMultiplier = 1.0 - (rep-0.5)*0.1
		}
		if reputationMultiplier < 0.9 {
			reputationMultiplier = 0.9
		}
		if reputationMultiplier > 1.2 {
			reputationMultiplier = 1.2
		}

		totalMultiplier = anomalyMultiplier * burstMultiplier * reputationMultiplier
		if totalMultiplier < 1.0 && hardRule2 {
			// Hard rule 2 is a deterministic floor.
			totalMultiplier = 1.0
		}

		if anomalyMultiplier != 1.0 {
			reasons = append(reasons, fmt.Sprintf("anomaly multiplier %.2fx (%s)", anomalyMultiplier, anomalySev))
		}
		if burstMultiplier != 1.0 {
			reasons = append(reasons, fmt.Sprintf("burst multiplier %.2fx (count=%d window=%s threshold=%d)", burstMultiplier, burst.Count, burst.Window, burst.Threshold))
		}
		if reputationMultiplier != 1.0 {
			reasons = append(reasons, fmt.Sprintf("reputation multiplier %.2fx (score=%.2f stable_for=%s)", reputationMultiplier, rep, ctx.Intel.ProcessReputationBefore.StableFor))
		}
	}

	finalScore := baseScore
	if ctx.HasIntel {
		finalScore = int(math.Round(float64(baseScore) * totalMultiplier))
		if finalScore < 0 {
			finalScore = 0
		}
		if hardRule2 && finalScore < baseScore {
			finalScore = baseScore
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

	if finalScore >= remediateThreshold {
		action = "remediate"
	} else if finalScore >= alertThreshold {
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
		Action:               action,
		Score:                finalScore,
		Reasons:              reasons,
		BaseScore:            baseScore,
		RiskMultiplier:       totalMultiplier,
		AnomalyMultiplier:    anomalyMultiplier,
		BurstMultiplier:      burstMultiplier,
		ReputationMultiplier: reputationMultiplier,
	}
}
