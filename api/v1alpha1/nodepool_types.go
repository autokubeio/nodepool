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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CloudProvider defines the cloud provider type
type CloudProvider string

// Supported cloud providers
const (
	CloudProviderHetzner  CloudProvider = "hetzner"
	CloudProviderOVHcloud CloudProvider = "ovhcloud"
	// Future providers can be added here:
	// CloudProviderAWS     CloudProvider = "aws"
	// CloudProviderGCP     CloudProvider = "gcp"
	// CloudProviderAzure   CloudProvider = "azure"
)

// NodePoolSpec defines the desired state of NodePool
type NodePoolSpec struct {
	// Provider is the cloud provider (e.g., hetzner, ovhcloud)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=hetzner;ovhcloud
	// +kubebuilder:default=hetzner
	Provider CloudProvider `json:"provider"`

	// HetznerConfig contains Hetzner Cloud specific configuration
	// Required when provider is "hetzner"
	// +optional
	HetznerConfig *HetznerCloudConfig `json:"hetznerConfig,omitempty"`

	// OVHcloudConfig contains OVHcloud Public Cloud specific configuration
	// Required when provider is "ovhcloud"
	// +optional
	OVHcloudConfig *OVHcloudConfig `json:"ovhcloudConfig,omitempty"`

	// MinNodes is the minimum number of nodes in the pool
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	MinNodes int `json:"minNodes"`

	// MaxNodes is the maximum number of nodes in the pool
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	MaxNodes int `json:"maxNodes"`

	// TargetNodes is the desired number of nodes
	// +kubebuilder:validation:Minimum=0
	TargetNodes int `json:"targetNodes,omitempty"`

	// CloudInit is the cloud-init configuration for node initialization
	// +optional
	CloudInit string `json:"cloudInit,omitempty"`

	// SSHKeys is a list of SSH key IDs or names to add to the nodes
	// +optional
	SSHKeys []string `json:"sshKeys,omitempty"`

	// Labels are additional labels to apply to cloud provider resources
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// AutoScalingEnabled enables automatic scaling based on cluster load
	// +kubebuilder:default=true
	AutoScalingEnabled bool `json:"autoScalingEnabled"`

	// ScaleUpThreshold is the number of pending pods to trigger scale up
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=5
	ScaleUpThreshold int `json:"scaleUpThreshold,omitempty"`

	// ScaleDownThreshold is the CPU utilization percentage to trigger scale down
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=30
	ScaleDownThreshold int `json:"scaleDownThreshold,omitempty"`

	// Bootstrap contains cluster bootstrap configuration for automatic node joining
	// +optional
	Bootstrap *ClusterBootstrapConfig `json:"bootstrap,omitempty"`

	// FirewallRules contains custom firewall rules to apply
	// +optional
	FirewallRules []FirewallRule `json:"firewallRules,omitempty"`

	// RunCmd contains commands to run after node initialization
	// +optional
	RunCmd []string `json:"runCmd,omitempty"`
}

// HetznerCloudConfig contains Hetzner Cloud specific configuration
type HetznerCloudConfig struct {
	// ServerType is the Hetzner Cloud server type (e.g., cx11, cpx21)
	// +kubebuilder:validation:Required
	ServerType string `json:"serverType"`

	// Location is the Hetzner Cloud location (e.g., nbg1, fsn1, hel1)
	// +kubebuilder:validation:Required
	Location string `json:"location"`

	// Image is the OS image to use for nodes (e.g., ubuntu-22.04)
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// Network is the Hetzner Cloud network ID or name to attach nodes to
	// +optional
	Network string `json:"network,omitempty"`
}

// OVHcloudConfig contains OVHcloud Public Cloud specific configuration
type OVHcloudConfig struct {
	// Flavor is the flavor (instance type) name to use for instances (e.g., "b3-8", "c2-7")
	// Either Flavor or FlavorID must be specified
	// +optional
	Flavor string `json:"flavor,omitempty"`

	// FlavorID is the flavor (instance type) UUID to use for instances
	// Either Flavor or FlavorID must be specified
	// +optional
	FlavorID string `json:"flavorID,omitempty"`

	// Region is the OVHcloud region (e.g., GRA11, SBG5, BHS5, US-EAST-VA-1)
	// +kubebuilder:validation:Required
	Region string `json:"region"`

	// Image is the OS image name to use for instances (e.g., "Ubuntu 22.04")
	// Either Image or ImageID must be specified
	// +optional
	Image string `json:"image,omitempty"`

	// ImageID is the OS image UUID to use for instances
	// Either Image or ImageID must be specified
	// +optional
	ImageID string `json:"imageID,omitempty"`

	// Network is the OVHcloud private network name (vRack) to attach instances to
	// Either Network or NetworkID can be specified
	// +optional
	Network string `json:"network,omitempty"`

	// NetworkID is the OVHcloud private network ID (vRack) to attach instances to
	// Either Network or NetworkID can be specified
	// +optional
	NetworkID string `json:"networkID,omitempty"`

	// ProjectID is the OVHcloud project ID
	// +kubebuilder:validation:Required
	ProjectID string `json:"projectID"`
}

// FirewallRule defines a single firewall rule
type FirewallRule struct {
	// Port is the port or port range (e.g., "80", "8080:8090")
	Port string `json:"port"`

	// Protocol is the protocol (tcp, udp)
	// +kubebuilder:default=tcp
	Protocol string `json:"protocol,omitempty"`

	// Description is a human-readable description
	// +optional
	Description string `json:"description,omitempty"`
}

// NodePoolStatus defines the observed state of NodePool
type NodePoolStatus struct {
	// CurrentNodes is the current number of nodes in the pool
	CurrentNodes int `json:"currentNodes"`

	// ReadyNodes is the number of ready nodes
	ReadyNodes int `json:"readyNodes"`

	// Nodes is a list of node names in the pool
	Nodes []string `json:"nodes,omitempty"`

	// LastScaleTime is the last time the pool was scaled
	// +optional
	LastScaleTime *metav1.Time `json:"lastScaleTime,omitempty"`

	// Conditions represent the latest available observations of the node pool's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Phase represents the current phase of the node pool
	// +optional
	Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=np
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.provider`
// +kubebuilder:printcolumn:name="ServerType",type=string,JSONPath=`.spec.hetznerConfig.serverType`
// +kubebuilder:printcolumn:name="Location",type=string,JSONPath=`.spec.hetznerConfig.location`
// +kubebuilder:printcolumn:name="Min",type=integer,JSONPath=`.spec.minNodes`
// +kubebuilder:printcolumn:name="Max",type=integer,JSONPath=`.spec.maxNodes`
// +kubebuilder:printcolumn:name="Current",type=integer,JSONPath=`.status.currentNodes`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyNodes`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NodePool is the Schema for the nodepools API
type NodePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodePoolSpec   `json:"spec,omitempty"`
	Status NodePoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodePoolList contains a list of NodePool
type NodePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodePool{}, &NodePoolList{})
}
