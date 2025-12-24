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

package bootstrap

import (
	"strings"
	"testing"
)

func TestGenerateKubeadmCloudInit(t *testing.T) {
	generator := NewCloudInitGenerator()

	tests := []struct {
		name              string
		apiServerEndpoint string
		token             string
		caCertHash        string
		labels            map[string]string
		wantContains      []string
		wantNotContains   []string
	}{
		{
			name:              "basic kubeadm cloud-init",
			apiServerEndpoint: "10.0.0.1:6443",
			token:             "abcdef.0123456789abcdef",
			caCertHash:        "sha256:1234567890abcdef",
			labels: map[string]string{
				"node-role": "worker",
			},
			wantContains: []string{
				"#cloud-config",
				"kubeadm join 10.0.0.1:6443",
				"--token abcdef.0123456789abcdef",
				"--discovery-token-ca-cert-hash sha256:1234567890abcdef",
				"package_update: true",
				"containerd",
				"kubelet",
				"kubeadm",
			},
			wantNotContains: []string{
				"k3s",
				"rke2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generator.GenerateKubeadmCloudInit(
				tt.apiServerEndpoint,
				tt.token,
				tt.caCertHash,
				tt.labels,
			)

			if err != nil {
				t.Errorf("GenerateKubeadmCloudInit() error = %v", err)
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("GenerateKubeadmCloudInit() result missing %q", want)
				}
			}

			for _, notWant := range tt.wantNotContains {
				if strings.Contains(result, notWant) {
					t.Errorf("GenerateKubeadmCloudInit() result contains unwanted %q", notWant)
				}
			}
		})
	}
}

func TestGenerateK3sCloudInit(t *testing.T) {
	generator := NewCloudInitGenerator()

	tests := []struct {
		name         string
		serverURL    string
		token        string
		labels       map[string]string
		wantContains []string
	}{
		{
			name:      "basic k3s cloud-init",
			serverURL: "https://10.0.0.1:6443",
			token:     "K10abcdef1234567890::server:abcdef1234567890",
			labels: map[string]string{
				"node-role": "worker",
			},
			wantContains: []string{
				"#cloud-config",
				"server: https://10.0.0.1:6443",
				"token: K10abcdef1234567890::server:abcdef1234567890",
				"curl -sfL https://get.k3s.io",
				"node-label",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generator.GenerateK3sCloudInit(
				tt.serverURL,
				tt.token,
				tt.labels,
			)

			if err != nil {
				t.Errorf("GenerateK3sCloudInit() error = %v", err)
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("GenerateK3sCloudInit() result missing %q", want)
				}
			}
		})
	}
}

func TestGenerateRancherCloudInit(t *testing.T) {
	generator := NewCloudInitGenerator()

	tests := []struct {
		name         string
		serverURL    string
		token        string
		labels       map[string]string
		wantContains []string
	}{
		{
			name:      "basic rke2 cloud-init",
			serverURL: "https://10.0.0.1:9345",
			token:     "K10abcdef1234567890::server:abcdef1234567890",
			labels: map[string]string{
				"node-role": "worker",
			},
			wantContains: []string{
				"#cloud-config",
				"server: https://10.0.0.1:9345",
				"token: K10abcdef1234567890::server:abcdef1234567890",
				"curl -sfL https://get.rke2.io",
				"INSTALL_RKE2_TYPE=\"agent\"",
				"rke2-agent.service",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generator.GenerateRancherCloudInit(
				tt.serverURL,
				tt.token,
				tt.labels,
			)

			if err != nil {
				t.Errorf("GenerateRancherCloudInit() error = %v", err)
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("GenerateRancherCloudInit() result missing %q", want)
				}
			}
		})
	}
}

func TestGenerateKubeadmCloudInitWithVersion(t *testing.T) {
	generator := NewCloudInitGenerator()

	tests := []struct {
		name              string
		apiServerEndpoint string
		token             string
		caCertHash        string
		labels            map[string]string
		k8sVersion        string
		wantContains      []string
	}{
		{
			name:              "kubeadm with custom version",
			apiServerEndpoint: "10.0.0.1:6443",
			token:             "abcdef.0123456789abcdef",
			caCertHash:        "sha256:1234567890abcdef",
			labels:            map[string]string{},
			k8sVersion:        "1.30",
			wantContains: []string{
				"v1.30",
				"kubeadm join",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generator.GenerateKubeadmCloudInitWithVersion(
				tt.apiServerEndpoint,
				tt.token,
				tt.caCertHash,
				tt.labels,
				tt.k8sVersion,
			)

			if err != nil {
				t.Errorf("GenerateKubeadmCloudInitWithVersion() error = %v", err)
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("GenerateKubeadmCloudInitWithVersion() result missing %q", want)
				}
			}
		})
	}
}

func TestGenerateKubeadmCloudInitFull(t *testing.T) {
	generator := NewCloudInitGenerator()

	tests := []struct {
		name              string
		apiServerEndpoint string
		token             string
		caCertHash        string
		labels            map[string]string
		k8sVersion        string
		firewallRules     []string
		runCmd            []string
		wantContains      []string
	}{
		{
			name:              "kubeadm with firewall and custom commands",
			apiServerEndpoint: "10.0.0.1:6443",
			token:             "abcdef.0123456789abcdef",
			caCertHash:        "sha256:1234567890abcdef",
			labels:            map[string]string{},
			k8sVersion:        "1.29",
			firewallRules:     []string{"80/tcp", "443/tcp"},
			runCmd:            []string{"echo 'Custom command'"},
			wantContains: []string{
				"kubeadm join",
				"# User command",
				"echo 'Custom command'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generator.GenerateKubeadmCloudInitFull(
				tt.apiServerEndpoint,
				tt.token,
				tt.caCertHash,
				tt.labels,
				tt.k8sVersion,
				tt.firewallRules,
				tt.runCmd,
			)

			if err != nil {
				t.Errorf("GenerateKubeadmCloudInitFull() error = %v", err)
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("GenerateKubeadmCloudInitFull() result missing %q", want)
				}
			}
		})
	}
}
