package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/register", strings.NewReader(string(body)))
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/heartbeat", strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	r, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	r.Body.Close()
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/events", strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	r, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return fmt.Errorf("upload failed: %s", r.Status)
	}
	return nil
}

// LogControlPlaneWarning logs a non-fatal warning about control-plane failures.
// Control-plane errors are operationally important but must not affect the
// agent's primary monitoring and remediation responsibilities.
func LogControlPlaneWarning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "warning: control-plane: %s\n", msg)
}
