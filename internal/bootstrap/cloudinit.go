// Package bootstrap provides cloud-init generation and token management for Kubernetes clusters.
package bootstrap

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

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"

	"github.com/autokubeio/autokube/internal/security"
)

//go:embed templates/*.yaml
var templateFS embed.FS

// CloudInitGenerator generates cloud-init configurations
type CloudInitGenerator struct {
	secretsManager *security.SecretsManager
}

// CloudInitGeneratorOption is a function that configures a CloudInitGenerator
type CloudInitGeneratorOption func(*CloudInitGenerator)

// WithSecretsManager sets a secrets manager for encryption
func WithSecretsManager(sm *security.SecretsManager) CloudInitGeneratorOption {
	return func(g *CloudInitGenerator) {
		g.secretsManager = sm
	}
}

// NewCloudInitGenerator creates a new cloud-init generator
func NewCloudInitGenerator(opts ...CloudInitGeneratorOption) *CloudInitGenerator {
	g := &CloudInitGenerator{}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// loadTemplate loads a template from the embedded filesystem
func (g *CloudInitGenerator) loadTemplate(name string) (*template.Template, error) {
	content, err := templateFS.ReadFile("templates/" + name)
	if err != nil {
		return nil, fmt.Errorf("failed to read template %s: %w", name, err)
	}
	return template.New(name).Parse(string(content))
}

// EncryptSensitiveData encrypts sensitive data if encryption is enabled
func (g *CloudInitGenerator) EncryptSensitiveData(data string) (string, error) {
	if g.secretsManager == nil {
		// If no secrets manager, return data as-is (backward compatibility)
		return data, nil
	}
	return g.secretsManager.EncryptData(data)
}

// GenerateKubeadmCloudInit generates cloud-init for kubeadm clusters
func (g *CloudInitGenerator) GenerateKubeadmCloudInit(
	apiServerEndpoint, token, caCertHash string,
	labels map[string]string,
) (string, error) {
	return g.GenerateKubeadmCloudInitWithVersion(apiServerEndpoint, token, caCertHash, labels, "1.29")
}

// GenerateKubeadmCloudInitWithVersion generates cloud-init for kubeadm clusters with specific version
func (g *CloudInitGenerator) GenerateKubeadmCloudInitWithVersion(
	apiServerEndpoint, token, caCertHash string,
	labels map[string]string,
	k8sVersion string,
) (string, error) {
	return g.GenerateKubeadmCloudInitFull(apiServerEndpoint, token, caCertHash, labels, k8sVersion, nil, nil)
}

// GenerateKubeadmCloudInitFull generates cloud-init for kubeadm clusters with firewall and custom commands
func (g *CloudInitGenerator) GenerateKubeadmCloudInitFull(
	apiServerEndpoint, token, caCertHash string,
	_ map[string]string,
	k8sVersion string,
	firewallRules []string,
	runCmd []string,
) (string, error) {
	t, err := g.loadTemplate("kubeadm.yaml")
	if err != nil {
		return "", err
	}

	config := struct {
		APIServerEndpoint   string
		Token               string
		CACertHash          string
		K8sVersion          string
		CustomFirewallRules []string
		RunCmd              []string
	}{
		APIServerEndpoint:   apiServerEndpoint,
		Token:               token,
		CACertHash:          caCertHash,
		K8sVersion:          k8sVersion,
		CustomFirewallRules: firewallRules,
		RunCmd:              runCmd,
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, config); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// GenerateK3sCloudInit generates cloud-init for k3s clusters
func (g *CloudInitGenerator) GenerateK3sCloudInit(serverURL, token string, labels map[string]string) (string, error) {
	t, err := g.loadTemplate("k3s.yaml")
	if err != nil {
		return "", err
	}

	config := struct {
		ServerURL string
		Token     string
		Labels    map[string]string
	}{
		ServerURL: serverURL,
		Token:     token,
		Labels:    labels,
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, config); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// GenerateTalosCloudInit generates cloud-init for Talos clusters
// Note: Talos doesn't use cloud-init but machine configs
func (g *CloudInitGenerator) GenerateTalosCloudInit(controlPlaneEndpoint, machineConfig string) (string, error) {
	t, err := g.loadTemplate("talos.yaml")
	if err != nil {
		return "", err
	}

	config := struct {
		ControlPlaneEndpoint string
		MachineConfig        string
	}{
		ControlPlaneEndpoint: controlPlaneEndpoint,
		MachineConfig:        machineConfig,
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, config); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// GenerateRancherCloudInit generates cloud-init for Rancher/RKE2 clusters
func (g *CloudInitGenerator) GenerateRancherCloudInit(
	serverURL, token string,
	labels map[string]string,
) (string, error) {
	t, err := g.loadTemplate("rke2.yaml")
	if err != nil {
		return "", err
	}

	config := struct {
		ServerURL string
		Token     string
		Labels    map[string]string
	}{
		ServerURL: serverURL,
		Token:     token,
		Labels:    labels,
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, config); err != nil {
		return "", err
	}

	return buf.String(), nil
}
