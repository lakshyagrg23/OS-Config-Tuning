package main

// simulateEvent orchestrates the full decision pipeline from a raw event
// to a final decision, incorporating context building, policy evaluation,
// and cooldown management.
//
// Pipeline:
//  1. Build context from event + policy
//  2. Evaluate decision using policy rules
//  3. Apply cooldown logic (downgrade if blocked by cooldown)
//  4. Record remediation in cooldown manager (if remediate action)
//  5. Return final decision
//
// This function is deterministic and contains no side effects beyond
// updating the cooldown manager.
func simulateEvent(
	event WorkEvent,
	param string,
	policyEntry SysctlPolicy,
	policy *Policy,
	cm *CooldownManager,
	actual string,
) Decision {
	// Step 1: Build enriched context from event and policy
	ctx := BuildContext(event, param, policyEntry, actual, policy)

	// Step 2: Evaluate decision using policy-driven engine
	decision := EvaluateDecision(ctx, policyEntry, policy.Global)

	// Step 3: Apply cooldown logic
	// If decision is "remediate" but parameter is in cooldown,
	// downgrade to "alert" to respect the cooldown period.
	if decision.Action == "remediate" && cm.InCooldown(param, policyEntry.Cooldown) {
		decision.Action = "alert"
		decision.Reasons = append(decision.Reasons, "remediation blocked by cooldown")
	}

	// Step 4: Record remediation for future cooldown tracking
	// Only record if the final decision is to remediate (after cooldown check).
	if decision.Action == "remediate" {
		cm.Record(param)
	}

	// Step 5: Return final decision
	return decision
}
