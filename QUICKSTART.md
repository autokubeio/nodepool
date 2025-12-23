# Quick Start Guide

This guide will help you get started with NodePool - Multi-Cloud Kubernetes Autoscaler Operator quickly.

## Prerequisites

- Kubernetes cluster (v1.19+)
- `kubectl` configured
- Helm 3.x installed
- Cloud provider account with API token (currently supports Hetzner Cloud)

## Step 1: Get Your Hetzner Cloud API Token

1. Log in to [Hetzner Cloud Console](https://console.hetzner.cloud/)
2. Go to your project
3. Navigate to Security → API Tokens
4. Generate a new token with Read & Write permissions
5. Save the token securely

## Step 2: Install the Operator

### Option A: Using Helm (Recommended)

```bash
# Add Helm repository
helm repo add nodepool https://autokubeio.github.io/nodepool
helm repo update

# Install the operator
helm install nodepool nodepool/nodepool \
  --namespace hcloud-system \
  --create-namespace \
  --set hcloudToken=<YOUR_HCLOUD_TOKEN>
```

### Option B: From Source

```bash
# Clone the repository
git clone https://github.com/autokubeio/nodepool.git
cd nodepool

# Install CRD
kubectl apply -f charts/nodepool/templates/crd.yaml

# Create namespace and secret
kubectl create namespace hcloud-system
kubectl create secret generic hcloud-token \
  --from-literal=token=<YOUR_HCLOUD_TOKEN> \
  -n hcloud-system

# Install via Helm from local chart
helm install nodepool ./charts/nodepool \
  --namespace hcloud-system \
  --set hcloudTokenSecret=hcloud-token
```

## Step 3: Verify Installation

```bash
# Check if the operator is running
kubectl get pods -n hcloud-system

# Check if CRD is installed
kubectl get crd nodepools.hcloud.autokube.io

# Expected output:
# NAME                                  CREATED AT
# nodepools.hcloud.autokube.io    2024-12-06T...
```

## Step 4: Create Your First Node Pool

### Prepare SSH Key (Optional)

If you want SSH access to your nodes:

1. Go to Hetzner Cloud Console → Security → SSH Keys
2. Add your SSH public key or note the name of an existing key

### Create Node Pool

Create a file `my-nodepool.yaml`:

```yaml
apiVersion: hcloud.autokube.io/v1alpha1
kind: NodePool
metadata:
  name: worker-nodes
  namespace: default
spec:
  serverType: cx11          # Small server for testing
  location: nbg1            # Nuremberg datacenter
  image: ubuntu-22.04       # Ubuntu 22.04 LTS
  minNodes: 1               # Start with 1 node
  maxNodes: 3               # Scale up to 3 nodes
  autoScalingEnabled: true
  scaleUpThreshold: 3       # Add node when 3+ pods pending
  sshKeys:
    - my-key                # Replace with your SSH key name
```

Apply it:

```bash
kubectl apply -f my-nodepool.yaml
```

## Step 5: Monitor Your Node Pool

```bash
# Watch the node pool status
kubectl get nodepools -w

# Get detailed information
kubectl describe hcloudnodepool worker-nodes

# Check operator logs
kubectl logs -n hcloud-system -l app.kubernetes.io/name=nodepool -f
```

You should see output like:

```
NAME           SERVERTYPE   LOCATION   MIN   MAX   CURRENT   READY   AGE
worker-nodes   cx11         nbg1       1     3     1         1       2m
```

## Step 6: Test Auto-Scaling

Create some test pods to trigger scaling:

```bash
# Create a deployment with many replicas
kubectl create deployment test-nodepool --image=nginx:alpine --replicas=10

# Watch the node pool nodepool up
kubectl get nodepools -w

# Clean up
kubectl delete deployment test-nodepool
```

## Step 7: Check Prometheus Metrics (Optional)

If you have Prometheus installed:

```bash
# Port-forward to the metrics endpoint
kubectl port-forward -n hcloud-system svc/nodepool-metrics 8080:8080

# In another terminal, check metrics
curl http://localhost:8080/metrics | grep hcloud_operator
```

## Common Commands

```bash
# List all node pools
kubectl get np -A

# Scale a node pool manually
kubectl patch hcloudnodepool worker-nodes \
  --type='merge' \
  -p '{"spec":{"targetNodes":5}}'

# Disable auto-scaling
kubectl patch hcloudnodepool worker-nodes \
  --type='merge' \
  -p '{"spec":{"autoScalingEnabled":false}}'

# Delete a node pool (this will delete all servers!)
kubectl delete hcloudnodepool worker-nodes
```

## Troubleshooting

### Operator Not Starting

```bash
# Check pod status
kubectl get pods -n hcloud-system

# Check logs
kubectl logs -n hcloud-system deployment/nodepool

# Common issues:
# - Invalid HCLOUD_TOKEN
# - RBAC permissions not configured
# - CRD not installed
```

### Nodes Not Creating

```bash
# Check operator logs for errors
kubectl logs -n hcloud-system deployment/nodepool | grep -i error

# Verify token has correct permissions
# Verify server type exists in location
# Check Hetzner Cloud Console for any quota limits
```

### Nodes Not Joining Cluster

If you're using cloud-init to join nodes to your cluster:

- Verify cloud-init script is correct
- Check node has network connectivity
- Verify kubeadm join token is valid
- Check security group/firewall rules

## Next Steps

- Read the [full documentation](README.md)
- Explore [advanced examples](config/samples/)
- Set up [Prometheus monitoring](README.md#metrics)
- Join our [community discussions](https://github.com/autokubeio/nodepool/discussions)

## Getting Help

- 📖 [Full Documentation](README.md)
- 🐛 [Report Issues](https://github.com/autokubeio/nodepool/issues)
- 💬 [Discussions](https://github.com/autokubeio/nodepool/discussions)

## Clean Up

To completely remove the operator:

```bash
# Delete all node pools first
kubectl delete nodepools --all -A

# Uninstall the operator
helm uninstall nodepool -n hcloud-system

# Remove CRD (optional)
kubectl delete crd nodepools.hcloud.autokube.io

# Remove namespace
kubectl delete namespace hcloud-system
```

---

Happy scaling! 🚀
