package main

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"drift-agent/stats"
)

// agentPID stores the PID of this agent process to prevent infinite loops
// when the agent itself executes remediation commands.
var agentPID uint32

func init() {
	agentPID = uint32(os.Getpid())
}

// WorkerContext holds state managers for coordinating decisions across events.
type WorkerContext struct {
	policy          *Policy
	cooldownManager *CooldownManager
	conflictManager *ConflictManager
	intel           *stats.Engine
}

// StartWorkerPool launches runtime.NumCPU() goroutines that drain eventQueue.
// Each worker calls processEvent for every WorkEvent it receives.
// The returned *sync.WaitGroup will be released once all workers finish
// (i.e. after eventQueue is closed).
func StartWorkerPool(eventQueue <-chan WorkEvent, policy *Policy) *sync.WaitGroup {
	numWorkers := runtime.NumCPU()
	fmt.Printf("Starting %d worker(s)\n", numWorkers)

	// Initialize state managers
	cooldownManager := NewCooldownManager()
	conflictManager := NewConflictManager()
	intel := stats.NewEngine(stats.Config{})
	if policy != nil {
		intel = stats.NewEngine(policy.Global.Stats)
	}

	ctx := &WorkerContext{
		policy:          policy,
		cooldownManager: cooldownManager,
		conflictManager: conflictManager,
		intel:           intel,
	}

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for event := range eventQueue {
				processEvent(event, ctx)
			}
		}(i)
	}
	return &wg
}

// processEvent runs the full policy pipeline for a single WorkEvent.
// The pipeline: event → context → decision → cooldown → conflict → action → trace
func processEvent(event WorkEvent, ctx *WorkerContext) {
	// 0. Ignore self-events (prevent loops)
	if event.Pid == agentPID {
		return
	}

	// 1. Only process WRITE operations (ignore READ)
	if event.Access != "WRITE" {
		return
	}

	// 2. Resolve parameter name
	param := ResolveParameter(event.FilePath)
	if param == "" {
		return
	}

	// 3. Check policy exists for this parameter
	policyEntry, ok := ctx.policy.Sysctl[param]
	if !ok {
		return
	}

	now := time.Now()
	processWriteMetrics := ctx.intel.ObserveProcessWrite(event.Process, now)
	processRepBefore := ctx.intel.GetReputation(event.Process, now)

	// 4. Read actual current value
	actual, err := ReadSysctlValue(param)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", param, err)
		return
	}

	// 5. Early exit if no drift detected
	if actual == policyEntry.Expected {
		ctx.intel.UpdateReputation(event.Process, now, stats.ReputationUpdate{
			WasDrift:        false,
			FinalAction:     "allow",
			AnomalySeverity: processWriteMetrics.Anomaly.Severity,
			BurstSeverity:   processWriteMetrics.Burst.Severity,
		})
		return
	}

	// 6. Build context (enriches event with policy data)
	eventCtx := BuildContext(event, param, policyEntry, actual, ctx.policy)
	paramDriftMetrics := ctx.intel.ObserveParamDrift(param, now)
	behavioralAnomaly, anomalySev, burst := stats.FuseBehavioralSignals(processWriteMetrics, paramDriftMetrics)
	eventCtx.HasIntel = true
	eventCtx.Intel = stats.EventIntel{
		ProcessWrite:              processWriteMetrics,
		ParamDrift:                paramDriftMetrics,
		ProcessReputationBefore:   processRepBefore,
		BehavioralAnomaly:         behavioralAnomaly,
		BehavioralAnomalySeverity: anomalySev,
		Burst:                     burst,
	}

	// 7. Make policy decision (hard rules, risk scoring, thresholds)
	decision := EvaluateDecision(eventCtx, policyEntry, ctx.policy.Global)

	// 8. Track conflict event (record this drift occurrence)
	ctx.conflictManager.Record(param)

	// 9. Apply cooldown logic
	cooldownApplied := false
	if decision.Action == "remediate" {
		cooldown := policyEntry.Cooldown
		if cooldown == 0 {
			cooldown = ctx.policy.Global.DefaultCooldown
		}

		if ctx.cooldownManager.InCooldown(param, cooldown) {
			decision.Action = "alert"
			cooldownApplied = true
		}
	}

	// 10. Apply conflict detection
	conflictDetected := false
	const conflictWindow = 10 * time.Second
	const conflictThreshold = 3

	if ctx.conflictManager.IsConflicting(param, conflictWindow, conflictThreshold) {
		if decision.Action == "remediate" {
			decision.Action = "alert"
		}
		conflictDetected = true
	}

	// 11. Determine final action after all policy layers
	finalAction := decision.Action

	// 12. Execute final action
	remediationSucceeded := false
	switch finalAction {
	case "remediate":
		err := ApplyRemediation(param, policyEntry.Expected)
		if err != nil {
			fmt.Printf("❌ REMEDIATION FAILED\n  Parameter: %s\n  Error: %v\n", param, err)
		} else {
			fmt.Printf("🔧 REMEDIATION APPLIED\n  Parameter: %s\n  Restored : %s\n", param, policyEntry.Expected)
			fmt.Printf("🔧 REMEDIATED %s → %s\n", param, policyEntry.Expected)
			remediationSucceeded = true

			// Record successful remediation for cooldown window
			ctx.cooldownManager.Record(param)

			// Update remediation frequency intelligence after success.
			eventCtx.Intel.Remediation = ctx.intel.ObserveRemediation(time.Now())
		}

	case "alert":
		fmt.Printf("⚠️  ALERT: drift detected on %s by %s (actual=%s, expected=%s)\n",
			param, event.Process, actual, policyEntry.Expected)

	case "allow":
		// intentionally no-op (allowed drift)
	}

	// 12.5 Update dynamic reputation based on final outcome.
	repAction := finalAction
	if finalAction == "remediate" && !remediationSucceeded {
		repAction = "alert"
	}
	repAfter := ctx.intel.UpdateReputation(event.Process, time.Now(), stats.ReputationUpdate{
		WasDrift:         true,
		FinalAction:      repAction,
		CooldownApplied:  cooldownApplied,
		ConflictDetected: conflictDetected,
		AnomalySeverity:  eventCtx.Intel.BehavioralAnomalySeverity,
		BurstSeverity:    eventCtx.Intel.Burst.Severity,
	})
	eventCtx.Intel.ProcessReputationAfter = repAfter

	// 13. Emit structured trace log for observability
	trace := BuildTraceLog(
		eventCtx,
		decision,
		cooldownApplied,
		conflictDetected,
		finalAction,
	)
	EmitTrace(trace)
}
