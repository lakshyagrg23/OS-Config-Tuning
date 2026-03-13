package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// SysctlPolicy defines the baseline value and remediation strategy for a
// single sysctl parameter.
type SysctlPolicy struct {
	Value       string `yaml:"value"`
	Remediation string `yaml:"remediation"`
}

// Policy holds the baseline configuration loaded from baseline.yaml.
type Policy struct {
	Sysctl map[string]SysctlPolicy `yaml:"sysctl"`
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
	return &p, nil
}
