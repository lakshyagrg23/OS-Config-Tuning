package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"drift-agent/agent/controlplane"
)

func TestStartUploader_SendsEventWithNodeID(t *testing.T) {
	t.Helper()

	received := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/events" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		received <- b
		w.WriteHeader(200)
	}))
	defer srv.Close()

	nodeID := "node-1"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cpClient := controlplane.NewControlPlaneClient(srv.URL, srv.Client())
	up := controlplane.StartUploader(ctx, cpClient, nodeID, 10)
	if up == nil {
		t.Fatal("uploader should not be nil")
	}

	trace := TraceLog{Param: "kernel.randomize_va_space", Process: "sysctl", DecisionAction: "remediate", FinalAction: "alert"}
	up.Enqueue(trace)

	select {
	case b := <-received:
		var pl map[string]interface{}
		if err := json.Unmarshal(b, &pl); err != nil {
			t.Fatalf("failed to unmarshal payload: %v", err)
		}
		if pl["nodeId"] != nodeID {
			t.Fatalf("expected nodeId %s, got %v", nodeID, pl["nodeId"])
		}
		if pl["param"] != "kernel.randomize_va_space" {
			t.Fatalf("unexpected param: %v", pl["param"])
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for event upload")
	}
}
