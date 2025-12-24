# Contributing to Hetzner Cloud Operator

Thank you for your interest in contributing to the Hetzner Cloud Operator! We welcome contributions from the community.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/nodepool.git`
3. Create a branch: `git checkout -b feature/your-feature-name`
4. Make your changes
5. Run tests: `make test`
6. Commit your changes: `git commit -m "Add some feature"`
7. Push to your fork: `git push origin feature/your-feature-name`
8. Create a Pull Request

## Development Setup

### Prerequisites

- Go 1.21 or later
- Docker
- kubectl
- Access to a Kubernetes cluster (e.g., kind, minikube, or a cloud provider)
- Hetzner Cloud account and API token

### Building and Testing

```bash
# Install dependencies
make deps

# Generate code after API changes
make generate

# Generate CRD manifests
make manifests

# Format code
make fmt

# Run tests
make test

# Build the binary
make build
```

### Usage Examples

**Using environment variable (traditional):**
```bash
export HCLOUD_TOKEN="your-64-char-token"
./manager
```

**Using Kubernetes Secret (recommended):**
```bash
# Create secret
kubectl create secret generic hcloud-credentials \
  --from-literal=token=your-64-char-token \
  -n nodepool-system

# Run with secret
./manager --use-k8s-secret --secret-namespace=nodepool-system
```

**With encryption for cloud-init:**
```bash
export ENCRYPTION_KEY="your-32-byte-encryption-key"
./manager --use-k8s-secret
```

## Code Guidelines

- Follow Go best practices and idioms
- Write meaningful commit messages
- Add tests for new features
- Update documentation as needed
- Run `make fmt` and `make lint` before committing

## Testing

- Unit tests should be added for all new code
- Integration tests should be added for controller logic
- Test with different Kubernetes versions if possible

## Pull Request Process

1. Ensure all tests pass
2. Update the README.md with details of changes if needed
3. Update the CHANGELOG.md with a note describing your changes
4. The PR will be merged once you have the sign-off of at least one maintainer

## Code of Conduct

Please be respectful and constructive in all interactions.

## Questions?

Feel free to open an issue for any questions or concerns.
