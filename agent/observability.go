package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"drift-agent/agent/controlplane"
)

// TraceLog captures the complete lifecycle of a sysctl drift event.
// It records context, decision logic, and final action for observability.
type TraceLog struct {
	// Timing and identification
	Timestamp time.Time `json:"timestamp"`

	// Event details
	Param    string `json:"param"`
	Process  string `json:"process"`
	Actual   string `json:"actual"`
	Expected string `json:"expected"`

	// Policy context
	Category    string `json:"category"`
	Criticality string `json:"criticality"`
	Trusted     bool   `json:"trusted"`
	Allowed     bool   `json:"allowed"`

	// Decision phase
	DecisionAction       string   `json:"decisionAction"` // <-- CHANGED
    Score                int      `json:"score"`
    BaseScore            int      `json:"base_score"`
    RiskMultiplier       float64  `json:"risk_multiplier"`
    AnomalyMultiplier    float64  `json:"anomaly_multiplier"`
    BurstMultiplier      float64  `json:"burst_multiplier"`
    ReputationMultiplier float64  `json:"reputation_multiplier"`
    Reasons              []string `json:"reasons"`

	// Streaming intelligence (optional)
	IntelEnabled bool `json:"intel_enabled"`

	ProcessWriteEWMA            float64 `json:"process_write_ewma_frequency"`
	ProcessWriteStdDev          float64 `json:"process_write_stddev"`
	ProcessWriteZScore          float64 `json:"process_write_zscore"`
	ProcessWriteAnomalous       bool    `json:"process_write_anomalous"`
	ProcessWriteAnomalySeverity string  `json:"process_write_anomaly_severity"`

	ParamDriftEWMA            float64 `json:"param_drift_ewma_frequency"`
	ParamDriftStdDev          float64 `json:"param_drift_stddev"`
	ParamDriftZScore          float64 `json:"param_drift_zscore"`
	ParamDriftAnomalous       bool    `json:"param_drift_anomalous"`
	ParamDriftAnomalySeverity string  `json:"param_drift_anomaly_severity"`

	RemediationEWMA            float64 `json:"remediation_ewma_frequency"`
	RemediationStdDev          float64 `json:"remediation_stddev"`
	RemediationZScore          float64 `json:"remediation_zscore"`
	RemediationAnomalous       bool    `json:"remediation_anomalous"`
	RemediationAnomalySeverity string  `json:"remediation_anomaly_severity"`

	BehavioralAnomaly bool   `json:"behavioral_anomaly"`
	AnomalySeverity   string `json:"anomaly_severity"`

	BurstScore     float64 `json:"burst_score"`
	BurstSeverity  string  `json:"burst_severity"`
	BurstCount     int     `json:"burst_count"`
	BurstWindowMs  int     `json:"burst_window_ms"`
	BurstThreshold int     `json:"burst_threshold"`

	ProcessReputation            float64 `json:"process_reputation"`
	ProcessReputationAfter       float64 `json:"process_reputation_after"`
	ProcessStableForMs           int64   `json:"process_stable_for_ms"`
	ProcessAnomalyCount          int64   `json:"process_anomaly_count"`
	ProcessRemediationAssocCount int64   `json:"process_remediation_assoc_count"`
	ProcessConflictAssocCount    int64   `json:"process_conflict_assoc_count"`

	// Cooldown interaction
	// Cooldown interaction
    CooldownApplied bool `json:"cooldownApplied"` // <-- CHANGED
    CooldownWindow  int  `json:"cooldown_window_ms"` 

    // Conflict detection
    ConflictDetected  bool `json:"conflictDetected"` // <-- CHANGED
    ConflictThreshold int  `json:"conflict_threshold,omitempty"` 

    // Final disposition
    FinalAction string `json:"finalAction"` // <-- CHANGE
}

// EmitTrace serializes a TraceLog to JSON and writes it to stdout.
// Non-blocking: errors are logged to stderr but don't affect agent operation.
func EmitTrace(log TraceLog) {
	// Ensure timestamp is set
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trace error: failed to marshal log: %v\n", err)
		return
	}

	// Write to stdout with newline
	fmt.Println(string(jsonData))

	// Non-blocking: enqueue for asynchronous upload to control plane
	controlplane.EnqueueToDefault(log)
}

// EmitTraceWithIndent serializes a TraceLog to indented JSON for human readability.
// Useful for debugging; EmitTrace() is preferred for production.
func EmitTraceWithIndent(log TraceLog) {
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}

	jsonData, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "trace error: failed to marshal log with indent: %v\n", err)
		return
	}

	fmt.Println(string(jsonData))
}

// BuildTraceLog constructs a TraceLog from a Context, Decision, and state flags.
// This helper reduces boilerplate at the call site by automatically populating
// all fields from the context enrichment and decision pipeline.
func BuildTraceLog(
	ctx Context,
	decision Decision,
	cooldownApplied bool,
	conflictDetected bool,
	finalAction string,
) TraceLog {
	return TraceLog{
		Timestamp:            time.Now(),
		Param:                ctx.Param,
		Process:              ctx.Process,
		Actual:               ctx.Actual,
		Expected:             ctx.Expected,
		Category:             ctx.Category,
		Criticality:          ctx.Criticality,
		Trusted:              ctx.IsTrustedProcess,
		Allowed:              ctx.IsAllowedProcess,
		DecisionAction:       decision.Action,
		Score:                decision.Score,
		BaseScore:            decision.BaseScore,
		RiskMultiplier:       decision.RiskMultiplier,
		AnomalyMultiplier:    decision.AnomalyMultiplier,
		BurstMultiplier:      decision.BurstMultiplier,
		ReputationMultiplier: decision.ReputationMultiplier,
		Reasons:              decision.Reasons,
		IntelEnabled:         ctx.HasIntel,

		ProcessWriteEWMA:            ctx.Intel.ProcessWrite.EWMA,
		ProcessWriteStdDev:          ctx.Intel.ProcessWrite.StdDev,
		ProcessWriteZScore:          ctx.Intel.ProcessWrite.Anomaly.ZScore,
		ProcessWriteAnomalous:       ctx.Intel.ProcessWrite.Anomaly.IsAnomalous,
		ProcessWriteAnomalySeverity: string(ctx.Intel.ProcessWrite.Anomaly.Severity),

		ParamDriftEWMA:            ctx.Intel.ParamDrift.EWMA,
		ParamDriftStdDev:          ctx.Intel.ParamDrift.StdDev,
		ParamDriftZScore:          ctx.Intel.ParamDrift.Anomaly.ZScore,
		ParamDriftAnomalous:       ctx.Intel.ParamDrift.Anomaly.IsAnomalous,
		ParamDriftAnomalySeverity: string(ctx.Intel.ParamDrift.Anomaly.Severity),

		RemediationEWMA:            ctx.Intel.Remediation.EWMA,
		RemediationStdDev:          ctx.Intel.Remediation.StdDev,
		RemediationZScore:          ctx.Intel.Remediation.Anomaly.ZScore,
		RemediationAnomalous:       ctx.Intel.Remediation.Anomaly.IsAnomalous,
		RemediationAnomalySeverity: string(ctx.Intel.Remediation.Anomaly.Severity),

		BehavioralAnomaly: ctx.Intel.BehavioralAnomaly,
		AnomalySeverity:   string(ctx.Intel.BehavioralAnomalySeverity),

		BurstScore:     ctx.Intel.Burst.Score,
		BurstSeverity:  string(ctx.Intel.Burst.Severity),
		BurstCount:     ctx.Intel.Burst.Count,
		BurstWindowMs:  int(ctx.Intel.Burst.Window.Milliseconds()),
		BurstThreshold: ctx.Intel.Burst.Threshold,

		ProcessReputation:            ctx.Intel.ProcessReputationBefore.Score,
		ProcessReputationAfter:       ctx.Intel.ProcessReputationAfter.Score,
		ProcessStableForMs:           int64(ctx.Intel.ProcessReputationBefore.StableFor.Milliseconds()),
		ProcessAnomalyCount:          ctx.Intel.ProcessReputationBefore.AnomalyCount,
		ProcessRemediationAssocCount: ctx.Intel.ProcessReputationBefore.RemediationAssocCnt,
		ProcessConflictAssocCount:    ctx.Intel.ProcessReputationBefore.ConflictAssocCnt,
		CooldownApplied:              cooldownApplied,
		ConflictDetected:             conflictDetected,
		FinalAction:                  finalAction,
	}
}
