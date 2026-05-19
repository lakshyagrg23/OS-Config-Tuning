package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const DefaultRegistrationTimeout = 5 * time.Second

// RegistrationRequest announces a node to the control plane.
// The agent keeps a persistent node ID locally and includes it in the payload.
type RegistrationRequest struct {
	NodeID       string `json:"nodeId"`
	Hostname     string `json:"hostname"`
	IPAddress    string `json:"ipAddress"`
	AgentVersion string `json:"agentVersion"`
}

// RegistrationResponse is the minimal acknowledgment returned by the control plane.
type RegistrationResponse struct {
	NodeID string `json:"nodeId"`
}

// RegistrationConfig contains the fields required to announce an agent node.
type RegistrationConfig struct {
	ControlPlaneURL string
	NodeID          string
	Hostname        string
	AgentVersion    string
}

// RegisterNode posts the current agent identity to the control plane's /register endpoint.
// It returns the node ID acknowledged by the backend, if any.
func RegisterNode(ctx context.Context, config RegistrationConfig) (string, error) {
	return RegisterNodeWithDependencies(ctx, config, ResolveLocalIPAddress, nil)
}

// RegisterNodeWithDependencies is the testable variant of RegisterNode.
func RegisterNodeWithDependencies(
	ctx context.Context,
	config RegistrationConfig,
	ipResolver func() (string, error),
	httpClient *http.Client,
) (string, error) {
	if strings.TrimSpace(config.ControlPlaneURL) == "" {
		return "", fmt.Errorf("control plane URL is empty")
	}

	if ipResolver == nil {
		ipResolver = ResolveLocalIPAddress
	}

	ipAddress, err := ipResolver()
	if err != nil {
		return "", err
	}

	payload := RegistrationRequest{
		NodeID:       config.NodeID,
		Hostname:     config.Hostname,
		IPAddress:    ipAddress,
		AgentVersion: config.AgentVersion,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal registration payload: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(config.ControlPlaneURL, "/")+"/register", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create registration request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: DefaultRegistrationTimeout}
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("send registration request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("registration failed: %s", response.Status)
	}

	var ack RegistrationResponse
	if err := json.NewDecoder(response.Body).Decode(&ack); err != nil {
		return "", fmt.Errorf("decode registration response: %w", err)
	}

	if ack.NodeID == "" {
		ack.NodeID = config.NodeID
	}

	return ack.NodeID, nil
}

// ResolveLocalIPAddress finds the first usable IPv4 address on an active interface.
func ResolveLocalIPAddress() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("list network interfaces: %w", err)
	}

	var fallback string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil || ip == nil {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue
			}
			if ip.IsLoopback() {
				if fallback == "" {
					fallback = ip.String()
				}
				continue
			}
			return ip.String(), nil
		}
	}

	if fallback != "" {
		return fallback, nil
	}

	return "", fmt.Errorf("no IPv4 address found")
}
