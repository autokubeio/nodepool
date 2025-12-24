# NodePool - Multi-Cloud Kubernetes Autoscaler Operator

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/autokubeio/nodepool)](https://goreportcard.com/report/github.com/autokubeio/nodepool)
[![GitHub release](https://img.shields.io/github/release/autokubeio/nodepool.svg)](https://github.com/autokubeio/nodepool/releases)

A Kubernetes operator for autoscaling cloud nodes based on cluster load. This operator provides seamless integration with multiple cloud providers to automatically manage your cluster's compute resources.

## Features

- ✨ **Custom Resource Definition (CRD)** for NodePool management
- 🚀 **Automatic node provisioning** and deprovisioning
- 🔌 **Full integration** with Hetzner Cloud API
- 🛡️ **Automatic Firewall Management** - creates and manages Hetzner Cloud Firewalls
- ☁️ **Automatic cluster joining** - supports kubeadm, k3s, Talos, and RKE2
- 🔑 **Bootstrap token management** - automatic token generation and rotation
- 📊 **Pod-based autoscaling** - scale based on pending pods
- 🔄 **Graceful node drain** before deletion
- 📈 **Prometheus metrics** for monitoring
- 🎯 **Multi-region support** across all Hetzner locations
- 🌐 **Multi-cluster support** - works with different Kubernetes distributions
- ⚙️ **Custom cloud-init commands** - execute post-initialization scripts
- 🔧 **Kubernetes version control** - specify exact K8s version per node pool
- 🔒 **Secure** - runs with minimal privileges
- 🎨 **Easy to use** - simple YAML configuration
- 🏗️ **Multi-cloud ready** - extensible provider architecture (currently Hetzner, more coming soon)

## Supported Cloud Providers

- ✅ **Hetzner Cloud** - Production ready
- 🔜 **OVHcloud** - Planned Q1 2026
- 🔜 **UpCloud** - Planned Q1 2026
- 🔜 **DigitalOcean** - Planned Q1 2026
- 🔜 **Scaleway** - Planned Q2 2026
- 🔜 **Linode/Akamai** - Planned Q2 2026
- 🔜 **Vultr** - Planned Q3 2026
- 🔜 **Contabo** - Planned Q3 2026

The operator uses a provider-based architecture that makes it easy to add support for additional cloud providers. See [Adding Providers Guide](docs/ADDING_PROVIDERS.md) for details on implementing new providers.

## Architecture

The operator watches `NodePool` custom resources and automatically manages the lifecycle of Hetzner Cloud servers. It monitors cluster load and scales the node pool up or down based on configurable thresholds.

## Prerequisites

- Kubernetes cluster (v1.19+)
- Hetzner Cloud account and API token
- Helm 3.x (for installation)
- `kubectl` configured to access your cluster

## Installation

### Using Helm (Recommended)

1. **Add the Helm repository:**

```bash
helm repo add nodepool https://autokubeio.github.io/nodepool
helm repo update
```

2. **Create a namespace:**

```bash
kubectl create namespace nodepool-system
```

3. **Install the operator with your Hetzner Cloud token:**

```bash
helm install nodepool-operator nodepool/nodepool \
  --namespace nodepool-system \
  --set hcloudToken=<YOUR_HCLOUD_TOKEN>
```

**Or using a secret:**

```bash
# Create secret with your token
kubectl create secret generic hcloud-token \
  --from-literal=token=<YOUR_HCLOUD_TOKEN> \
  -n nodepool-system

# Install with existing secret
helm install nodepool-operator nodepool/nodepool \
  --namespace nodepool-system \
  --set hcloudTokenSecret=hcloud-token
```

4. **Verify the installation:**

```bash
kubectl get pods -n nodepool-system
kubectl get crd nodepools.autokube.io
```

### Manual Installation

**Option 1: All-in-One Manifest (Easiest)**

```bash
# Download and apply the complete installation manifest
kubectl apply -f https://raw.githubusercontent.com/autokubeio/nodepool/main/dist/install.yaml

# Create namespace and secret
kubectl create namespace nodepool-system
kubectl create secret generic hcloud-credentials \
  --from-literal=token=<YOUR_HCLOUD_TOKEN> \
  -n nodepool-system
```

**Option 2: Step-by-Step Installation**

1. **Apply the CRD:**

```bash
kubectl apply -f https://raw.githubusercontent.com/autokubeio/nodepool/main/config/crd/bases/autokube.io_nodepools.yaml
```

2. **Create the namespace and secret:**

```bash
kubectl create namespace nodepool-system
kubectl create secret generic hcloud-token \
  --from-literal=token=<YOUR_HCLOUD_TOKEN> \
  -n nodepool-system
```

3. **Deploy the operator:**

```bash
# Apply RBAC
kubectl apply -f https://raw.githubusercontent.com/autokubeio/nodepool/main/config/rbac/role.yaml

# Apply manager deployment
kubectl apply -f https://raw.githubusercontent.com/autokubeio/nodepool/main/config/manager/manager.yaml
```

## Usage

### Basic Example

Create a node pool with automatic scaling:

```yaml
apiVersion: autokube.io/v1alpha1
kind: NodePool
metadata:
  name: worker-pool
  namespace: default
spec:
  provider: hetzner      # Cloud provider (currently: hetzner)
  
  # Hetzner Cloud configuration
  hetznerConfig:
    serverType: cx11     # Server type (cx11, cpx21, ccx13, etc.)
    location: nbg1       # Location (nbg1, fsn1, hel1, etc.)
    image: ubuntu-22.04  # OS image
  
  minNodes: 2            # Minimum nodes
  maxNodes: 10           # Maximum nodes
  targetNodes: 3         # Initial target (optional)
  autoScalingEnabled: true
  scaleUpThreshold: 5    # Scale up when 5+ pods are pending
  scaleDownThreshold: 30 # Scale down at 30% utilization

  # SSH keys for access
  sshKeys:
    - my-ssh-key         # Your SSH key name in Hetzner
```

Apply the configuration:

```bash
kubectl apply -f nodepool.yaml
```

### Advanced Example with Cloud-Init

```yaml
apiVersion: autokube.io/v1alpha1
kind: NodePool
metadata:
  name: production-pool
  namespace: production
spec:
  provider: hetzner      # Cloud provider (currently: hetzner)
 
  # Hetzner Cloud configuration
  hetznerConfig:
    serverType: cx11     # Server type (cx11, cpx21, ccx13, etc.)
    location: nbg1       # Location (nbg1, fsn1, hel1, etc.)
    image: ubuntu-22.04  # OS image

  minNodes: 2            # Minimum nodes
  maxNodes: 10           # Maximum nodes
  targetNodes: 3         # Initial target (optional)
  autoScalingEnabled: true
  scaleUpThreshold: 5    # Scale up when 5+ pods are pending
  scaleDownThreshold: 30 # Scale down at 30% utilization

  # SSH keys for access
  sshKeys:
    - production-key
  
  # Custom labels for Hetzner Cloud
  labels:
    environment: production
    team: platform
  
  # Cloud-init configuration
  cloudInit: |
    #cloud-config
    package_update: true
    package_upgrade: true
    packages:
      - docker.io
      - kubelet
      - kubeadm
      - kubectl
    runcmd:
      - systemctl enable docker
      - systemctl start docker
      - kubeadm join <your-cluster-endpoint> --token <token> --discovery-token-ca-cert-hash <hash>
```

### Automatic Cluster Joining

The operator supports **automatic node joining** for multiple Kubernetes distributions, eliminating the need to manually configure cloud-init scripts with bootstrap tokens.

#### Kubeadm Clusters

For standard kubeadm clusters, the operator automatically generates bootstrap tokens and configures nodes to join:

```yaml
apiVersion: autokube.io/v1alpha1
kind: NodePool
metadata:
  name: worker-pool-kubeadm
  namespace: default
spec:
  # Cloud provider - currently supports: hetzner
  provider: hetzner
  
  # Hetzner Cloud specific configuration
  hetznerConfig:
    serverType: cpx11
    location: nbg1
    image: ubuntu-22.04
    network: "network-1"  # Network name or ID
  
  minNodes: 1
  maxNodes: 5
  targetNodes: 1
  autoScalingEnabled: true
  scaleUpThreshold: 5
  scaleDownThreshold: 30
  
  # Automatic cluster joining with kubeadm
  bootstrap:
    type: kubeadm
    autoGenerateToken: true
    kubernetesVersion: "1.32"  # Kubernetes version to install
    # Optional: override API server endpoint
    # apiServerEndpoint: "10.0.0.1:6443"
  
  labels:
    role: worker
    environment: production
  
  # Firewall rules
  firewallRules:
    - port: "80"
      protocol: tcp
      description: "HTTP traffic"
    - port: "443"
      protocol: tcp
      description: "HTTPS traffic"
  
  # Commands to run after initialization
  runCmd:
    - echo "Custom setup starting..."
    - apt-get install -y htop vim
    - echo "Custom setup completed"
    
  sshKeys:
    - my-ssh-key

```

#### K3s Clusters

For k3s clusters, provide the server URL and token:

```yaml
apiVersion: autokube.io/v1alpha1
kind: NodePool
metadata:
  name: worker-pool-k3s
  namespace: default
spec:
  # Cloud provider - currently supports: hetzner
  provider: hetzner
  
  # Hetzner Cloud specific configuration
  hetznerConfig:
    serverType: cpx21
    location: nbg1  # Changed from fsn1 (Nuremberg datacenter)
    image: ubuntu-22.04
  
  minNodes: 1
  maxNodes: 10
  targetNodes: 2
  autoScalingEnabled: true
  
  # Automatic cluster joining with k3s
  bootstrap:
    type: k3s
    k3sConfig:
      serverURL: "https://k3s-server:6443"
      tokenSecretRef:
        name: k3s-token
        key: token
  
  labels:
    role: worker
    cluster-type: k3s

  firewallRules:
    - port: "80"
      protocol: tcp
      description: "HTTP traffic"
    - port: "443"
      protocol: tcp
      description: "HTTPS traffic"
    
  sshKeys:
    - my-ssh-key
---
apiVersion: v1
kind: Secret
metadata:
  name: k3s-token
  namespace: default
type: Opaque
stringData:
    token: "K10abcdef1234567890::server:abcdef1234567890"
```

#### RKE2/Rancher Clusters

For RKE2 or Rancher-managed clusters:

```yaml
apiVersion: autokube.io/v1alpha1
kind: NodePool
metadata:
  name: worker-pool-rke2
  namespace: default
spec:
  # Cloud provider - currently supports: hetzner
  provider: hetzner
  
  # Hetzner Cloud specific configuration
  hetznerConfig:
    serverType: cpx22
    location: nbg1
    image: ubuntu-22.04
    network: "network-1"
  
  minNodes: 1
  maxNodes: 8
  targetNodes: 2
  autoScalingEnabled: true
  
  # Automatic cluster joining with RKE2/Rancher
  bootstrap:
    type: rke2
    rke2Config:
      serverURL: "https://rke2-server:9345"
      tokenSecretRef:
        name: rke2-token
        key: token
  
  labels:
    role: worker
    cluster-type: rke2

  firewallRules:
    - port: "80"
      protocol: tcp
      description: "HTTP traffic"
    - port: "443"
      protocol: tcp
      description: "HTTPS traffic"
    
  sshKeys:
    - my-ssh-key
---
apiVersion: v1
kind: Secret
metadata:
  name: rke2-token
  namespace: default
type: Opaque
stringData:
  token: "K10abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

```

#### Talos Clusters

For Talos clusters (note: Talos uses machine configs, not cloud-init):

```yaml
apiVersion: autokube.io/v1alpha1
kind: NodePool
metadata:
  name: worker-pool-talos
  namespace: default
spec:
  # Cloud provider - currently supports: hetzner
  provider: hetzner
  
  # Hetzner Cloud specific configuration
  hetznerConfig:
    serverType: cpx22
    location: nbg1
    image: ubuntu-22.04
    network: "network-1"
  
  bootstrap:
    type: talos
    talosConfig:
      controlPlaneEndpoint: "https://talos-control-plane:6443"
      configSecretRef:
        name: talos-machine-config
        key: config
  
  labels:
    cluster-type: talos
```

**Supported Cluster Types:**
- `kubeadm` - Standard Kubernetes with kubeadm (default)
- `k3s` - Lightweight Kubernetes from Rancher
- `rke2` / `rancher` - Rancher Kubernetes Engine 2
- `talos` - Talos Linux immutable OS

**Benefits:**
- ✅ No manual bootstrap token management
- ✅ Automatic token rotation (kubeadm)
- ✅ Multi-cluster support
- ✅ Secure secret management
- ✅ Production-ready cloud-init templates

### Firewall Management

The operator can automatically create and manage **Hetzner Cloud Firewalls** for your node pools. Firewalls are visible in the Hetzner Cloud Console and attached to all servers in the pool.

```yaml
apiVersion: autokube.io/v1alpha1
kind: NodePool
metadata:
  name: web-workers
  namespace: default
spec:
  provider: hetzner
  
  # Hetzner Cloud specific configuration
  hetznerConfig:
    serverType: cpx11
    location: nbg1
    image: ubuntu-22.04
    network: "network-1"  # Network name or ID
  
  minNodes: 1
  maxNodes: 5
  targetNodes: 1
  autoScalingEnabled: true
  scaleUpThreshold: 5
  scaleDownThreshold: 30
  
  
  # Automatic firewall rules
  firewallRules:
    - port: "80"
      protocol: tcp
      description: "HTTP traffic"
    - port: "443"
      protocol: tcp
      description: "HTTPS traffic"
    - port: "9090"
      protocol: tcp
      description: "Prometheus metrics"
    - port: "53"
      protocol: udp
      description: "DNS"
  
  # Automatic cluster joining
  bootstrap:
    type: kubeadm
    autoGenerateToken: true
    kubernetesVersion: "1.29"
  
  # Custom commands after initialization
  runCmd:
    - echo "Setting up monitoring agent..."
    - apt-get install -y node-exporter
    - systemctl enable node-exporter
  
  sshKeys:
    - production-key
```

**Firewall Features:**
- 🔥 **Automatic Creation**: Firewall created on first deployment
- 🔄 **Dynamic Updates**: Rules updated when you change the spec
- 🔗 **Auto-Attachment**: All servers automatically attached
- 🌐 **Portal Visible**: Manage firewalls in Hetzner Console
- 📋 **Rule Naming**: Firewall named `<namespace>-<nodepool>-firewall`

**Supported Protocols:**
- `tcp` - TCP traffic
- `udp` - UDP traffic  
- `icmp` - ICMP (ping)
- `esp` - IPsec ESP
- `gre` - GRE tunnels

**Default Behavior:**
- Rules apply to inbound traffic from anywhere (0.0.0.0/0, ::/0)
- All outbound traffic is allowed
- Firewall is shared across all servers in the node pool
- Deleting the node pool does NOT delete the firewall (manual cleanup needed)

### Private Network Attachment

The operator supports attaching servers to **Hetzner Cloud Private Networks** for secure internal communication between your resources.

```yaml
apiVersion: autokube.io/v1alpha1
kind: NodePool
metadata:
  name: private-workers
  namespace: default
spec:
  provider: hetzner
  
  # Hetzner Cloud specific configuration
  hetznerConfig:
    serverType: cpx11
    location: nbg1
    image: ubuntu-22.04
    # Attach to Hetzner private network
    # Use network name or ID
    network: "network-1" 
  

  bootstrap:
    type: kubeadm
    autoGenerateToken: true
  
  sshKeys:
    - production-key
```

**Network Features:**
- 🔒 **Private Communication**: Servers get private IPs for internal traffic
- 🌐 **Network Isolation**: Separate network per environment (dev/staging/prod)
- 📡 **Dual Stack**: Servers have both public and private IPs
- 🔄 **Automatic Assignment**: Private IP assigned on server creation
- 📋 **Flexible Input**: Accepts network name or numeric ID

**How to Create a Private Network:**

1. **Via Hetzner Cloud Console:**
   - Go to Networks → Create Network
   - Choose IP range (e.g., 10.0.0.0/16)
   - Select location (must match server location)
   - Note the network name or ID

2. **Via CLI:**
   ```bash
   hcloud network create \
     --name my-k8s-network \
     --ip-range 10.0.0.0/16
   
   hcloud network add-subnet my-k8s-network \
     --type cloud \
     --network-zone eu-central \
     --ip-range 10.0.0.0/24
   ```

3. **Use in operator:**
   - Set `network: "my-k8s-network"` in your NodePool spec
   - All servers will automatically join this network
   - Servers can communicate via private IPs (10.0.0.x)

**Best Practices:**
- ✅ Use separate networks for different clusters/environments
- ✅ Plan IP ranges to avoid conflicts (use RFC 1918 ranges)
- ✅ Network must exist in same location as servers
- ✅ Configure firewall rules for private network traffic if needed


- Rules apply to inbound traffic from anywhere (0.0.0.0/0, ::/0)
- All outbound traffic is allowed
- Firewall is shared across all servers in the node pool
- Deleting the node pool does NOT delete the firewall (manual cleanup needed)



Check the status:

```bash
# List all node pools
kubectl get nodepools
kubectl get np  # Short name

# Get detailed information
kubectl describe nodepool worker-pool

# Watch for changes
kubectl get np -w
```

Example output:
```
NAME          SERVERTYPE   LOCATION   MIN   MAX   CURRENT   READY   AGE
worker-pool   cx11         nbg1       2     10    3         3       5m
```

## Configuration Options

### NodePool Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `provider` | string | Yes | hetzner | Cloud provider (currently: hetzner) |
| `hetznerConfig` | object | Yes* | - | Hetzner Cloud configuration (*required when provider is hetzner) |
| `hetznerConfig.serverType` | string | Yes | - | Hetzner server type (cx11, cpx21, ccx13, etc.) |
| `hetznerConfig.location` | string | Yes | - | Hetzner location (nbg1, fsn1, hel1, ash, etc.) |
| `hetznerConfig.image` | string | Yes | - | OS image (ubuntu-22.04, debian-11, etc.) |
| `hetznerConfig.network` | string | No | - | Hetzner private network name or ID |
| `minNodes` | int | No | 1 | Minimum number of nodes |
| `maxNodes` | int | No | 10 | Maximum number of nodes |
| `targetNodes` | int | No | - | Fixed number of nodes (takes priority over auto-scaling) |
| `autoScalingEnabled` | bool | No | true | Enable/disable auto-scaling |
| `scaleUpThreshold` | int | No | 5 | Pending pods to trigger scale up |
| `scaleDownThreshold` | int | No | 30 | CPU % to trigger scale down |
| `cloudInit` | string | No | - | Cloud-init user data (overridden by bootstrap config) |
| `bootstrap` | object | No | - | Automatic cluster joining configuration |
| `firewallRules` | []FirewallRule | No | - | Hetzner Cloud Firewall rules |
| `runCmd` | []string | No | - | Custom commands to run after initialization |
| `sshKeys` | []string | No | - | SSH key names from cloud provider |
| `labels` | map | No | - | Custom labels for cloud resources |
| `firewallRules` | []FirewallRule | No | - | Firewall rules (Hetzner Cloud specific) |

#### FirewallRule Object

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `port` | string | Yes | Port number or range (e.g., "80", "8000-9000") |
| `protocol` | string | Yes | Protocol: tcp, udp, icmp, esp, gre |
| `description` | string | No | Human-readable description |

### Helm Chart Values

Key configuration options for the Helm chart:

```yaml
# Image configuration
image:
  repository: ghcr.io/autokubeio/nodepool
  tag: "0.1.0"
  pullPolicy: IfNotPresent

# Hetzner Cloud token
hcloudToken: ""  # Your token here
# Or use existing secret
hcloudTokenSecret: ""

# Resources
resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 100m
    memory: 128Mi

# High availability
replicaCount: 1
leaderElection:
  enabled: true

# Monitoring
monitoring:
  enabled: true
  serviceMonitor:
    enabled: false  # Enable if you have Prometheus Operator
```

## Building from Source

### Prerequisites

- Go 1.21+
- Docker
- kubectl
- Access to a Kubernetes cluster

### Build the Operator

```bash
# Clone the repository
git clone https://github.com/autokubeio/nodepool.git
cd nodepool

# Download dependencies
go mod download

# Build the binary
make build

# Run tests
make test

# Build Docker image
make docker-build IMG=<your-registry>/nodepool:tag

# Push to registry
make docker-push IMG=<your-registry>/nodepool:tag
```

### Local Development

```bash
# Install CRDs
make install

# Run the operator locally
export HCLOUD_TOKEN=<your-token>
make run

# In another terminal, create a test node pool
kubectl apply -f config/samples/hcloud_v1alpha1_nodepools.yaml
```

## Publishing

### Container Image

Build and push to GitHub Container Registry:

```bash
# Login to GitHub Container Registry
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin

# Build multi-arch image
docker buildx create --use
docker buildx build --platform linux/amd64,linux/arm64 \
  -t ghcr.io/autokubeio/nodepool:latest \
  -t ghcr.io/autokubeio/nodepool:v0.1.0 \
  --push .
```

### Helm Chart

Package and publish the Helm chart:

```bash
# Package the chart
helm package charts/nodepool

# Create Helm repository index
helm repo index --url https://autokubeio.github.io/nodepool/ .

# Commit and push to gh-pages branch
git checkout gh-pages
git add .
git commit -m "Release v0.1.0"
git push origin gh-pages
```

### GitHub Release

```bash
# Create a git tag
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0

# GitHub Actions will automatically:
# - Build and push Docker images
# - Create GitHub release
# - Publish Helm chart
```

## Metrics

The operator exposes Prometheus metrics on port 8080:

- `hcloud_operator_nodepool_size` - Current and ready nodes per pool
- `hcloud_operator_nodepool_scale_ups_total` - Total scale up operations
- `hcloud_operator_nodepool_scale_downs_total` - Total scale down operations
- `hcloud_operator_reconcile_errors_total` - Total reconciliation errors

### Prometheus Configuration

```yaml
apiVersion: v1
kind: ServiceMonitor
metadata:
  name: nodepool
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: nodepool
  endpoints:
  - port: metrics
    interval: 30s
```

## Troubleshooting

### Check operator logs

```bash
kubectl logs -n nodepool-system deployment/nodepool -f
```

### Common Issues

**Operator not starting:**
- Verify HCLOUD_TOKEN is set correctly
- Check RBAC permissions
- Review pod logs

**Nodes not being created:**
- Verify Hetzner Cloud API token has correct permissions
- Check if server type exists in the specified location
- Ensure SSH keys exist in your Hetzner account
- Review operator logs for API errors

**Nodes not joining cluster:**
- Verify cloud-init configuration
- Check network connectivity
- Ensure kubeadm join command is correct

**Scale operations not happening:**
- Verify autoScalingEnabled is true
- Check if current nodes are within min/max range
- Review scaleUpThreshold and scaleDownThreshold values

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Guidelines

- Follow Go best practices and conventions
- Add tests for new features
- Update documentation as needed
- Run `make fmt` and `make lint` before committing

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- [Hetzner Cloud](https://www.hetzner.com/cloud) for their excellent cloud platform
- [Kubebuilder](https://book.kubebuilder.io/) for the operator framework
- The Kubernetes community

## Support

- 📧 Email: [info@autokube.io]
- 🐛 Issues: [GitHub Issues](https://github.com/autokubeio/nodepool/issues)
- 💬 Discussions: [GitHub Discussions](https://github.com/autokubeio/nodepool/discussions)

## Roadmap

### v0.2.0 (Q1 2026)
- [ ] **OVHcloud Public Cloud provider support**
  - European data sovereignty and GDPR compliance
  - Competitive pricing for EU customers
  - Multiple instance types and flavors
  - Private networking (vRack)
  - Anti-DDoS protection included
- [ ] **UpCloud provider support**
  - High-performance MaxIOPS storage
  - Simple and predictable pricing
  - European and US datacenters
  - Private networking (SDN)
  - Fast server provisioning (45 seconds)
- [ ] **DigitalOcean Droplets provider support**
  - Simple API and predictable pricing
  - Flexible Droplet sizes
  - VPC network support
  - Load balancer integration
  - Popular among startups and SMBs
- [ ] Node health checks and auto-replacement
- [ ] Enhanced logging with structured output

### v0.3.0 (Q2 2026)
- [ ] **Scaleway provider support**
  - ARM-based instances (cost-effective)
  - GPU instances for AI/ML workloads
  - European focus (France, Netherlands, Poland)
  - Elastic Metal (bare metal) support
  - Private networks and managed Kubernetes
- [ ] **Linode/Akamai Cloud Compute provider support**
  - Competitive pricing globally
  - NodeBalancer integration
  - VLAN support for private networking
  - Global datacenter presence
- [ ] Load balancer integration across providers
- [ ] Node pool templates and presets
- [ ] Cost optimization mode (prefer cheaper instance types)

### v0.4.0 (Q3 2026)
- [ ] **Vultr Compute provider support**
  - High-performance instances
  - Bare Metal support
  - Global locations (25+ datacenters)
  - Reserved instances for cost savings
- [ ] **Contabo provider support**
  - Extremely competitive pricing
  - High storage capacity options
  - European and US locations
- [ ] Backup and disaster recovery features
- [ ] Multi-zone/multi-region node distribution
- [ ] Advanced auto-scaling strategies (CPU, memory, custom metrics)

### v0.5.0 (Q4 2026)
- [ ] **AWS EC2 provider support**
  - EC2 Auto Scaling Groups integration
  - Spot Instance support for cost optimization
  - Multiple instance types and families
  - EBS volume management
  - Integration with AWS VPC
- [ ] **Azure Virtual Machines provider support**
  - VM Scale Sets integration
  - Spot VMs for cost optimization
  - Managed disk support
  - Azure Virtual Network integration
- [ ] Spot/Preemptible instance support across all providers
- [ ] Cross-cloud cost comparison and recommendations

### v0.6.0 (Q1 2027)
- [ ] **GCP Compute Engine provider support**
  - Instance groups and templates
  - Preemptible VMs for cost savings
  - Custom machine types
  - GPU support for AI/ML workloads
- [ ] Node pool scheduling policies
- [ ] Advanced placement strategies

### Long-term Vision
- [ ] Web UI dashboard for management and monitoring
- [ ] Cost analytics and optimization recommendations
- [ ] Real-time cost tracking per provider
- [ ] Integration with Cluster Autoscaler
- [ ] GitOps integration (ArgoCD, Flux)
- [ ] Cluster-API provider implementation
- [ ] Terraform provider for NodePool resources
- [ ] Support for bare-metal providers (Equinix Metal, Packet)
- [ ] AI-driven scaling predictions based on workload patterns
- [ ] Hybrid cloud support (mix multiple providers in same cluster)
- [ ] Multi-cloud disaster recovery and failover
- [ ] Carbon footprint tracking and green cloud optimization

---

Made with ❤️ by [Mahyar Moghadam](https://github.com/MahyarMoghadam) and the Autokube.io team.
