# Development Workflow

This document describes the standard development workflow for working with the NodePool CRD.

## Generating Code and Manifests

After making changes to the API types in `api/v1alpha1/`, you need to regenerate the code and CRD manifests.

### Generate DeepCopy Methods

```bash
make generate
```

This runs `controller-gen` to generate:
- `zz_generated.deepcopy.go` files with DeepCopy, DeepCopyInto, and DeepCopyObject methods
- Required for all Kubernetes API types

### Generate CRD Manifests

```bash
make manifests
```

This runs `controller-gen` to generate:
- CRD manifests in `config/crd/bases/`
- RBAC manifests (if rbac markers are present)
- Webhook configurations (if webhook markers are present)

### Combined Workflow

After modifying API types:

```bash
# 1. Generate code
make generate

# 2. Generate manifests
make manifests

# 3. Apply CRD to your cluster (for testing)
kubectl apply -f config/crd/bases/autokube.io_nodepools.yaml

# 4. Test with example files
kubectl apply -f examples/nodepool-kubeadm.yaml --dry-run=server
```

## API Type Annotations

The code generation uses kubebuilder markers (special comments) to control the output:

### Common CRD Markers

```go
// Field validation
// +kubebuilder:validation:Required
// +kubebuilder:validation:Optional
// +kubebuilder:validation:Enum=value1;value2;value3
// +kubebuilder:validation:Minimum=0
// +kubebuilder:validation:Maximum=100
// +kubebuilder:validation:Pattern=^[a-z]+$
// +kubebuilder:default=defaultValue

// Field description (appears in CRD)
// Description text here

// Type-level markers
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=np
// +kubebuilder:printcolumn:name="ColumnName",type=string,JSONPath=`.spec.field`
```

### Example from NodePool

```go
// NodePoolSpec defines the desired state of NodePool
type NodePoolSpec struct {
	// Provider is the cloud provider (e.g., hetzner)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=hetzner
	// +kubebuilder:default=hetzner
	Provider CloudProvider `json:"provider"`
	
	// HetznerConfig contains Hetzner Cloud specific configuration
	// Required when provider is "hetzner"
	// +optional
	HetznerConfig *HetznerCloudConfig `json:"hetznerConfig,omitempty"`
	
	// MinNodes is the minimum number of nodes in the pool
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	MinNodes int `json:"minNodes"`
}
```

## Tools

### controller-gen

The Makefile automatically downloads and manages `controller-gen`:

```bash
# Check version
./bin/controller-gen --version

# Manual run (if needed)
./bin/controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."
./bin/controller-gen rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
```

Version is defined in Makefile:
```makefile
CONTROLLER_TOOLS_VERSION ?= v0.16.5
```

### Binary Location

Generated tools are stored in `./bin/`:
- `./bin/controller-gen` - CRD and code generator
- `./bin/setup-envtest` - For running tests with envtest

This keeps tools version-locked per project and avoids global installation conflicts.

## Updating CRD in Production

### Option 1: Direct kubectl apply

```bash
kubectl apply -f config/crd/bases/hcloud.mahyar.net_hcloudnodepools.yaml
```

### Option 2: Via Helm Chart

If you're using the Helm chart, sync the generated CRD:

```bash
# Copy spec section from generated CRD to Helm template
# Then upgrade the Helm release
helm upgrade nodepool ./charts/nodepool --namespace hcloud-system
```

## Validation

### Validate YAML Files

```bash
# Server-side validation (checks against cluster CRD)
kubectl apply -f examples/nodepool-kubeadm.yaml --dry-run=server

# Client-side validation (basic YAML syntax)
kubectl apply -f examples/nodepool-kubeadm.yaml --dry-run=client
```

### Check CRD Schema

```bash
# View CRD schema
kubectl get crd nodepools.autokube.io -o yaml

# Check specific field validation
kubectl get crd nodepools.autokube.io -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.provider}'
```

## Common Issues

### "unknown field" errors

If you add new fields to the API but get "unknown field" errors:

1. Run `make manifests` to regenerate CRD
2. Apply the updated CRD: `kubectl apply -f config/crd/bases/...`
3. Retry your operation

### "missing required field" errors

Check your kubebuilder markers:
- Use `+kubebuilder:validation:Required` for required fields
- Use `+optional` for optional fields
- Required fields are also defined in the `required:` section of the CRD

### VS Code schema validation issues

VS Code caches CRD schemas. If changes don't appear:

1. Apply updated CRD to cluster
2. Reload VS Code window (Cmd+Shift+P → "Developer: Reload Window")
3. Or restart the Kubernetes extension

## Best Practices

1. **Always run `make generate` and `make manifests` after API changes**
2. **Test with --dry-run=server before applying to production**
3. **Keep CRD in sync between `config/crd/bases/` and `charts/nodepool/templates/crd.yaml`**
4. **Use descriptive comments** - they become field descriptions in the CRD
5. **Version your APIs** - breaking changes should bump the API version (v1alpha1 → v1alpha2 → v1beta1 → v1)

## Reference

- [Kubebuilder Markers](https://book.kubebuilder.io/reference/markers.html)
- [controller-gen CLI](https://book.kubebuilder.io/reference/controller-gen.html)
- [API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
