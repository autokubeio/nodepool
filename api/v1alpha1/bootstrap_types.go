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

package v1alpha1

// ClusterType defines the type of Kubernetes cluster
type ClusterType string

// Supported cluster bootstrap types
const (
	ClusterTypeKubeadm ClusterType = "kubeadm"
	ClusterTypeK3s     ClusterType = "k3s"
	ClusterTypeTalos   ClusterType = "talos"
	ClusterTypeRKE2    ClusterType = "rke2"
	ClusterTypeRancher ClusterType = "rancher"
)

// ClusterBootstrapConfig contains configuration for joining nodes to the cluster
type ClusterBootstrapConfig struct {
	// Type is the type of cluster (kubeadm, k3s, talos, rke2)
	// +kubebuilder:validation:Enum=kubeadm;k3s;talos;rke2;rancher
	// +kubebuilder:default=kubeadm
	Type ClusterType `json:"type,omitempty"`

	// APIServerEndpoint is the endpoint of the Kubernetes API server
	// Required for kubeadm clusters
	// +optional
	APIServerEndpoint string `json:"apiServerEndpoint,omitempty"`

	// TokenSecretRef is a reference to a secret containing the bootstrap token
	// The secret should have keys: token, ca-cert-hash (for kubeadm)
	// +optional
	TokenSecretRef *SecretReference `json:"tokenSecretRef,omitempty"`

	// AutoGenerateToken indicates whether to automatically generate bootstrap tokens
	// +kubebuilder:default=true
	AutoGenerateToken bool `json:"autoGenerateToken,omitempty"`

	// KubernetesVersion specifies the Kubernetes version to install (e.g., "1.29", "1.30")
	// +kubebuilder:default="1.29"
	// +optional
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`

	// K3sConfig contains k3s-specific configuration
	// +optional
	K3sConfig *K3sBootstrapConfig `json:"k3sConfig,omitempty"`

	// TalosConfig contains Talos-specific configuration
	// +optional
	TalosConfig *TalosBootstrapConfig `json:"talosConfig,omitempty"`

	// RKE2Config contains RKE2-specific configuration
	// +optional
	RKE2Config *RKE2BootstrapConfig `json:"rke2Config,omitempty"`
}

// SecretReference references a secret in the same namespace
type SecretReference struct {
	// Name is the name of the secret
	Name string `json:"name"`

	// Key is the key in the secret containing the token
	// +kubebuilder:default=token
	Key string `json:"key,omitempty"`
}

// K3sBootstrapConfig contains k3s-specific bootstrap configuration
type K3sBootstrapConfig struct {
	// ServerURL is the k3s server URL
	ServerURL string `json:"serverURL"`

	// TokenSecretRef references the secret containing the k3s token
	TokenSecretRef *SecretReference `json:"tokenSecretRef,omitempty"`
}

// TalosBootstrapConfig contains Talos-specific bootstrap configuration
type TalosBootstrapConfig struct {
	// ControlPlaneEndpoint is the Talos control plane endpoint
	ControlPlaneEndpoint string `json:"controlPlaneEndpoint"`

	// ConfigSecretRef references the secret containing Talos machine config
	ConfigSecretRef *SecretReference `json:"configSecretRef,omitempty"`
}

// RKE2BootstrapConfig contains RKE2-specific bootstrap configuration
type RKE2BootstrapConfig struct {
	// ServerURL is the RKE2 server URL
	ServerURL string `json:"serverURL"`

	// TokenSecretRef references the secret containing the RKE2 token
	TokenSecretRef *SecretReference `json:"tokenSecretRef,omitempty"`
}
