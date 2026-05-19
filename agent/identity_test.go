package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateNodeID_Format(t *testing.T) {
	nodeID, err := generateNodeID()
	if err != nil {
		t.Fatalf("generateNodeID returned error: %v", err)
	}

	if len(nodeID) != 36 {
		t.Fatalf("expected UUID-like node ID length 36, got %d (%q)", len(nodeID), nodeID)
	}
	if nodeID[14] != '4' {
		t.Fatalf("expected version 4 UUID format, got %q", nodeID)
	}
	if nodeID[8] != '-' || nodeID[13] != '-' || nodeID[18] != '-' || nodeID[23] != '-' {
		t.Fatalf("expected UUID hyphen layout, got %q", nodeID)
	}
}

func TestLoadOrCreateNodeID_PersistsAcrossCalls(t *testing.T) {
	tempDir := t.TempDir()
	nodeIDPath := filepath.Join(tempDir, "node-id")

	first, err := loadOrCreateNodeID(nodeIDPath)
	if err != nil {
		t.Fatalf("first loadOrCreateNodeID returned error: %v", err)
	}

	data, err := os.ReadFile(nodeIDPath)
	if err != nil {
		t.Fatalf("failed to read persisted node id: %v", err)
	}
	if strings.TrimSpace(string(data)) != first {
		t.Fatalf("persisted node id mismatch: got %q want %q", strings.TrimSpace(string(data)), first)
	}

	second, err := loadOrCreateNodeID(nodeIDPath)
	if err != nil {
		t.Fatalf("second loadOrCreateNodeID returned error: %v", err)
	}
	if second != first {
		t.Fatalf("expected node id to persist, got first=%q second=%q", first, second)
	}
}

func TestLoadAgentConfigAt_UsesProvidedIdentitySources(t *testing.T) {
	tempDir := t.TempDir()
	nodeIDPath := filepath.Join(tempDir, "node-id")

	t.Setenv("DRIFT_AGENT_VERSION", "test-version")
	t.Setenv("DRIFT_CONTROL_PLANE_URL", "https://control.example")

	config, err := LoadAgentConfigAt("", nodeIDPath)
	if err != nil {
		t.Fatalf("LoadAgentConfigAt returned error: %v", err)
	}

	if config.NodeID == "" {
		t.Fatal("expected NodeID to be populated")
	}
	if config.Hostname == "" {
		t.Fatal("expected Hostname to be populated")
	}
	if config.AgentVersion != "test-version" {
		t.Fatalf("expected AgentVersion from env, got %q", config.AgentVersion)
	}
	if config.ControlPlaneURL != "https://control.example" {
		t.Fatalf("expected ControlPlaneURL from env, got %q", config.ControlPlaneURL)
	}
}
