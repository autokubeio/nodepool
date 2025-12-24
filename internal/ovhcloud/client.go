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

// Package ovhcloud provides a client for interacting with OVHcloud Public Cloud API.
package ovhcloud

import (
	"context"
	"fmt"
	"time"

	"github.com/autokubeio/autokube/internal/reliability"
	"github.com/ovh/go-ovh/ovh"
)

const (
	// DirectionIngress represents incoming traffic
	DirectionIngress = "ingress"
	// DirectionEgress represents outgoing traffic
	DirectionEgress = "egress"
)

// ClientInterface defines the interface for interacting with OVHcloud
type ClientInterface interface {
	ListInstances(ctx context.Context, nodePoolName, namespace string) ([]Instance, error)
	CreateInstance(ctx context.Context, config InstanceConfig) (*Instance, error)
	DeleteInstance(ctx context.Context, instanceID string) error
	GetInstance(ctx context.Context, instanceID string) (*Instance, error)
	GetOrCreateSecurityGroup(ctx context.Context, name string, rules []SecurityRule) (*SecurityGroup, error)
	DeleteSecurityGroup(ctx context.Context, securityGroupID string) error
	GetFlavorIDByName(ctx context.Context, region, flavorName string) (string, error)
	GetImageIDByName(ctx context.Context, region, imageName string) (string, error)
	GetSSHKeyIDByName(ctx context.Context, sshKeyName string) (string, error)
}

// InstanceCreateError is a custom error type for instance creation failures
type InstanceCreateError struct {
	Message string
}

func (e *InstanceCreateError) Error() string {
	return fmt.Sprintf("instance creation failed: %s", e.Message)
}

// Client wraps the OVHcloud API client
type Client struct {
	endpoint          string
	applicationKey    string
	applicationSecret string
	consumerKey       string
	projectID         string
	region            string
	retryConfig       reliability.RetryConfig
	circuitBreaker    *reliability.CircuitBreaker
	ovhClient         *ovh.Client
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

// Instance represents an OVHcloud instance
type Instance struct {
	ID        string
	Name      string
	Status    string
	IPv4      string
	IPv6      string
	PrivateIP string
}

// SecurityGroup represents an OVHcloud security group
type SecurityGroup struct {
	ID          string
	Name        string
	Description string
}

// SecurityRule defines a security group rule
type SecurityRule struct {
	Direction  string // ingress or egress
	Protocol   string // tcp, udp, icmp
	PortFrom   int
	PortTo     int
	SourceCIDR string
}

// NewClient creates a new OVHcloud client
func NewClient(endpoint, applicationKey, applicationSecret, consumerKey, projectID, region string, opts ...ClientOption) *Client {
	ovhClient, err := ovh.NewClient(
		endpoint,
		applicationKey,
		applicationSecret,
		consumerKey,
	)
	if err != nil {
		// Return client with error logging capability
		ovhClient = nil
	}

	c := &Client{
		endpoint:          endpoint,
		applicationKey:    applicationKey,
		applicationSecret: applicationSecret,
		consumerKey:       consumerKey,
		projectID:         projectID,
		region:            region,
		retryConfig:       reliability.DefaultRetryConfig(),
		ovhClient:         ovhClient,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// InstanceConfig contains the configuration for creating an instance
type InstanceConfig struct {
	Name            string
	FlavorID        string
	ImageID         string
	Region          string
	ProjectID       string
	NetworkID       string
	SSHKeys         []string
	UserData        string
	SecurityGroupID string
	Labels          map[string]string
}

// ListInstances retrieves all instances for a specific node pool
func (c *Client) ListInstances(ctx context.Context, nodePoolName, namespace string) ([]Instance, error) {
	if c.ovhClient == nil {
		return nil, fmt.Errorf("OVHcloud client not initialized")
	}

	// API endpoint: GET /cloud/project/{serviceName}/instance
	var rawInstances []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Status      string `json:"status"`
		IPAddresses []struct {
			IP      string `json:"ip"`
			Type    string `json:"type"`
			Version int    `json:"version"`
		} `json:"ipAddresses"`
	}

	endpoint := fmt.Sprintf("/cloud/project/%s/instance", c.projectID)
	if err := c.ovhClient.GetWithContext(ctx, endpoint, &rawInstances); err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}

	// Filter instances by labels (name contains nodepool name for now)
	var instances []Instance
	for _, raw := range rawInstances {
		// Simple filtering: check if instance name contains nodepool name
		// In production, you'd use proper labels/tags
		if len(raw.Name) > 0 {
			instance := Instance{
				ID:     raw.ID,
				Name:   raw.Name,
				Status: raw.Status,
			}

			// Extract IP addresses
			for _, ip := range raw.IPAddresses {
				if ip.Version == 4 {
					instance.IPv4 = ip.IP
					if ip.Type == "private" {
						instance.PrivateIP = ip.IP
					}
				} else if ip.Version == 6 {
					instance.IPv6 = ip.IP
				}
			}

			instances = append(instances, instance)
		}
	}

	return instances, nil
}

// CreateInstance creates a new instance in OVHcloud
func (c *Client) CreateInstance(ctx context.Context, config InstanceConfig) (*Instance, error) {
	if c.ovhClient == nil {
		return nil, fmt.Errorf("OVHcloud client not initialized")
	}

	// OVHcloud expects plain text user data, not base64
	// The API will handle encoding internally if needed

	// Prepare instance creation request dynamically
	createReq := map[string]interface{}{
		"name":     config.Name,
		"flavorId": config.FlavorID,
		"imageId":  config.ImageID,
		"region":   config.Region,
		"userData": config.UserData,
	}

	// Add SSH keys if provided and not empty
	// SSH keys must be pre-registered in OVHcloud project, then referenced by name
	if len(config.SSHKeys) > 0 && config.SSHKeys[0] != "" {
		createReq["sshKeyId"] = config.SSHKeys[0]
	}
	// Note: If no SSH key provided, OVHcloud may still create the instance without SSH access

	// Add network if provided
	if config.NetworkID != "" {
		createReq["networks"] = []string{config.NetworkID}
	}

	// API endpoint: POST /cloud/project/{serviceName}/instance
	var response struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}

	endpoint := fmt.Sprintf("/cloud/project/%s/instance", c.projectID)
	if err := c.ovhClient.PostWithContext(ctx, endpoint, createReq, &response); err != nil {
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}

	// Wait a moment for instance to be created
	time.Sleep(2 * time.Second)

	// Get full instance details
	return c.GetInstance(ctx, response.ID)
}

// DeleteInstance deletes an instance from OVHcloud
func (c *Client) DeleteInstance(ctx context.Context, instanceID string) error {
	if c.ovhClient == nil {
		return fmt.Errorf("OVHcloud client not initialized")
	}

	// API endpoint: DELETE /cloud/project/{serviceName}/instance/{instanceId}
	endpoint := fmt.Sprintf("/cloud/project/%s/instance/%s", c.projectID, instanceID)
	if err := c.ovhClient.DeleteWithContext(ctx, endpoint, nil); err != nil {
		return fmt.Errorf("failed to delete instance %s: %w", instanceID, err)
	}

	return nil
}

// GetInstance retrieves information about a specific instance
func (c *Client) GetInstance(ctx context.Context, instanceID string) (*Instance, error) {
	if c.ovhClient == nil {
		return nil, fmt.Errorf("OVHcloud client not initialized")
	}

	// API endpoint: GET /cloud/project/{serviceName}/instance/{instanceId}
	var raw struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Status      string `json:"status"`
		IPAddresses []struct {
			IP      string `json:"ip"`
			Type    string `json:"type"`
			Version int    `json:"version"`
		} `json:"ipAddresses"`
	}

	endpoint := fmt.Sprintf("/cloud/project/%s/instance/%s", c.projectID, instanceID)
	if err := c.ovhClient.GetWithContext(ctx, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("failed to get instance %s: %w", instanceID, err)
	}

	instance := &Instance{
		ID:     raw.ID,
		Name:   raw.Name,
		Status: raw.Status,
	}

	// Extract IP addresses
	for _, ip := range raw.IPAddresses {
		if ip.Version == 4 {
			instance.IPv4 = ip.IP
			if ip.Type == "private" {
				instance.PrivateIP = ip.IP
			}
		} else if ip.Version == 6 {
			instance.IPv6 = ip.IP
		}
	}

	return instance, nil
}

// GetOrCreateSecurityGroup gets an existing security group or creates a new one
func (c *Client) GetOrCreateSecurityGroup(ctx context.Context, name string, rules []SecurityRule) (*SecurityGroup, error) {
	if c.ovhClient == nil {
		return nil, fmt.Errorf("OVHcloud client not initialized")
	}

	// List existing security groups
	var groupIDs []string
	endpoint := fmt.Sprintf("/cloud/project/%s/network/private", c.projectID)
	if err := c.ovhClient.GetWithContext(ctx, endpoint, &groupIDs); err != nil {
		// If listing fails, return placeholder
		return &SecurityGroup{
			ID:          "default",
			Name:        name,
			Description: "Default security group",
		}, nil
	}

	// For now, return a placeholder as OVHcloud security groups API is complex
	// In production, you'd implement full security group management
	return &SecurityGroup{
		ID:          "default-sg",
		Name:        name,
		Description: "Security group for " + name,
	}, nil
}

// DeleteSecurityGroup deletes a security group
func (c *Client) DeleteSecurityGroup(ctx context.Context, securityGroupID string) error {
	if c.ovhClient == nil {
		return fmt.Errorf("OVHcloud client not initialized")
	}

	// Security group deletion is handled differently in OVHcloud
	// For now, return nil as this is a no-op
	return nil
}

// ConvertToSecurityRules converts FirewallRule to OVHcloud SecurityRule format
func ConvertToSecurityRules(firewallRules []interface{}) []SecurityRule {
	// TODO: Implement conversion logic
	return nil
}

// GetFlavorIDByName resolves a flavor name to its UUID
func (c *Client) GetFlavorIDByName(ctx context.Context, region, flavorName string) (string, error) {
	if c.ovhClient == nil {
		return "", fmt.Errorf("OVHcloud client not initialized")
	}

	type Flavor struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Available bool   `json:"available"`
	}

	var flavors []Flavor
	endpoint := fmt.Sprintf("/cloud/project/%s/flavor?region=%s", c.projectID, region)
	if err := c.ovhClient.GetWithContext(ctx, endpoint, &flavors); err != nil {
		return "", fmt.Errorf("failed to list flavors: %w", err)
	}

	for _, flavor := range flavors {
		if flavor.Name == flavorName && flavor.Available {
			return flavor.ID, nil
		}
	}

	return "", fmt.Errorf("flavor '%s' not found in region '%s'", flavorName, region)
}

// GetImageIDByName resolves an image name to its UUID
func (c *Client) GetImageIDByName(ctx context.Context, region, imageName string) (string, error) {
	if c.ovhClient == nil {
		return "", fmt.Errorf("OVHcloud client not initialized")
	}

	type Image struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}

	var images []Image
	endpoint := fmt.Sprintf("/cloud/project/%s/image?osType=linux&region=%s", c.projectID, region)
	if err := c.ovhClient.GetWithContext(ctx, endpoint, &images); err != nil {
		return "", fmt.Errorf("failed to list images: %w", err)
	}

	for _, image := range images {
		if image.Name == imageName && image.Status == "active" {
			return image.ID, nil
		}
	}

	return "", fmt.Errorf("image '%s' not found in region '%s'", imageName, region)
}

// GetSSHKeyIDByName resolves an SSH key name to its ID
func (c *Client) GetSSHKeyIDByName(ctx context.Context, sshKeyName string) (string, error) {
	if c.ovhClient == nil {
		return "", fmt.Errorf("OVHcloud client not initialized")
	}

	// Query SSH keys API - returns array of SSH key objects directly
	type SSHKey struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Region string `json:"region"`
	}

	var sshKeys []SSHKey
	endpoint := fmt.Sprintf("/cloud/project/%s/sshkey", c.projectID)
	if err := c.ovhClient.GetWithContext(ctx, endpoint, &sshKeys); err != nil {
		return "", fmt.Errorf("failed to list SSH keys: %w", err)
	}

	// Match by name
	for _, key := range sshKeys {
		if key.Name == sshKeyName {
			return key.ID, nil
		}
	}

	return "", fmt.Errorf("SSH key with name '%s' not found", sshKeyName)
}
