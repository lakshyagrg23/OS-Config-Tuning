/*
This file demonstrates how CooldownManager integrates with the drift-agent
to prevent infinite remediation loops.

USAGE PATTERN:

    agent/main.go:
    =====================
    // Initialize cooldown manager at startup
    cm := NewCooldownManager()

    // In the worker pool (agent/worker.go):
    func processEvent(event WorkEvent, policy *Policy, cm *CooldownManager) {
        // ... earlier steps ...

        param := ResolveParameter(event.FilePath)
        policyEntry := policy.Sysctl[param]

        // Check if parameter is in cooldown
        if cm.InCooldown(param, policyEntry.Cooldown) {
            fmt.Printf("⏱ Parameter %s is in cooldown, skipping\n", param)
            return
        }

        // ... drift detection ...

        // Get decision
        ctx := BuildContext(event, param, policyEntry, actual, policy)
        decision := EvaluateDecision(ctx, policyEntry, policy.Global)

        // Apply remediation if needed
        if decision.Action == "remediate" {
            err := ApplyRemediation(param, policyEntry.Expected)
            if err == nil {
                // Record only on successful remediation
                cm.Record(param)
            }
        }
    }

DESIGN RATIONALE:

1. Thread-Safe: RWMutex allows many concurrent readers (InCooldown checks)
   and exclusive writers (Record updates).

2. Efficient: RLock is cheaper than Lock, so InCooldown has minimal overhead.

3. Per-Parameter: Each parameter tracks its own cooldown independently.

4. Time-Based: Uses elapsed time since last remediation, immune to clock skew.

5. Graceful: Untracked parameters never block (false on first check).

6. Simple API: Two methods (InCooldown, Record) hide complexity.

COOLDOWN FLOW:

    T=0:  remediation attempt 1
          ↓→ InCooldown("param", 30s) = false (allowed)
          ↓→ ApplyRemediation succeeds
          ↓→ Record("param")

    T=5:  remediation attempt 2
          ↓→ InCooldown("param", 30s) = true (blocked)
          ↓→ Skip remediation

    T=31: remediation attempt 3
          ↓→ InCooldown("param", 30s) = false (cooldown expired)
          ↓→ ApplyRemediation succeeds
          ↓→ Record("param")
*/

package main
