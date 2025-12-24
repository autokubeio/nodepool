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

// Package mock provides mock implementations for testing.
package mock

import (
	"context"
	"fmt"
	"sync"

	"github.com/autokubeio/autokube/internal/hetzner"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// HetznerClient is a mock implementation of the Hetzner Cloud client for testing
type HetznerClient struct {
	mu      sync.RWMutex
	servers map[int64]*hetzner.Server
	nextID  int64

	// Configurable behaviors for testing
	ListServersFunc  func(ctx context.Context, nodePoolName, namespace string) ([]hetzner.Server, error)
	CreateServerFunc func(ctx context.Context, config hetzner.ServerConfig) (*hetzner.Server, error)
	DeleteServerFunc func(ctx context.Context, serverID int64) error
	GetServerFunc    func(ctx context.Context, serverID int64) (*hetzner.Server, error)

	// Call tracking for assertions
	ListServersCalls  int
	CreateServerCalls int
	DeleteServerCalls int
	GetServerCalls    int
}

// NewMockHetznerClient creates a new mock Hetzner client
func NewMockHetznerClient() *HetznerClient {
	return &HetznerClient{
		servers: make(map[int64]*hetzner.Server),
		nextID:  1,
	}
}

// ListServers lists all servers for a given node pool
func (m *HetznerClient) ListServers(ctx context.Context, nodePoolName, namespace string) ([]hetzner.Server, error) {
	m.mu.Lock()
	m.ListServersCalls++
	m.mu.Unlock()

	if m.ListServersFunc != nil {
		return m.ListServersFunc(ctx, nodePoolName, namespace)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var servers []hetzner.Server
	for _, server := range m.servers {
		servers = append(servers, *server)
	}

	return servers, nil
}

// CreateServer creates a new server
func (m *HetznerClient) CreateServer(ctx context.Context, config hetzner.ServerConfig) (*hetzner.Server, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CreateServerCalls++

	if m.CreateServerFunc != nil {
		return m.CreateServerFunc(ctx, config)
	}

	server := &hetzner.Server{
		ID:     m.nextID,
		Name:   config.Name,
		Status: "running",
		IPv4:   fmt.Sprintf("192.0.2.%d", m.nextID), // TEST-NET-1 address
		IPv6:   fmt.Sprintf("2001:db8::%d", m.nextID),
	}

	m.servers[m.nextID] = server
	m.nextID++

	return server, nil
}

// DeleteServer deletes a server
func (m *HetznerClient) DeleteServer(ctx context.Context, serverID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeleteServerCalls++

	if m.DeleteServerFunc != nil {
		return m.DeleteServerFunc(ctx, serverID)
	}

	if _, exists := m.servers[serverID]; !exists {
		return fmt.Errorf("server %d not found", serverID)
	}

	delete(m.servers, serverID)
	return nil
}

// GetServer gets a server by ID
func (m *HetznerClient) GetServer(ctx context.Context, serverID int64) (*hetzner.Server, error) {
	m.mu.Lock()
	m.GetServerCalls++
	m.mu.Unlock()

	if m.GetServerFunc != nil {
		return m.GetServerFunc(ctx, serverID)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	server, exists := m.servers[serverID]
	if !exists {
		return nil, fmt.Errorf("server %d not found", serverID)
	}

	return server, nil
}

// Reset resets the mock state for a new test
func (m *HetznerClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.servers = make(map[int64]*hetzner.Server)
	m.nextID = 1
	m.ListServersCalls = 0
	m.CreateServerCalls = 0
	m.DeleteServerCalls = 0
	m.GetServerCalls = 0
}

// SetServers sets the servers for testing
func (m *HetznerClient) SetServers(servers map[int64]*hetzner.Server) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.servers = servers
}

// GetServers returns all servers for assertions
func (m *HetznerClient) GetServers() map[int64]*hetzner.Server {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent race conditions
	servers := make(map[int64]*hetzner.Server)
	for k, v := range m.servers {
		servers[k] = v
	}
	return servers
}

// GetOrCreateFirewall mock implementation
func (m *HetznerClient) GetOrCreateFirewall(_ context.Context, name string, _ []hcloud.FirewallRule) (*hcloud.Firewall, error) {
	// Simple mock implementation that returns a firewall
	return &hcloud.Firewall{
		ID:   1,
		Name: name,
	}, nil
}

// DeleteFirewall mock implementation
func (m *HetznerClient) DeleteFirewall(_ context.Context, _ int64) error {
	// Simple mock implementation
	return nil
}
