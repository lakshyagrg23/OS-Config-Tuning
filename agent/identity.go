package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

const defaultNodeIDPath = "/var/lib/drift-agent/node-id"

// AgentConfig identifies a node to the control plane.
// The node ID is persisted locally so the same machine keeps the same identity
// across restarts and reboots.
type AgentConfig struct {
	NodeID          string
	Hostname        string
	AgentVersion    string
	ControlPlaneURL string
}

// LoadAgentConfig resolves the current node identity and agent metadata.
// The node ID is loaded from disk if present, or generated and persisted on
// first startup.
func LoadAgentConfig(controlPlaneURL string) (AgentConfig, error) {
	return LoadAgentConfigAt(controlPlaneURL, resolveNodeIDPath())
}

// LoadAgentConfigAt is the testable variant of LoadAgentConfig.
func LoadAgentConfigAt(controlPlaneURL, nodeIDPath string) (AgentConfig, error) {
	nodeID, err := loadOrCreateNodeID(nodeIDPath)
	if err != nil {
		return AgentConfig{}, err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return AgentConfig{}, fmt.Errorf("get hostname: %w", err)
	}

	if controlPlaneURL == "" {
		controlPlaneURL = os.Getenv("DRIFT_CONTROL_PLANE_URL")
	}

	return AgentConfig{
		NodeID:          nodeID,
		Hostname:        hostname,
		AgentVersion:    resolveAgentVersion(),
		ControlPlaneURL: controlPlaneURL,
	}, nil
}

func resolveNodeIDPath() string {
	if path := strings.TrimSpace(os.Getenv("DRIFT_AGENT_NODE_ID_PATH")); path != "" {
		return path
	}

	if stateDir := strings.TrimSpace(os.Getenv("DRIFT_AGENT_STATE_DIR")); stateDir != "" {
		return filepath.Join(stateDir, "node-id")
	}

	return defaultNodeIDPath
}

func loadOrCreateNodeID(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		nodeID := strings.TrimSpace(string(data))
		if nodeID != "" {
			return nodeID, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read node id: %w", err)
	}

	nodeID, err := generateNodeID()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create node id directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(nodeID+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("persist node id: %w", err)
	}

	return nodeID, nil
}

func generateNodeID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate node id: %w", err)
	}

	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(raw[:])
	return fmt.Sprintf(
		"%s-%s-%s-%s-%s",
		encoded[0:8],
		encoded[8:12],
		encoded[12:16],
		encoded[16:20],
		encoded[20:32],
	), nil
}

func resolveAgentVersion() string {
	if version := strings.TrimSpace(os.Getenv("DRIFT_AGENT_VERSION")); version != "" {
		return version
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		if version := strings.TrimSpace(info.Main.Version); version != "" && version != "(devel)" {
			return version
		}
	}

	return "dev"
}
