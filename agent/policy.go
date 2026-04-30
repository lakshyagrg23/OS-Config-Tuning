package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SysctlPolicy defines the baseline value and remediation strategy for a
// single sysctl parameter.
type SysctlPolicy struct {
	Expected       string        `yaml:"expected"`
	Category       string        `yaml:"category"`
	Criticality    string        `yaml:"criticality"`
	Remediation    string        `yaml:"remediation"`
	Cooldown       time.Duration `yaml:"cooldown"`
	AllowProcesses []string      `yaml:"allow_processes"`
	// Backward compatibility: support old "value" field name
	Value string `yaml:"value"`
}

// Policy holds the baseline configuration loaded from baseline.yaml.
type Policy struct {
	Global           GlobalConfig            `yaml:"global"`
	TrustedProcesses []string                `yaml:"trusted_processes"`
	Sysctl           map[string]SysctlPolicy `yaml:"sysctl"`
}

// GlobalConfig holds global settings from the policy file.
type GlobalConfig struct {
	DefaultCooldown    time.Duration `yaml:"default_cooldown"`
	RemediateThreshold int           `yaml:"remediate_threshold"`
	AlertThreshold     int           `yaml:"alert_threshold"`
}

// Context is a structured representation of a sysctl drift event and its policy,
// used by the decision engine. It contains all information needed to make a
// remediation decision without any side effects.
type Context struct {
	Param            string
	Expected         string
	Actual           string
	Category         string
	Criticality      string
	Process          string
	IsTrustedProcess bool
	IsAllowedProcess bool
}

// LoadPolicy parses the YAML file at path and returns a Policy.
func LoadPolicy(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy file: %w", err)
	}
	var p Policy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse policy file: %w", err)
	}
	if p.Sysctl == nil {
		p.Sysctl = make(map[string]SysctlPolicy)
	}

	// Post-process: handle backward compatibility and normalize Expected field
	for name, entry := range p.Sysctl {
		if entry.Expected == "" && entry.Value != "" {
			entry.Expected = entry.Value
			p.Sysctl[name] = entry
		}
	}

	return &p, nil
}

// isTrusted returns true if the given process name contains any of the trusted
// process names in the policy. Supports flexible matching for process paths and names.
// Returns false if policy is nil, process is empty, or no trusted processes are defined.
func isTrusted(process string, policy *Policy) bool {
	if policy == nil || process == "" || len(policy.TrustedProcesses) == 0 {
		return false
	}
	for _, trustedProc := range policy.TrustedProcesses {
		if strings.Contains(process, trustedProc) {
			return true
		}
	}
	return false
}

// isAllowed returns true if the given process name contains any of the allowed
// process names in the policy entry. Supports flexible matching for process paths and names.
// Returns false if process is empty or no allowed processes are defined.
func isAllowed(process string, policyEntry SysctlPolicy) bool {
	if process == "" || len(policyEntry.AllowProcesses) == 0 {
		return false
	}
	for _, allowedProc := range policyEntry.AllowProcesses {
		if strings.Contains(process, allowedProc) {
			return true
		}
	}
	return false
}

// BuildContext transforms a raw event and policy into a structured Context object.
// This function is pure—no side effects, no logging, no decision-making—just
// data transformation used by the decision engine.
func BuildContext(
	event WorkEvent,
	param string,
	policyEntry SysctlPolicy,
	actual string,
	policy *Policy,
) Context {
	return Context{
		Param:            param,
		Expected:         policyEntry.Expected,
		Actual:           actual,
		Category:         policyEntry.Category,
		Criticality:      policyEntry.Criticality,
		Process:          event.Process,
		IsTrustedProcess: isTrusted(event.Process, policy),
		IsAllowedProcess: isAllowed(event.Process, policyEntry),
	}
}
