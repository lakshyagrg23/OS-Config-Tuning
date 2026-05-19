package controlplane

import (
	"context"
	"time"
)

// HeartbeatRequest is sent periodically to the control plane to indicate liveness.
type HeartbeatRequest struct {
	NodeID string `json:"nodeId"`
	Status string `json:"status"`
}

// StartHeartbeatLoop launches a background goroutine that sends heartbeats
// to the control plane using the provided client every interval. It returns
// immediately; the loop stops when ctx is cancelled. It is best-effort and
// never blocks the agent.
func StartHeartbeatLoop(ctx context.Context, client *ControlPlaneClient, nodeID string, interval time.Duration) {
	if client == nil || client.BaseURL == "" {
		return
	}

	if interval <= 0 {
		interval = 10 * time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		send := func() {
			hb := HeartbeatRequest{NodeID: nodeID, Status: "healthy"}
			if err := client.SendHeartbeat(ctx, hb); err != nil {
				LogControlPlaneWarning("failed to send heartbeat: %v", err)
			}
		}

		send()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				send()
			}
		}
	}()
}
