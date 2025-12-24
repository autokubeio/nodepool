# OVHcloud Provider Setup Guide

This guide explains how to set up and use the OVHcloud provider with the NodePool operator.

## Prerequisites

- OVHcloud Public Cloud account
- OVHcloud project created
- API credentials (Application Key, Application Secret, Consumer Key)
- `kubectl` configured to access your cluster

## Getting OVHcloud API Credentials

### 1. Create API Application

Visit the OVHcloud API token creation page:
- EU: https://eu.api.ovh.com/createToken/
- CA: https://ca.api.ovh.com/createToken/
- US: https://api.us.ovhcloud.com/createToken/

### 2. Set Application Rights

Grant the following rights for your application:

```
GET    /cloud/project/*
POST   /cloud/project/*
PUT    /cloud/project/*
DELETE /cloud/project/*
```

### 3. Save Your Credentials

After creation, you'll receive:
- **Application Key** (AK)
- **Application Secret** (AS)
- **Consumer Key** (CK)

**Keep these credentials secure!**

## Installation

### Using Kubernetes Secrets

1. **Create a secret with your OVHcloud credentials:**

```bash
kubectl create secret generic ovhcloud-credentials \
  --from-literal=endpoint=ovh-eu \
  --from-literal=application-key=YOUR_APPLICATION_KEY \
  --from-literal=application-secret=YOUR_APPLICATION_SECRET \
  --from-literal=consumer-key=YOUR_CONSUMER_KEY \
  --from-literal=project-id=YOUR_PROJECT_ID \
  -n nodepool-system
```

Available endpoints:
- `ovh-eu` - Europe
- `ovh-ca` - Canada
- `ovh-us` - United States

2. **Install the operator with OVHcloud support:**

```bash
helm install nodepool-operator nodepool/nodepool \
  --namespace nodepool-system \
  --set ovhcloud.enabled=true \
  --set ovhcloud.credentialsSecret=ovhcloud-credentials
```

## Configuration

### Finding Flavor IDs

List available flavors in your region:

```bash
curl -X GET \
  -H "X-Ovh-Application: $APPLICATION_KEY" \
  -H "X-Ovh-Consumer: $CONSUMER_KEY" \
  "https://api.ovh.com/1.0/cloud/project/$PROJECT_ID/flavor"
```

Common flavors:
- **s1-2**: 1 vCore, 2GB RAM (shared)
- **s1-4**: 1 vCore, 4GB RAM (shared)
- **b2-7**: 2 vCores, 7GB RAM (balanced)
- **b2-15**: 4 vCores, 15GB RAM (balanced)
- **c2-7**: 2 vCores, 7GB RAM (compute)
- **c2-15**: 4 vCores, 15GB RAM (compute)
- **r2-15**: 2 vCores, 15GB RAM (memory)

### Finding Image IDs

List available images:

```bash
curl -X GET \
  -H "X-Ovh-Application: $APPLICATION_KEY" \
  -H "X-Ovh-Consumer: $CONSUMER_KEY" \
  "https://api.ovh.com/1.0/cloud/project/$PROJECT_ID/image"
```

Common images:
- Ubuntu 22.04
- Ubuntu 20.04
- Debian 11
- Debian 12

### Available Regions

OVHcloud regions:
- **GRA** (Gravelines, France): GRA11, GRA9, GRA7
- **SBG** (Strasbourg, France): SBG5, SBG7
- **BHS** (Beauharnois, Canada): BHS5
- **DE** (Frankfurt, Germany): DE1
- **UK** (London, UK): UK1
- **WAW** (Warsaw, Poland): WAW1
- **SYD** (Sydney, Australia): SYD1
- **SIN** (Singapore): SIN1

## Usage Examples

### Basic NodePool

```yaml
apiVersion: autokube.io/v1alpha1
kind: NodePool
metadata:
  name: worker-pool
  namespace: default
spec:
  provider: ovhcloud
  
  ovhcloudConfig:
    flavorID: s1-4
    region: GRA11
    imageID: "Ubuntu 22.04"
    projectID: "your-project-id"
  
  minNodes: 2
  maxNodes: 10
  autoScalingEnabled: true
```

### With Private Network (vRack)

```yaml
apiVersion: autokube.io/v1alpha1
kind: NodePool
metadata:
  name: private-workers
  namespace: default
spec:
  provider: ovhcloud
  
  ovhcloudConfig:
    flavorID: b2-7
    region: GRA11
    imageID: "Ubuntu 22.04"
    networkID: "your-vrack-network-id"
    projectID: "your-project-id"
  
  minNodes: 1
  maxNodes: 5
  
  bootstrap:
    type: kubeadm
    autoGenerateToken: true
    kubernetesVersion: "1.29"
  
  sshKeys:
    - production-key
```

### With Security Groups

```yaml
apiVersion: autokube.io/v1alpha1
kind: NodePool
metadata:
  name: web-workers
  namespace: default
spec:
  provider: ovhcloud
  
  ovhcloudConfig:
    flavorID: c2-7
    region: SBG5
    imageID: "Ubuntu 22.04"
    projectID: "your-project-id"
  
  minNodes: 2
  maxNodes: 8
  
  firewallRules:
    - port: "80"
      protocol: tcp
      description: "HTTP"
    - port: "443"
      protocol: tcp
      description: "HTTPS"
    - port: "6443"
      protocol: tcp
      description: "Kubernetes API"
  
  bootstrap:
    type: kubeadm
    autoGenerateToken: true
  
  sshKeys:
    - web-server-key
```

## vRack Private Network

### Creating a Private Network

1. **Via OVHcloud Control Panel:**
   - Go to Public Cloud → Network → Private Networks
   - Click "Create a private network"
   - Select vRack
   - Choose VLAN ID and subnet
   - Note the network ID

2. **Via API:**

```bash
curl -X POST \
  -H "X-Ovh-Application: $APPLICATION_KEY" \
  -H "X-Ovh-Consumer: $CONSUMER_KEY" \
  -H "Content-Type: application/json" \
  "https://api.ovh.com/1.0/cloud/project/$PROJECT_ID/network/private" \
  -d '{
    "name": "my-k8s-network",
    "regions": ["GRA11"],
    "vlanId": 42
  }'
```

### Attaching to vRack

Instances automatically join the specified private network when `networkID` is provided in the configuration.

## Troubleshooting

### Check API Connectivity

```bash
curl -X GET \
  -H "X-Ovh-Application: $APPLICATION_KEY" \
  -H "X-Ovh-Consumer: $CONSUMER_KEY" \
  "https://api.ovh.com/1.0/cloud/project/$PROJECT_ID"
```

### View Instance Creation Logs

```bash
kubectl logs -n nodepool-system deployment/nodepool-operator -f
```

### Common Issues

**Instance Creation Fails:**
- Verify flavor ID exists in the selected region
- Check project quota limits
- Ensure image ID is valid
- Verify SSH key exists in your project

**Network Issues:**
- Verify vRack network exists
- Check network is available in the instance region
- Ensure security group rules allow required traffic

**Authentication Errors:**
- Verify API credentials are correct
- Check endpoint matches your region
- Ensure consumer key has required permissions

## Best Practices

- ✅ Use vRack for private communication between nodes
- ✅ Use security groups for firewall management
- ✅ Select regions close to your users for better latency
- ✅ Use appropriate flavor types for your workload
- ✅ Enable auto-scaling for production workloads
- ✅ Use separate projects for different environments
- ✅ Tag instances with meaningful labels

## Cost Optimization

- Use shared instances (s1) for development
- Use balanced instances (b2) for general workloads
- Enable auto-scaling to scale down during low usage
- Use cheaper regions where possible
- Monitor usage with OVHcloud billing dashboard

## Support

For OVHcloud-specific issues:
- OVHcloud Documentation: https://docs.ovh.com/
- OVHcloud Community: https://community.ovh.com/
- API Documentation: https://api.ovh.com/

For operator issues:
- GitHub Issues: https://github.com/autokubeio/nodepool/issues
