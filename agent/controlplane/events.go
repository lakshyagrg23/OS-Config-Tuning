package controlplane

import (
	"context"
	"encoding/json"
)

// Uploader sends events to the control plane asynchronously.
type Uploader struct {
	nodeID string
	ch     chan any
	client *ControlPlaneClient
}

// StartUploader creates an uploader with a buffered channel of size 'size'
// and starts a background goroutine that delivers events to the control plane.
// Returns nil if no control plane URL is configured.
func StartUploader(ctx context.Context, client *ControlPlaneClient, nodeID string, size int) *Uploader {
	if client == nil || client.BaseURL == "" {
		return nil
	}
	if size <= 0 {
		size = 100
	}

	u := &Uploader{
		nodeID: nodeID,
		ch:     make(chan any, size),
		client: client,
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case payload := <-u.ch:
				enriched, err := enrichPayloadWithNodeID(u.nodeID, payload)
				if err != nil {
					LogControlPlaneWarning("uploader: marshal failed: %v", err)
					continue
				}

				if err := u.client.UploadEvent(ctx, enriched); err != nil {
					LogControlPlaneWarning("failed to upload event: %v", err)
				}
			}
		}
	}()

	return u
}

func enrichPayloadWithNodeID(nodeID string, payload any) (map[string]any, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	event["nodeId"] = nodeID
	return event, nil
}

// Enqueue attempts to submit an event to the upload queue without blocking.
// If the queue is full the event is dropped and a warning is logged.
func (u *Uploader) Enqueue(payload any) {
	select {
	case u.ch <- payload:
	default:
		LogControlPlaneWarning("uploader: queue full, dropping event")
	}
}

var defaultUploader *Uploader

// SetDefaultUploader registers the global uploader used by the agent.
func SetDefaultUploader(u *Uploader) { defaultUploader = u }

// EnqueueToDefault submits an event to the package-level uploader when configured.
func EnqueueToDefault(payload any) {
	if defaultUploader != nil {
		defaultUploader.Enqueue(payload)
	}
}
