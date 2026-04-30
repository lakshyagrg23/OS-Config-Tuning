package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func captureOutput(fn func()) string {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestTraceLog_Empty(t *testing.T) {
	log := TraceLog{}
	if !log.Timestamp.IsZero() {
		t.Error("expected empty TraceLog to have zero timestamp")
	}
	if log.Param != "" {
		t.Error("expected empty TraceLog to have empty param")
	}
}

func TestTraceLog_CreationWithValues(t *testing.T) {
	now := time.Now()
	log := TraceLog{
		Timestamp:     now,
		Param:         "vm.swappiness",
		Process:       "kube-proxy",
		Actual:        "60",
		Expected:      "10",
		Category:      "performance",
		Criticality:   "medium",
		Trusted:       true,
		Allowed:       false,
		DecisionAction: "alert",
		Score:         5,
		Reasons:       []string{"process mismatch"},
		FinalAction:   "alert",
	}

	if log.Param != "vm.swappiness" {
		t.Errorf("param mismatch: expected vm.swappiness, got %s", log.Param)
	}
	if log.DecisionAction != "alert" {
		t.Errorf("decision mismatch: expected alert, got %s", log.DecisionAction)
	}
	if log.Score != 5 {
		t.Errorf("score mismatch: expected 5, got %d", log.Score)
	}
}

func TestEmitTrace_JSONFormat(t *testing.T) {
	log := TraceLog{
		Param:          "net.ipv4.ip_forward",
		Process:        "kube-proxy",
		Actual:         "1",
		Expected:       "0",
		Category:       "security",
		Criticality:    "high",
		Trusted:        false,
		Allowed:        false,
		DecisionAction: "remediate",
		Score:          10,
		Reasons:        []string{"untrusted process", "high criticality"},
		FinalAction:    "alert",
		CooldownApplied: true,
	}

	output := captureOutput(func() {
		EmitTrace(log)
	})

	output = strings.TrimSpace(output)

	// Verify it's valid JSON
	var unmarshaled TraceLog
	err := json.Unmarshal([]byte(output), &unmarshaled)
	if err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}

	if unmarshaled.Param != log.Param {
		t.Errorf("param not preserved in JSON")
	}
	if unmarshaled.DecisionAction != log.DecisionAction {
		t.Errorf("decision_action not preserved in JSON")
	}
	if unmarshaled.Score != log.Score {
		t.Errorf("score not preserved in JSON")
	}
	if len(unmarshaled.Reasons) != 2 {
		t.Errorf("reasons not preserved in JSON")
	}
}

func TestEmitTrace_TimestampSetting(t *testing.T) {
	log := TraceLog{
		Param: "test.param",
		// No timestamp set
	}

	output := captureOutput(func() {
		EmitTrace(log)
	})

	var unmarshaled TraceLog
	err := json.Unmarshal([]byte(strings.TrimSpace(output)), &unmarshaled)
	if err != nil {
		t.Errorf("failed to marshal: %v", err)
	}

	if unmarshaled.Timestamp.IsZero() {
		t.Error("expected EmitTrace to set timestamp")
	}
}

func TestEmitTraceWithIndent_Format(t *testing.T) {
	log := TraceLog{
		Param:          "vm.swappiness",
		DecisionAction: "remediate",
		Score:          8,
		Reasons:        []string{"reason1", "reason2"},
	}

	output := captureOutput(func() {
		EmitTraceWithIndent(log)
	})

	// Verify indentation (2-space)
	if !strings.Contains(output, "  ") {
		t.Error("expected indented output")
	}

	// Verify valid JSON
	var unmarshaled TraceLog
	err := json.Unmarshal([]byte(output), &unmarshaled)
	if err != nil {
		t.Errorf("indented output is not valid JSON: %v", err)
	}
}

func TestBuildTraceLog_Construction(t *testing.T) {
	ctx := Context{
		Param:              "vm.swappiness",
		Process:            "kube-proxy",
		Actual:             "60",
		Expected:           "10",
		Category:           "performance",
		Criticality:        "medium",
		IsTrustedProcess:   true,
		IsAllowedProcess:   false,
	}

	decision := Decision{
		Action:  "alert",
		Score:   5,
		Reasons: []string{"reason1", "reason2"},
	}

	log := BuildTraceLog(ctx, decision, false, false, "alert")

	if log.Param != "vm.swappiness" {
		t.Error("param not set correctly")
	}
	if log.Process != "kube-proxy" {
		t.Error("process not set correctly")
	}
	if log.DecisionAction != "alert" {
		t.Error("decision action not set correctly")
	}
	if log.Score != 5 {
		t.Error("score not set correctly")
	}
	if len(log.Reasons) != 2 {
		t.Error("reasons not copied correctly")
	}
	if log.CooldownApplied {
		t.Error("cooldown applied should be false")
	}
	if log.ConflictDetected {
		t.Error("conflict detected should be false")
	}
	if log.FinalAction != "alert" {
		t.Error("final action not set correctly")
	}
	if log.Timestamp.IsZero() {
		t.Error("timestamp should be set by BuildTraceLog")
	}
}

func TestBuildTraceLog_WithCooldownAndConflict(t *testing.T) {
	ctx := Context{
		Param:              "net.ipv4.ip_forward",
		Process:            "malicious-app",
		Actual:             "1",
		Expected:           "0",
		Category:           "security",
		Criticality:        "high",
		IsTrustedProcess:   false,
		IsAllowedProcess:   false,
	}

	decision := Decision{
		Action:  "remediate",
		Score:   10,
		Reasons: []string{"critical"},
	}

	log := BuildTraceLog(ctx, decision, true, true, "alert")

	if !log.CooldownApplied {
		t.Error("cooldown applied not set")
	}
	if !log.ConflictDetected {
		t.Error("conflict detected not set")
	}
	if log.FinalAction != "alert" {
		t.Error("final action should reflect cooldown downgrade")
	}
}

func TestTraceLog_JSONMarshaling_Fields(t *testing.T) {
	log := TraceLog{
		Param:            "test.param",
		Category:         "security",
		DecisionAction:   "remediate",
		CooldownApplied:  true,
		ConflictDetected: false,
		FinalAction:      "alert",
	}

	jsonData, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Verify all expected fields are in JSON
	jsonStr := string(jsonData)
	expectedFields := []string{
		"param",
		"category",
		"decision_action",
		"cooldown_applied",
		"conflict_detected",
		"final_action",
	}

	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, `"`+field+`"`) {
			t.Errorf("expected field '%s' not found in JSON", field)
		}
	}
}

func TestTraceLog_JSONMarshaling_SnakeCaseConversion(t *testing.T) {
	log := TraceLog{
		DecisionAction:   "remediate",
		CooldownApplied:  true,
		ConflictDetected: true,
		FinalAction:      "alert",
	}

	jsonData, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	jsonStr := string(jsonData)

	// Verify snake_case conversion in JSON
	if strings.Contains(jsonStr, "DecisionAction") {
		t.Error("field names should be snake_case in JSON, not PascalCase")
	}
	if !strings.Contains(jsonStr, "decision_action") {
		t.Error("expected snake_case field 'decision_action'")
	}
	if !strings.Contains(jsonStr, "cooldown_applied") {
		t.Error("expected snake_case field 'cooldown_applied'")
	}
}

func TestTraceLog_EmptyReasons(t *testing.T) {
	log := TraceLog{
		Param:   "test.param",
		Reasons: []string{}, // Empty slice
	}

	output := captureOutput(func() {
		EmitTrace(log)
	})

	var unmarshaled TraceLog
	err := json.Unmarshal([]byte(strings.TrimSpace(output)), &unmarshaled)
	if err != nil {
		t.Errorf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Reasons == nil || len(unmarshaled.Reasons) != 0 {
		t.Error("empty reasons should be preserved as empty slice")
	}
}

func TestEmitTrace_MultipleWrites(t *testing.T) {
	log1 := TraceLog{Param: "param1", FinalAction: "allow"}
	log2 := TraceLog{Param: "param2", FinalAction: "remediate"}

	output := captureOutput(func() {
		EmitTrace(log1)
		EmitTrace(log2)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines of output, got %d", len(lines))
	}

	var log1Unmarshaled, log2Unmarshaled TraceLog
	err1 := json.Unmarshal([]byte(lines[0]), &log1Unmarshaled)
	err2 := json.Unmarshal([]byte(lines[1]), &log2Unmarshaled)

	if err1 != nil || err2 != nil {
		t.Error("failed to unmarshal multiple traces")
	}

	if log1Unmarshaled.Param != "param1" || log2Unmarshaled.Param != "param2" {
		t.Error("parameters not preserved in multiple traces")
	}
}

func TestTraceLog_FullScenario(t *testing.T) {
	// Simulate a complete drift detection scenario
	ctx := Context{
		Param:              "net.ipv4.ip_forward",
		Process:            "malware",
		Actual:             "1",
		Expected:           "0",
		Category:           "security",
		Criticality:        "high",
		IsTrustedProcess:   false,
		IsAllowedProcess:   false,
	}

	decision := Decision{
		Action:  "remediate",
		Score:   10,
		Reasons: []string{"untrusted process modifying high-critical security parameter"},
	}

	log := BuildTraceLog(ctx, decision, true, true, "alert")

	output := captureOutput(func() {
		EmitTrace(log)
	})

	var unmarshaled TraceLog
	err := json.Unmarshal([]byte(strings.TrimSpace(output)), &unmarshaled)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify complete scenario
	if unmarshaled.Process != "malware" {
		t.Error("process not preserved")
	}
	if unmarshaled.DecisionAction != "remediate" {
		t.Error("decision action not preserved")
	}
	if unmarshaled.Score != 10 {
		t.Error("score not preserved")
	}
	if unmarshaled.FinalAction != "alert" {
		t.Error("final action not preserved")
	}
	if !unmarshaled.CooldownApplied || !unmarshaled.ConflictDetected {
		t.Error("cooldown and conflict flags not preserved")
	}
}

func TestEmitTrace_NoOutput_OnError(t *testing.T) {
	// Note: Current implementation always appends timestamp, so this test
	// documents the non-blocking behavior. In production, errors go to stderr.
	log := TraceLog{
		Param: "test",
		// This should still emit successfully
	}

	output := captureOutput(func() {
		EmitTrace(log)
	})

	if strings.TrimSpace(output) == "" {
		t.Error("EmitTrace should produce output even with minimal data")
	}
}

func BenchmarkEmitTrace(b *testing.B) {
	log := TraceLog{
		Param:          "vm.swappiness",
		Process:        "kube-proxy",
		Actual:         "60",
		Expected:       "10",
		Category:       "performance",
		Criticality:    "medium",
		Trusted:        true,
		DecisionAction: "alert",
		Score:          4,
		Reasons:        []string{"reason1", "reason2"},
		FinalAction:    "alert",
	}

	// Redirect stdout to /dev/null for benchmark
	devNull, _ := os.Open(os.DevNull)
	oldStdout := os.Stdout
	os.Stdout = devNull
	defer func() {
		os.Stdout = oldStdout
		devNull.Close()
	}()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		EmitTrace(log)
	}
}

func BenchmarkBuildTraceLog(b *testing.B) {
	ctx := Context{
		Param:              "vm.swappiness",
		Process:            "kube-proxy",
		Actual:             "60",
		Expected:           "10",
		Category:           "performance",
		Criticality:        "medium",
		IsTrustedProcess:   true,
		IsAllowedProcess:   false,
	}

	decision := Decision{
		Action:  "alert",
		Score:   5,
		Reasons: []string{"reason1", "reason2"},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		BuildTraceLog(ctx, decision, false, false, "alert")
	}
}
