package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ControlPlaneClient centralizes HTTP interactions with the control plane.
type ControlPlaneClient struct {
	BaseURL string
	Client  *http.Client
}

// NewControlPlaneClient builds a client with sensible defaults if httpClient is nil.
func NewControlPlaneClient(baseURL string, httpClient *http.Client) *ControlPlaneClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &ControlPlaneClient{BaseURL: strings.TrimRight(baseURL, "/"), Client: httpClient}
}

// RegisterNode POSTs /register with the provided payload and returns the ack.
func (c *ControlPlaneClient) RegisterNode(ctx context.Context, payload RegistrationRequest) (RegistrationResponse, error) {
	var resp RegistrationResponse
	if c == nil || c.BaseURL == "" {
		return resp, fmt.Errorf("control plane client not configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return resp, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/register", bytes.NewReader(body))
	if err != nil {
		return resp, err
	}
	req.Header.Set("Content-Type", "application/json")
	r, err := c.Client.Do(req)
	if err != nil {
		return resp, err
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return resp, fmt.Errorf("registration failed: %s", r.Status)
	}
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		return resp, err
	}
	if resp.NodeID == "" {
		resp.NodeID = payload.NodeID
	}
	return resp, nil
}

// SendHeartbeat posts a heartbeat payload.
func (c *ControlPlaneClient) SendHeartbeat(ctx context.Context, hb HeartbeatRequest) error {
	if c == nil || c.BaseURL == "" {
		return fmt.Errorf("control plane client not configured")
	}
	b, err := json.Marshal(hb)
	if err != nil {
		return err
	}
	
	// Optimization: Changed strings.NewReader(string(b)) to bytes.NewReader(b)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/heartbeat", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	
	r, err := c.Client.Do(req)
	// FIX: Check transport errors immediately before touching 'r'
	if err != nil {
		return fmt.Errorf("heartbeat transport failed: %w", err)
	}
	defer r.Body.Close() // Safe to defer closing now that r is guaranteed not to be nil

	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return fmt.Errorf("heartbeat failed: %s", r.Status)
	}
	return nil
}

// UploadEvent posts an event payload to /events.
func (c *ControlPlaneClient) UploadEvent(ctx context.Context, payload interface{}) error {
	if c == nil || c.BaseURL == "" {
		return fmt.Errorf("control plane client not configured")
	}
	
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/events", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	
	r, err := c.Client.Do(req)
	if err != nil {
		return err 
	}
	defer r.Body.Close() 

	if r.StatusCode < 200 || r.StatusCode >= 300 {
		bodyBytes, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			return fmt.Errorf("upload failed with status %s (could not read response body: %v)", r.Status, readErr)
		}
		
		fmt.Printf("--- UPLOAD FAILED DETAILS ---\nStatus: %s\nResponse: %s\n-----------------------------\n", r.Status, string(bodyBytes))
		
		return fmt.Errorf("upload failed: %s - %s", r.Status, string(bodyBytes))
	}
	
	return nil
}

// LogControlPlaneWarning logs a non-fatal warning about control-plane failures.
func LogControlPlaneWarning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "warning: control-plane: %s\n", msg)
}