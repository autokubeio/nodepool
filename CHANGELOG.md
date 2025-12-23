# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2024-12-06

### Added
- Initial release of NodePool - Multi-Cloud Kubernetes Autoscaler Operator
- Custom Resource Definition (CRD) for NodePool
- Automatic node provisioning and deprovisioning
- Integration with Hetzner Cloud API
- Cloud-init support for node initialization
- Pod-based autoscaling (scale based on pending pods)
- Graceful node drain before deletion
- Prometheus metrics for monitoring
- Helm chart for easy deployment
- Multi-region support across all Hetzner locations
- Comprehensive documentation and examples

### Features
- Auto-scaling based on cluster load
- Configurable min/max node limits
- SSH key management
- Custom server labels
- Network attachment support
- Health checks and readiness probes
- Leader election for high availability
- RBAC configuration

[Unreleased]: https://github.com/autokubeio/nodepool/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/autokubeio/nodepool/releases/tag/v0.1.0
