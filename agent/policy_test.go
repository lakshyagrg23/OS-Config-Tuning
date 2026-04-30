package main

import (
"testing"
"time"
)

func TestBuildContext(t *testing.T) {
// Setup
policy := &Policy{
TrustedProcesses: []string{"systemd", "kubelet"},
Sysctl: map[string]SysctlPolicy{
"net.ipv4.ip_forward": {
Expected:       "0",
Category:       "security",
Criticality:    "high",
Remediation:    "auto",
Cooldown:       30 * time.Second,
AllowProcesses: []string{"kube-proxy"},
},
},
}

event := WorkEvent{
Pid:      1234,
Process:  "kube-proxy",
FilePath: "/proc/sys/net/ipv4/ip_forward",
Access:   "WRITE",
}

policyEntry := policy.Sysctl["net.ipv4.ip_forward"]

// Call BuildContext
ctx := BuildContext(event, "net.ipv4.ip_forward", policyEntry, "1", policy)

// Verify
if ctx.Param != "net.ipv4.ip_forward" {
t.Errorf("Param: expected 'net.ipv4.ip_forward', got '%s'", ctx.Param)
}
if ctx.Expected != "0" {
t.Errorf("Expected: expected '0', got '%s'", ctx.Expected)
}
if ctx.Actual != "1" {
t.Errorf("Actual: expected '1', got '%s'", ctx.Actual)
}
if ctx.Category != "security" {
t.Errorf("Category: expected 'security', got '%s'", ctx.Category)
}
if ctx.Criticality != "high" {
t.Errorf("Criticality: expected 'high', got '%s'", ctx.Criticality)
}
if ctx.Process != "kube-proxy" {
t.Errorf("Process: expected 'kube-proxy', got '%s'", ctx.Process)
}
if !ctx.IsAllowedProcess {
t.Errorf("IsAllowedProcess: expected true, got false")
}
if ctx.IsTrustedProcess {
t.Errorf("IsTrustedProcess: expected false (kube-proxy not in trusted list), got true")
}
}

func TestIsTrusted(t *testing.T) {
policy := &Policy{
TrustedProcesses: []string{"systemd", "kubelet", "dockerd"},
}

tests := []struct {
process  string
expected bool
}{
{"systemd", true},
{"kubelet", true},
{"dockerd", true},
{"kube-proxy", false},
{"unknown", false},
}

for _, test := range tests {
result := isTrusted(test.process, policy)
if result != test.expected {
t.Errorf("isTrusted(%s): expected %v, got %v", test.process, test.expected, result)
}
}
}

func TestIsAllowed(t *testing.T) {
policyEntry := SysctlPolicy{
AllowProcesses: []string{"kube-proxy", "systemd"},
}

tests := []struct {
process  string
expected bool
}{
{"kube-proxy", true},
{"systemd", true},
{"kubelet", false},
{"unknown", false},
}

for _, test := range tests {
result := isAllowed(test.process, policyEntry)
if result != test.expected {
t.Errorf("isAllowed(%s): expected %v, got %v", test.process, test.expected, result)
}
}
}
