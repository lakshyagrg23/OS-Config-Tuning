package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Policy holds the baseline configuration loaded from baseline.yaml.
type Policy struct {
	Sysctl map[string]string `yaml:"sysctl"`
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
		p.Sysctl = make(map[string]string)
	}
	return &p, nil
}
