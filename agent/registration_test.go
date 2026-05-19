package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"drift-agent/agent/controlplane"
)

func TestRegisterNodeWithDependencies_SendsExpectedPayload(t *testing.T) {
	t.Helper()

	var got controlplane.RegistrationRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/register" {
			t.Fatalf("expected /register path, got %s", r.URL.Path)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("request body is not valid JSON: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeId":"abc123"}`))
	}))
	defer server.Close()

	config := controlplane.RegistrationConfig{
		NodeID:          "node-123",
		Hostname:        "vm-node-01",
		AgentVersion:    "1.0.0",
		ControlPlaneURL: server.URL,
	}

	ackNodeID, err := controlplane.RegisterNodeWithDependencies(
		context.Background(),
		config,
		func() (string, error) { return "192.168.56.101", nil },
		server.Client(),
	)
	if err != nil {
		t.Fatalf("RegisterNodeWithDependencies returned error: %v", err)
	}

	if got.NodeID != config.NodeID {
		t.Fatalf("expected nodeId %q, got %q", config.NodeID, got.NodeID)
	}
	if got.Hostname != config.Hostname {
		t.Fatalf("expected hostname %q, got %q", config.Hostname, got.Hostname)
	}
	if got.AgentVersion != config.AgentVersion {
		t.Fatalf("expected agentVersion %q, got %q", config.AgentVersion, got.AgentVersion)
	}
	if got.IPAddress != "192.168.56.101" {
		t.Fatalf("expected ipAddress %q, got %q", "192.168.56.101", got.IPAddress)
	}
	if ackNodeID != "abc123" {
		t.Fatalf("expected ack nodeId %q, got %q", "abc123", ackNodeID)
	}
}

func TestRegisterNodeWithDependencies_UsesConfigNodeIDWhenAckMissing(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	config := controlplane.RegistrationConfig{
		NodeID:          "node-123",
		Hostname:        "vm-node-01",
		AgentVersion:    "1.0.0",
		ControlPlaneURL: server.URL,
	}

	ackNodeID, err := controlplane.RegisterNodeWithDependencies(
		context.Background(),
		config,
		func() (string, error) { return "192.168.56.101", nil },
		server.Client(),
	)
	if err != nil {
		t.Fatalf("RegisterNodeWithDependencies returned error: %v", err)
	}
	if strings.TrimSpace(ackNodeID) != config.NodeID {
		t.Fatalf("expected fallback node id %q, got %q", config.NodeID, ackNodeID)
	}
}
