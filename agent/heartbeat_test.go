package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"drift-agent/agent/controlplane"
)

func TestStartHeartbeatLoop_SendsPeriodicHeartbeats(t *testing.T) {
	t.Helper()

	count := 0
	var lastBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/heartbeat" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		lastBody = string(b)
		count++
		w.WriteHeader(200)
	}))
	defer srv.Close()

	nodeID := "node-1"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cpClient := controlplane.NewControlPlaneClient(srv.URL, srv.Client())
	controlplane.StartHeartbeatLoop(ctx, cpClient, nodeID, 100*time.Millisecond)

	// allow a few heartbeats
	time.Sleep(350 * time.Millisecond)
	cancel()

	if count < 2 {
		t.Fatalf("expected >=2 heartbeats, got %d", count)
	}
	if !strings.Contains(lastBody, nodeID) {
		t.Fatalf("unexpected heartbeat body: %s", lastBody)
	}
}
