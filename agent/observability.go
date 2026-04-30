package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
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
	DecisionAction string   `json:"decision_action"` // "allow", "alert", or "remediate"
	Score          int      `json:"score"`
	Reasons        []string `json:"reasons"`

	// Cooldown interaction
	CooldownApplied bool `json:"cooldown_applied"`
	CooldownWindow  int  `json:"cooldown_window_ms"` // Window duration in milliseconds

	// Conflict detection
	ConflictDetected   bool `json:"conflict_detected"`
	ConflictThreshold  int  `json:"conflict_threshold,omitempty"` // Only if detected

	// Final disposition
	FinalAction string `json:"final_action"` // Action after cooldown + conflict adjustments
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
		Timestamp:        time.Now(),
		Param:            ctx.Param,
		Process:          ctx.Process,
		Actual:           ctx.Actual,
		Expected:         ctx.Expected,
		Category:         ctx.Category,
		Criticality:      ctx.Criticality,
		Trusted:          ctx.IsTrustedProcess,
		Allowed:          ctx.IsAllowedProcess,
		DecisionAction:   decision.Action,
		Score:            decision.Score,
		Reasons:          decision.Reasons,
		CooldownApplied:  cooldownApplied,
		ConflictDetected: conflictDetected,
		FinalAction:      finalAction,
	}
}
