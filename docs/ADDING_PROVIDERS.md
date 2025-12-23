# Adding New Cloud Providers to NodePool

This document describes how to extend the NodePool operator to support additional cloud providers beyond Hetzner Cloud.

## Architecture Overview

The NodePool CRD now uses a provider-based architecture:

```yaml
apiVersion: hcloud.autokube.io/v1alpha1
kind: NodePool
metadata:
  name: example-pool
spec:
  provider: hetzner  # Cloud provider identifier
  
  # Provider-specific configuration
  hetznerConfig:
    serverType: cpx11
    location: nbg1
    image: ubuntu-22.04
    network: my-network  # optional
  
  # Common fields (shared across all providers)
  minNodes: 1
  maxNodes: 10
  targetNodes: 3
  autoScalingEnabled: true
  labels:
    role: worker
  sshKeys:
    - my-key
```

## Adding a New Provider

### Step 1: Update API Types

Add your provider to the `CloudProvider` enum in `api/v1alpha1/nodepool_types.go`:

```go
// CloudProvider defines the cloud provider type
type CloudProvider string

const (
	CloudProviderHetzner CloudProvider = "hetzner"
	CloudProviderAWS     CloudProvider = "aws"      // New provider
	CloudProviderGCP     CloudProvider = "gcp"      // New provider
	CloudProviderAzure   CloudProvider = "azure"    // New provider
)
```

Update the kubebuilder validation enum:

```go
type NodePoolSpec struct {
	// Provider is the cloud provider (e.g., hetzner, aws, gcp, azure)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=hetzner;aws;gcp;azure
	// +kubebuilder:default=hetzner
	Provider CloudProvider `json:"provider"`
	
	// ... existing fields ...
}
```

### Step 2: Create Provider-Specific Config Struct

Add a new configuration struct for your provider:

```go
// AWSConfig contains AWS specific configuration
type AWSConfig struct {
	// InstanceType is the EC2 instance type (e.g., t3.medium, c5.large)
	// +kubebuilder:validation:Required
	InstanceType string `json:"instanceType"`

	// Region is the AWS region (e.g., us-east-1, eu-west-1)
	// +kubebuilder:validation:Required
	Region string `json:"region"`

	// AMI is the Amazon Machine Image ID
	// +kubebuilder:validation:Required
	AMI string `json:"ami"`

	// SubnetID is the VPC subnet ID to launch instances in
	// +optional
	SubnetID string `json:"subnetId,omitempty"`
	
	// SecurityGroupIDs are the security groups to attach
	// +optional
	SecurityGroupIDs []string `json:"securityGroupIds,omitempty"`
	
	// IAMInstanceProfile is the IAM role for the instances
	// +optional
	IAMInstanceProfile string `json:"iamInstanceProfile,omitempty"`
}
```

Add the config field to `NodePoolSpec`:

```go
type NodePoolSpec struct {
	// ... existing fields ...
	
	// AWSConfig contains AWS specific configuration
	// Required when provider is "aws"
	// +optional
	AWSConfig *AWSConfig `json:"awsConfig,omitempty"`
	
	// ... rest of fields ...
}
```

### Step 3: Implement Provider Client Interface

Create a new client package under `internal/providers/`:

```
internal/
  providers/
    interface.go          # Common provider interface
    hetzner/
      client.go           # Existing Hetzner implementation
    aws/
      client.go           # New AWS implementation
    gcp/
      client.go           # New GCP implementation
```

Define a common provider interface in `internal/providers/interface.go`:

```go
package providers

import (
	"context"
	hcloudv1alpha1 "github.com/autokubeio/autokube/api/v1alpha1"
)

// CloudProvider defines the interface that all cloud providers must implement
type CloudProvider interface {
	// ListServers returns all servers for the given node pool
	ListServers(ctx context.Context, poolName, namespace string) ([]Server, error)
	
	// CreateServer creates a new server
	CreateServer(ctx context.Context, spec *hcloudv1alpha1.NodePoolSpec, name string) (*Server, error)
	
	// DeleteServer deletes a server by ID
	DeleteServer(ctx context.Context, serverID string) error
	
	// GetServer retrieves server information
	GetServer(ctx context.Context, serverID string) (*Server, error)
}

// Server represents a generic cloud server
type Server struct {
	ID         string
	Name       string
	Status     string
	PublicIP   string
	PrivateIP  string
	Labels     map[string]string
	CreatedAt  time.Time
}
```

Implement the interface for your provider in `internal/providers/aws/client.go`:

```go
package aws

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	hcloudv1alpha1 "github.com/autokubeio/autokube/api/v1alpha1"
	"github.com/autokubeio/autokube/internal/providers"
)

type AWSProvider struct {
	ec2Client *ec2.EC2
}

func NewAWSProvider(region string) (*AWSProvider, error) {
	// Initialize AWS SDK client
	// ...
}

func (p *AWSProvider) ListServers(ctx context.Context, poolName, namespace string) ([]providers.Server, error) {
	// Implement AWS-specific server listing
	// ...
}

func (p *AWSProvider) CreateServer(ctx context.Context, spec *hcloudv1alpha1.NodePoolSpec, name string) (*providers.Server, error) {
	// Implement AWS EC2 instance creation
	awsConfig := spec.AWSConfig
	if awsConfig == nil {
		return nil, fmt.Errorf("awsConfig is required for AWS provider")
	}
	
	// Use awsConfig.InstanceType, awsConfig.Region, etc.
	// ...
}

func (p *AWSProvider) DeleteServer(ctx context.Context, serverID string) error {
	// Implement AWS instance termination
	// ...
}

func (p *AWSProvider) GetServer(ctx context.Context, serverID string) (*providers.Server, error) {
	// Implement AWS instance retrieval
	// ...
}
```

### Step 4: Update the Controller

Update `internal/controller/nodepool_controller.go` to support multiple providers:

```go
type NodePoolReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	ProviderFactory    *providers.Factory  // New: provider factory
	MetricsClient      *metrics.Collector
	KubeClient         kubernetes.Interface
	BootstrapManager   *bootstrap.BootstrapTokenManager
	CloudInitGenerator *bootstrap.CloudInitGenerator
}

func (r *NodePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// ... existing code ...
	
	// Get provider client based on nodePool.Spec.Provider
	providerClient, err := r.ProviderFactory.GetProvider(nodePool.Spec.Provider)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get provider client: %w", err)
	}
	
	// Use provider client for operations
	servers, err := providerClient.ListServers(ctx, nodePool.Name, nodePool.Namespace)
	// ...
}
```

Create a provider factory in `internal/providers/factory.go`:

```go
package providers

import (
	"fmt"
	hcloudv1alpha1 "github.com/autokubeio/autokube/api/v1alpha1"
	"github.com/autokubeio/autokube/internal/providers/aws"
	"github.com/autokubeio/autokube/internal/providers/hetzner"
)

type Factory struct {
	hetznerClient *hetzner.Client
	// Add other provider clients as needed
}

func NewFactory(hetznerToken string, awsRegion string) (*Factory, error) {
	// Initialize providers
	hetznerClient := hetzner.NewClient(hetznerToken)
	
	return &Factory{
		hetznerClient: hetznerClient,
	}, nil
}

func (f *Factory) GetProvider(provider hcloudv1alpha1.CloudProvider) (CloudProvider, error) {
	switch provider {
	case hcloudv1alpha1.CloudProviderHetzner:
		return f.hetznerClient, nil
	case hcloudv1alpha1.CloudProviderAWS:
		// Return AWS provider
		return aws.NewAWSProvider()
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}
```

### Step 5: Update CRD and Regenerate

After updating the API types, regenerate the CRD:

```bash
# Generate deepcopy methods
make generate

# Generate CRD manifests
make manifests
```

### Step 6: Create Examples

Add example YAML files for the new provider:

```yaml
# examples/nodepool-aws.yaml
apiVersion: hcloud.autokube.io/v1alpha1
kind: NodePool
metadata:
  name: worker-pool-aws
  namespace: default
spec:
  provider: aws
  
  awsConfig:
    instanceType: t3.medium
    region: us-east-1
    ami: ami-0c55b159cbfafe1f0
    subnetId: subnet-12345678
    securityGroupIds:
      - sg-12345678
    iamInstanceProfile: k8s-node-role
  
  minNodes: 2
  maxNodes: 10
  targetNodes: 3
  autoScalingEnabled: true
  
  bootstrap:
    type: kubeadm
    autoGenerateToken: true
    kubernetesVersion: "1.32"
  
  labels:
    role: worker
    provider: aws
  
  sshKeys:
    - my-aws-key
```

### Step 7: Documentation

Update the main README.md to document the new provider:

```markdown
## Supported Cloud Providers

- **Hetzner Cloud** - Production ready
- **AWS** - Beta
- **GCP** - Coming soon
- **Azure** - Coming soon

### AWS Configuration

See [examples/nodepool-aws.yaml](examples/nodepool-aws.yaml) for a complete example.
```

## Provider-Specific Considerations

### Authentication

Each provider needs its own authentication mechanism:

- **Hetzner**: API token from environment variable `HCLOUD_TOKEN`
- **AWS**: AWS credentials from environment variables or IAM role
- **GCP**: Service account JSON key
- **Azure**: Service principal credentials

Update the manager deployment to include necessary credentials:

```yaml
# config/manager/manager.yaml
env:
  - name: HCLOUD_TOKEN
    valueFrom:
      secretKeyRef:
        name: hcloud-secret
        key: token
  - name: AWS_ACCESS_KEY_ID
    valueFrom:
      secretKeyRef:
        name: aws-credentials
        key: access-key-id
  - name: AWS_SECRET_ACCESS_KEY
    valueFrom:
      secretKeyRef:
        name: aws-credentials
        key: secret-access-key
```

### Cloud-Specific Features

Some cloud providers may have unique features:

- **Spot/Preemptible Instances**: Add optional fields for spot instance configuration
- **Placement Groups**: Provider-specific instance placement options
- **Storage Options**: EBS volumes, persistent disks, etc.

Add these as optional fields in the provider-specific config structs.

## Testing

Create provider-specific tests:

```go
// internal/providers/aws/client_test.go
func TestAWSProvider_CreateServer(t *testing.T) {
	// Mock EC2 API
	// Test instance creation
}
```

## Migration from Old Structure

For backward compatibility with the deprecated fields (`serverType`, `location`, `image`, `network`), add logic in the controller:

```go
func (r *NodePoolReconciler) migrateToProviderConfig(nodePool *hcloudv1alpha1.NodePool) {
	// If using old format, populate hetznerConfig
	if nodePool.Spec.ServerType != "" && nodePool.Spec.HetznerConfig == nil {
		nodePool.Spec.HetznerConfig = &hcloudv1alpha1.HetznerCloudConfig{
			ServerType: nodePool.Spec.ServerType,
			Location:   nodePool.Spec.Location,
			Image:      nodePool.Spec.Image,
			Network:    nodePool.Spec.Network,
		}
	}
}
```

## Summary

To add a new cloud provider:

1. ✅ Update API types (CloudProvider enum, config struct)
2. ✅ Implement provider interface
3. ✅ Update controller to use provider factory
4. ✅ Add authentication handling
5. ✅ Create example YAML files
6. ✅ Update documentation
7. ✅ Add tests
8. ✅ Regenerate CRDs

This architecture makes it easy to add new cloud providers without breaking existing deployments.
