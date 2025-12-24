/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package hetzner provides a client for interacting with Hetzner Cloud API.
package hetzner

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"

	"github.com/autokubeio/autokube/internal/reliability"
)

// ClientInterface defines the interface for interacting with Hetzner Cloud
type ClientInterface interface {
	ListServers(ctx context.Context, nodePoolName, namespace string) ([]Server, error)
	CreateServer(ctx context.Context, config ServerConfig) (*Server, error)
	DeleteServer(ctx context.Context, serverID int64) error
	GetServer(ctx context.Context, serverID int64) (*Server, error)
	GetOrCreateFirewall(ctx context.Context, name string, rules []hcloud.FirewallRule) (*hcloud.Firewall, error)
	DeleteFirewall(ctx context.Context, firewallID int64) error
}

// ServerCreateError is a custom error type for server creation failures
type ServerCreateError struct {
	Message string
}

func (e *ServerCreateError) Error() string {
	return fmt.Sprintf("server creation failed: %s", e.Message)
}

// Client wraps the Hetzner Cloud API client
type Client struct {
	client         *hcloud.Client
	retryConfig    reliability.RetryConfig
	circuitBreaker *reliability.CircuitBreaker
}

// ClientOption is a function that configures a Client
type ClientOption func(*Client)

// WithRetryConfig sets a custom retry configuration
func WithRetryConfig(config reliability.RetryConfig) ClientOption {
	return func(c *Client) {
		c.retryConfig = config
	}
}

// WithCircuitBreaker sets a circuit breaker
func WithCircuitBreaker(cb *reliability.CircuitBreaker) ClientOption {
	return func(c *Client) {
		c.circuitBreaker = cb
	}
}

// Server represents a Hetzner Cloud server
type Server struct {
	ID        int64
	Name      string
	Status    string
	IPv4      string
	IPv6      string
	PrivateIP string
}

// NewClient creates a new Hetzner Cloud client
func NewClient(token string, opts ...ClientOption) *Client {
	c := &Client{
		client:      hcloud.NewClient(hcloud.WithToken(token)),
		retryConfig: reliability.DefaultRetryConfig(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// ServerConfig contains the configuration for creating a server
type ServerConfig struct {
	Name       string
	ServerType string
	Image      string
	Location   string
	SSHKeys    []string
	Labels     map[string]string
	UserData   string
	Network    string
	Firewalls  []int64 // Firewall IDs to attach to the server
}

// ListServers lists all servers for a given node pool
func (c *Client) ListServers(ctx context.Context, nodePoolName, namespace string) ([]Server, error) {
	opts := hcloud.ServerListOpts{
		ListOpts: hcloud.ListOpts{
			LabelSelector: fmt.Sprintf("nodepool=%s,namespace=%s", nodePoolName, namespace),
		},
	}

	servers, err := c.client.Server.AllWithOpts(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}

	result := make([]Server, len(servers))
	for i, s := range servers {
		result[i] = Server{
			ID:     s.ID,
			Name:   s.Name,
			Status: string(s.Status),
			IPv4:   s.PublicNet.IPv4.IP.String(),
		}
		if s.PublicNet.IPv6.Network != nil {
			result[i].IPv6 = s.PublicNet.IPv6.Network.String()
		}
	}

	return result, nil
}

// CreateServer creates a new server in Hetzner Cloud
//
//nolint:funlen,gocyclo // Server creation involves multiple API calls and configuration steps
func (c *Client) CreateServer(ctx context.Context, config ServerConfig) (*Server, error) {
	// Get server type
	serverType, _, err := c.client.ServerType.GetByName(ctx, config.ServerType)
	if err != nil {
		return nil, fmt.Errorf("failed to get server type: %w", err)
	}
	if serverType == nil {
		return nil, fmt.Errorf("server type %s not found", config.ServerType)
	}

	// Get image
	image, _, err := c.client.Image.GetByNameAndArchitecture(ctx, config.Image, hcloud.ArchitectureX86)
	if err != nil {
		return nil, fmt.Errorf("failed to get image: %w", err)
	}
	if image == nil {
		return nil, fmt.Errorf("image %s not found", config.Image)
	}

	// Get location
	location, _, err := c.client.Location.GetByName(ctx, config.Location)
	if err != nil {
		return nil, fmt.Errorf("failed to get location: %w", err)
	}
	if location == nil {
		return nil, fmt.Errorf("location %s not found", config.Location)
	}

	// Get SSH keys
	var sshKeys []*hcloud.SSHKey
	for _, keyName := range config.SSHKeys {
		key, _, err := c.client.SSHKey.GetByName(ctx, keyName)
		if err != nil {
			return nil, fmt.Errorf("failed to get SSH key %s: %w", keyName, err)
		}
		if key == nil {
			return nil, fmt.Errorf("SSH key not found: %s", keyName)
		}
		sshKeys = append(sshKeys, key)
	}

	// Create server
	createOpts := hcloud.ServerCreateOpts{
		Name:       config.Name,
		ServerType: serverType,
		Image:      image,
		Location:   location,
		SSHKeys:    sshKeys,
		Labels:     config.Labels,
		UserData:   config.UserData,
	}

	// Get network if specified (will attach after server creation)
	var network *hcloud.Network
	if config.Network != "" {
		var err error

		// Check if it's a numeric ID
		if networkID, parseErr := strconv.ParseInt(config.Network, 10, 64); parseErr == nil {
			// It's an ID
			network, _, err = c.client.Network.GetByID(ctx, networkID)
			if err != nil {
				return nil, fmt.Errorf("failed to get network by ID: %w", err)
			}
		} else {
			// It's a name
			network, _, err = c.client.Network.GetByName(ctx, config.Network)
			if err != nil {
				return nil, fmt.Errorf("failed to get network by name: %w", err)
			}
		}

		if network == nil {
			return nil, fmt.Errorf("network %s not found", config.Network)
		}
	}

	// Attach firewalls if specified
	if len(config.Firewalls) > 0 {
		var firewalls []*hcloud.ServerCreateFirewall
		for _, fwID := range config.Firewalls {
			firewalls = append(firewalls, &hcloud.ServerCreateFirewall{
				Firewall: hcloud.Firewall{ID: fwID},
			})
		}
		createOpts.Firewalls = firewalls
	}

	result, _, err := c.client.Server.Create(ctx, createOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	server := &Server{
		ID:     result.Server.ID,
		Name:   result.Server.Name,
		Status: string(result.Server.Status),
	}

	if result.Server.PublicNet.IPv4.IP != nil {
		server.IPv4 = result.Server.PublicNet.IPv4.IP.String()
	}

	// Attach to network after server creation if network was specified
	if network != nil {
		attachOpts := hcloud.ServerAttachToNetworkOpts{
			Network: network,
		}
		action, _, err := c.client.Server.AttachToNetwork(ctx, result.Server, attachOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to attach server to network: %w", err)
		}

		// Wait for the action to complete
		_, errCh := c.client.Action.WatchProgress(ctx, action)
		if err := <-errCh; err != nil {
			return nil, fmt.Errorf("failed to wait for network attachment: %w", err)
		}

		// Refresh server data to get the assigned private IP
		var updatedServer *hcloud.Server

		err = c.executeWithRetry(ctx, func() error {
			var err error
			updatedServer, _, err = c.client.Server.GetByID(ctx, result.Server.ID)
			if err != nil {
				return fmt.Errorf("failed to get server: %w", err)
			}

			if updatedServer == nil {
				return fmt.Errorf("server not found")
			}
			return nil
		})

		if err != nil {
			return nil, err
		}

		if len(updatedServer.PrivateNet) > 0 {
			server.PrivateIP = updatedServer.PrivateNet[0].IP.String()
		}
	}

	return server, nil
}

// DeleteServer deletes a server from Hetzner Cloud
func (c *Client) DeleteServer(ctx context.Context, serverID int64) error {
	server := &hcloud.Server{ID: serverID}

	_, _, err := c.client.Server.DeleteWithResult(ctx, server)
	if err != nil {
		return fmt.Errorf("failed to delete server: %w", err)
	}

	return nil
}

// GetServer gets a server by ID
func (c *Client) GetServer(ctx context.Context, serverID int64) (*Server, error) {
	server, _, err := c.client.Server.GetByID(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to get server: %w", err)
	}

	if server == nil {
		return nil, fmt.Errorf("server not found")
	}

	result := &Server{
		ID:     server.ID,
		Name:   server.Name,
		Status: string(server.Status),
	}

	if server.PublicNet.IPv4.IP != nil {
		result.IPv4 = server.PublicNet.IPv4.IP.String()
	}
	if server.PublicNet.IPv6.Network != nil {
		result.IPv6 = server.PublicNet.IPv6.Network.String()
	}

	return result, nil
}

// GetOrCreateFirewall creates or retrieves a Hetzner Cloud Firewall
func (c *Client) GetOrCreateFirewall(
	ctx context.Context,
	name string,
	rules []hcloud.FirewallRule,
) (*hcloud.Firewall, error) {
	// Try to find existing firewall
	firewall, _, err := c.client.Firewall.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get firewall: %w", err)
	}

	if firewall != nil {
		// Update rules if they differ
		_, _, err := c.client.Firewall.SetRules(ctx, firewall, hcloud.FirewallSetRulesOpts{
			Rules: rules,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update firewall rules: %w", err)
		}
		return firewall, nil
	}

	// Create new firewall
	result, _, err := c.client.Firewall.Create(ctx, hcloud.FirewallCreateOpts{
		Name:  name,
		Rules: rules,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create firewall: %w", err)
	}

	return result.Firewall, nil
}

// DeleteFirewall deletes a Hetzner Cloud Firewall
func (c *Client) DeleteFirewall(ctx context.Context, firewallID int64) error {
	firewall := &hcloud.Firewall{ID: firewallID}

	_, err := c.client.Firewall.Delete(ctx, firewall)
	if err != nil {
		return fmt.Errorf("failed to delete firewall: %w", err)
	}

	return nil
}

// executeWithRetry executes an operation with retry logic
func (c *Client) executeWithRetry(ctx context.Context, operation func() error) error {
	if c.circuitBreaker != nil {
		return c.circuitBreaker.Execute(func() error {
			return reliability.RetryOperation(ctx, c.retryConfig, operation)
		})
	}
	return reliability.RetryOperation(ctx, c.retryConfig, operation)
}
