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

// Package controller implements the NodePool operator controllers.
package controller

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	hcloudv1alpha1 "github.com/autokubeio/autokube/api/v1alpha1"
	"github.com/autokubeio/autokube/internal/bootstrap"
	"github.com/autokubeio/autokube/internal/hetzner"
	"github.com/autokubeio/autokube/internal/metrics"
	"github.com/autokubeio/autokube/internal/ovhcloud"
	"github.com/autokubeio/autokube/internal/reliability"
)

const (
	reconcileInterval = 30 * time.Second
	nodePoolFinalizer = "autokube.io/finalizer"
	defaultTokenKey   = "token"
)

// NodePoolReconciler reconciles a NodePool object
type NodePoolReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	HCloudClient       hetzner.ClientInterface
	OVHCloudClient     ovhcloud.ClientInterface
	MetricsClient      *metrics.Collector
	KubeClient         kubernetes.Interface
	BootstrapManager   *bootstrap.BootstrapTokenManager
	CloudInitGenerator *bootstrap.CloudInitGenerator
	DeadLetterQueue    *reliability.DeadLetterQueue
}

// +kubebuilder:rbac:groups=autokube.io,resources=nodepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autokube.io,resources=nodepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autokube.io,resources=nodepools/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop
//
//nolint:funlen // Core reconciliation logic requires multiple orchestration steps
func (r *NodePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the NodePool instance
	nodePool := &hcloudv1alpha1.NodePool{}
	if err := r.Get(ctx, req.NamespacedName, nodePool); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("NodePool resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get NodePool")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !nodePool.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, nodePool)
	}

	// Add finalizer if not present
	if !containsString(nodePool.Finalizers, nodePoolFinalizer) {
		nodePool.Finalizers = append(nodePool.Finalizers, nodePoolFinalizer)
		if err := r.Update(ctx, nodePool); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Get current state from cloud provider
	var currentNodes int
	var serverNames []string
	var readyNodes int

	switch nodePool.Spec.Provider {
	case hcloudv1alpha1.CloudProviderHetzner:
		servers, err := r.HCloudClient.ListServers(ctx, nodePool.Name, nodePool.Namespace)
		if err != nil {
			logger.Error(err, "Failed to list servers from Hetzner Cloud")
			r.updateStatus(ctx, nodePool, "Error", err.Error())
			return ctrl.Result{RequeueAfter: reconcileInterval}, err
		}
		currentNodes = len(servers)
		readyNodes = r.countReadyNodes(servers)
		serverNames = r.getServerNames(servers)

	case hcloudv1alpha1.CloudProviderOVHcloud:
		if r.OVHCloudClient == nil {
			err := fmt.Errorf("OVHcloud client not initialized")
			logger.Error(err, "OVHcloud provider selected but client is nil")
			r.updateStatus(ctx, nodePool, "Error", err.Error())
			return ctrl.Result{RequeueAfter: reconcileInterval}, err
		}
		instances, err := r.OVHCloudClient.ListInstances(ctx, nodePool.Name, nodePool.Namespace)
		if err != nil {
			logger.Error(err, "Failed to list instances from OVHcloud")
			r.updateStatus(ctx, nodePool, "Error", err.Error())
			return ctrl.Result{RequeueAfter: reconcileInterval}, err
		}
		currentNodes = len(instances)
		readyNodes = r.countReadyOVHInstances(instances)
		serverNames = r.getOVHInstanceNames(instances)

	default:
		err := fmt.Errorf("unsupported provider: %s", nodePool.Spec.Provider)
		logger.Error(err, "Invalid cloud provider")
		r.updateStatus(ctx, nodePool, "Error", err.Error())
		return ctrl.Result{RequeueAfter: reconcileInterval}, err
	}

	// Update status
	nodePool.Status.CurrentNodes = currentNodes
	nodePool.Status.ReadyNodes = readyNodes
	nodePool.Status.Nodes = serverNames

	// Determine desired number of nodes
	desiredNodes := nodePool.Spec.MinNodes // Default to min nodes

	// If TargetNodes is explicitly set, use it (takes priority)
	if nodePool.Spec.TargetNodes > 0 {
		desiredNodes = nodePool.Spec.TargetNodes
	} else if nodePool.Spec.AutoScalingEnabled {
		// Only use autoscaling if TargetNodes is not set
		desiredNodes = r.calculateDesiredNodes(ctx, nodePool)
	}

	// Enforce min/max constraints
	if desiredNodes < nodePool.Spec.MinNodes {
		desiredNodes = nodePool.Spec.MinNodes
	}
	if desiredNodes > nodePool.Spec.MaxNodes {
		desiredNodes = nodePool.Spec.MaxNodes
	}

	// Scale up if needed
	if currentNodes < desiredNodes {
		nodesToAdd := desiredNodes - currentNodes
		logger.Info("Scaling up", "current", currentNodes, "desired", desiredNodes, "adding", nodesToAdd)

		for i := 0; i < nodesToAdd; i++ {
			if err := r.createServer(ctx, nodePool); err != nil {
				logger.Error(err, "Failed to create server")
				r.updateStatus(ctx, nodePool, "ScaleUpFailed", err.Error())
				return ctrl.Result{RequeueAfter: reconcileInterval}, err
			}
		}

		now := metav1.Now()
		nodePool.Status.LastScaleTime = &now
		r.MetricsClient.RecordScaleUp(nodePool.Name, nodePool.Namespace, nodesToAdd)
	}

	// Scale down if needed
	if currentNodes > desiredNodes {
		nodesToRemove := currentNodes - desiredNodes
		logger.Info("Scaling down", "current", currentNodes, "desired", desiredNodes, "removing", nodesToRemove)

		// Scale down logic is provider-specific
		if err := r.scaleDown(ctx, nodePool, nodesToRemove); err != nil {
			logger.Error(err, "Failed to scale down")
			r.updateStatus(ctx, nodePool, "ScaleDownFailed", err.Error())
			return ctrl.Result{RequeueAfter: reconcileInterval}, err
		}

		now := metav1.Now()
		nodePool.Status.LastScaleTime = &now
		r.MetricsClient.RecordScaleDown(nodePool.Name, nodePool.Namespace, nodesToRemove)
	}

	// Update status
	nodePool.Status.Phase = "Ready"
	if err := r.Status().Update(ctx, nodePool); err != nil {
		logger.Error(err, "Failed to update NodePool status")
		return ctrl.Result{}, err
	}

	// Update metrics
	r.MetricsClient.RecordNodePoolSize(
		nodePool.Name,
		nodePool.Namespace,
		nodePool.Status.CurrentNodes,
		nodePool.Status.ReadyNodes,
	)

	return ctrl.Result{RequeueAfter: reconcileInterval}, nil
}

func (r *NodePoolReconciler) calculateDesiredNodes(ctx context.Context, nodePool *hcloudv1alpha1.NodePool) int {
	logger := log.FromContext(ctx)

	// Count pending pods
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList); err != nil {
		logger.Error(err, "Failed to list pods")
		return nodePool.Status.CurrentNodes
	}

	pendingPods := 0
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodPending {
			pendingPods++
		}
	}

	currentNodes := nodePool.Status.CurrentNodes

	// Scale up if too many pending pods
	if pendingPods >= nodePool.Spec.ScaleUpThreshold {
		return currentNodes + 1
	}

	// Scale down if utilization is low (simplified logic)
	if currentNodes > nodePool.Spec.MinNodes && pendingPods == 0 {
		return currentNodes - 1
	}

	return currentNodes
}

func (r *NodePoolReconciler) createServer(ctx context.Context, nodePool *hcloudv1alpha1.NodePool) error {
	logger := log.FromContext(ctx)

	// Generate a shorter, more readable name with random suffix
	suffix := fmt.Sprintf("%x", time.Now().UnixNano()%0xFFFF) // 4-char hex suffix
	serverName := fmt.Sprintf("%s-%s", nodePool.Name, suffix)

	labels := map[string]string{
		"nodepool":   nodePool.Name,
		"namespace":  nodePool.Namespace,
		"managed-by": "nodepools",
	}
	for k, v := range nodePool.Spec.Labels {
		labels[k] = v
	}

	// Generate cloud-init user data if bootstrap config is provided
	userData := nodePool.Spec.CloudInit
	if nodePool.Spec.Bootstrap != nil && userData == "" {
		var err error
		userData, err = r.generateCloudInit(ctx, nodePool)
		if err != nil {
			return fmt.Errorf("failed to generate cloud-init: %w", err)
		}
		logger.Info("Generated cloud-init for server", "server", serverName, "cloudInitLength", len(userData))
	}

	// Get or create firewall if firewall rules are specified
	var firewallIDs []int64
	if len(nodePool.Spec.FirewallRules) > 0 && nodePool.Spec.Provider == hcloudv1alpha1.CloudProviderHetzner {
		firewallID, err := r.getOrCreateFirewall(ctx, nodePool)
		if err != nil {
			return fmt.Errorf("failed to get or create firewall: %w", err)
		}
		firewallIDs = []int64{firewallID}
		logger.Info("Using firewall for server", "server", serverName, "firewallID", firewallID)
	}

	// Provider-specific server creation
	switch nodePool.Spec.Provider {
	case hcloudv1alpha1.CloudProviderHetzner:
		return r.createHetznerServer(ctx, nodePool, serverName, labels, userData, firewallIDs)
	case hcloudv1alpha1.CloudProviderOVHcloud:
		return r.createOVHcloudInstance(ctx, nodePool, serverName, labels, userData)
	default:
		return fmt.Errorf("unsupported provider: %s", nodePool.Spec.Provider)
	}
}

func (r *NodePoolReconciler) createHetznerServer(ctx context.Context, nodePool *hcloudv1alpha1.NodePool, serverName string, labels map[string]string, userData string, firewallIDs []int64) error {
	logger := log.FromContext(ctx)

	// Get Hetzner configuration
	if nodePool.Spec.HetznerConfig == nil {
		return fmt.Errorf("hetznerConfig is required when provider is hetzner")
	}

	server, err := r.HCloudClient.CreateServer(ctx, hetzner.ServerConfig{
		Name:       serverName,
		ServerType: nodePool.Spec.HetznerConfig.ServerType,
		Image:      nodePool.Spec.HetznerConfig.Image,
		Location:   nodePool.Spec.HetznerConfig.Location,
		SSHKeys:    nodePool.Spec.SSHKeys,
		Labels:     labels,
		UserData:   userData,
		Network:    nodePool.Spec.HetznerConfig.Network,
		Firewalls:  firewallIDs,
	})

	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	logger.Info("Server created successfully", "server", server.Name, "id", server.ID)
	return nil
}

func (r *NodePoolReconciler) createOVHcloudInstance(ctx context.Context, nodePool *hcloudv1alpha1.NodePool, instanceName string, labels map[string]string, userData string) error {
	logger := log.FromContext(ctx)

	// Get OVHcloud configuration
	if nodePool.Spec.OVHcloudConfig == nil {
		return fmt.Errorf("ovhcloudConfig is required when provider is ovhcloud")
	}

	config := nodePool.Spec.OVHcloudConfig

	// Resolve FlavorID from Flavor if needed
	flavorID := config.FlavorID
	if flavorID == "" && config.Flavor != "" {
		resolvedID, err := r.OVHCloudClient.GetFlavorIDByName(ctx, config.Region, config.Flavor)
		if err != nil {
			return fmt.Errorf("failed to resolve flavor name '%s': %w", config.Flavor, err)
		}
		flavorID = resolvedID
		logger.Info("Resolved flavor name to ID", "flavor", config.Flavor, "flavorID", flavorID)
	}
	if flavorID == "" {
		return fmt.Errorf("either flavor or flavorID must be specified")
	}

	// Resolve ImageID from Image if needed
	imageID := config.ImageID
	if imageID == "" && config.Image != "" {
		resolvedID, err := r.OVHCloudClient.GetImageIDByName(ctx, config.Region, config.Image)
		if err != nil {
			return fmt.Errorf("failed to resolve image name '%s': %w", config.Image, err)
		}
		imageID = resolvedID
		logger.Info("Resolved image name to ID", "image", config.Image, "imageID", imageID)
	}
	if imageID == "" {
		return fmt.Errorf("either image or imageID must be specified")
	}

	// Get or create security group if firewall rules are specified
	var securityGroupID string
	if len(nodePool.Spec.FirewallRules) > 0 {
		securityGroup, err := r.getOrCreateOVHSecurityGroup(ctx, nodePool)
		if err != nil {
			return fmt.Errorf("failed to get or create security group: %w", err)
		}
		securityGroupID = securityGroup.ID
		logger.Info("Using security group for instance", "instance", instanceName, "securityGroupID", securityGroupID)
	}

	// Resolve SSH key names to IDs
	var sshKeyIDs []string
	for _, sshKeyName := range nodePool.Spec.SSHKeys {
		if sshKeyName == "" {
			continue
		}
		keyID, err := r.OVHCloudClient.GetSSHKeyIDByName(ctx, sshKeyName)
		if err != nil {
			return fmt.Errorf("failed to resolve SSH key name '%s': %w", sshKeyName, err)
		}
		sshKeyIDs = append(sshKeyIDs, keyID)
		logger.Info("Resolved SSH key name to ID", "sshKeyName", sshKeyName, "sshKeyID", keyID)
	}

	// Resolve NetworkID from Network if needed
	networkID := config.NetworkID
	if networkID == "" && config.Network != "" {
		resolvedID, err := r.OVHCloudClient.GetNetworkIDByName(ctx, config.Region, config.Network)
		if err != nil {
			return fmt.Errorf("failed to resolve network name '%s': %w", config.Network, err)
		}
		networkID = resolvedID
		logger.Info("Resolved network name to ID", "network", config.Network, "networkID", networkID)
	}

	// Create a longer context for instance creation (OVHcloud can take 30-60s)
	createCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	instance, err := r.OVHCloudClient.CreateInstance(createCtx, ovhcloud.InstanceConfig{
		Name:            instanceName,
		FlavorID:        flavorID,
		ImageID:         imageID,
		Region:          config.Region,
		ProjectID:       config.ProjectID,
		NetworkID:       networkID,
		SSHKeys:         sshKeyIDs,
		Labels:          labels,
		UserData:        userData,
		SecurityGroupID: securityGroupID,
	})

	if err != nil {
		return fmt.Errorf("failed to create instance: %w", err)
	}

	logger.Info("Instance created successfully", "instance", instance.Name, "id", instance.ID)
	return nil
}

// generateCloudInit generates cloud-init configuration based on cluster type
//
//nolint:gocyclo,funlen // Multiple bootstrap types require branching logic and configuration
func (r *NodePoolReconciler) generateCloudInit(ctx context.Context, nodePool *hcloudv1alpha1.NodePool) (string, error) {
	logger := log.FromContext(ctx)
	bootstrapConfig := nodePool.Spec.Bootstrap

	switch bootstrapConfig.Type {
	case hcloudv1alpha1.ClusterTypeKubeadm:
		// Generate or get bootstrap token
		var token *bootstrap.BootstrapToken
		var err error
		if bootstrapConfig.AutoGenerateToken {
			token, err = r.BootstrapManager.GetOrGenerateBootstrapToken(ctx, nodePool.Name, 24*time.Hour)
			if err != nil {
				return "", fmt.Errorf("failed to get or generate bootstrap token: %w", err)
			}
			logger.Info("Using bootstrap token", "nodePool", nodePool.Name, "expiresAt", token.ExpiresAt)
		} else if bootstrapConfig.TokenSecretRef != nil {
			// Get token from secret
			var secret corev1.Secret
			secretKey := client.ObjectKey{
				Name:      bootstrapConfig.TokenSecretRef.Name,
				Namespace: nodePool.Namespace,
			}
			if err := r.Get(ctx, secretKey, &secret); err != nil {
				return "", fmt.Errorf("failed to get token secret: %w", err)
			}
			tokenKey := bootstrapConfig.TokenSecretRef.Key
			if tokenKey == "" {
				tokenKey = defaultTokenKey
			}
			tokenValue := string(secret.Data[tokenKey])
			if tokenValue == "" {
				return "", fmt.Errorf("token not found in secret")
			}
			token = &bootstrap.BootstrapToken{
				Token:   tokenValue,
				TokenID: "",
			}
		}

		// Get cluster info
		clusterInfo, err := r.BootstrapManager.GetClusterInfo(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to get cluster info: %w", err)
		}

		// Override endpoint if specified
		if bootstrapConfig.APIServerEndpoint != "" {
			clusterInfo.Endpoint = bootstrapConfig.APIServerEndpoint
		}

		// Get Kubernetes version
		k8sVersion := bootstrapConfig.KubernetesVersion
		if k8sVersion == "" {
			k8sVersion = "1.29" // default version
		}

		// Prepare firewall rules
		var firewallRules []string
		for _, rule := range nodePool.Spec.FirewallRules {
			protocol := rule.Protocol
			if protocol == "" {
				protocol = "tcp"
			}
			firewallRules = append(firewallRules, fmt.Sprintf("%s/%s", rule.Port, protocol))
		}

		cloudInit, err := r.CloudInitGenerator.GenerateKubeadmCloudInitFull(
			clusterInfo.Endpoint,
			token.Token,
			clusterInfo.CACertHash,
			nodePool.Spec.Labels,
			k8sVersion,
			firewallRules,
			nodePool.Spec.RunCmd,
		)
		if err != nil {
			return "", fmt.Errorf("failed to generate kubeadm cloud-init: %w", err)
		}
		return cloudInit, nil

	case hcloudv1alpha1.ClusterTypeK3s:
		if bootstrapConfig.K3sConfig == nil {
			return "", fmt.Errorf("k3s config is required for k3s cluster type")
		}

		// Get token from secret
		var token string
		if bootstrapConfig.K3sConfig.TokenSecretRef != nil {
			var secret corev1.Secret
			secretKey := client.ObjectKey{
				Name:      bootstrapConfig.K3sConfig.TokenSecretRef.Name,
				Namespace: nodePool.Namespace,
			}
			if err := r.Get(ctx, secretKey, &secret); err != nil {
				return "", fmt.Errorf("failed to get k3s token secret: %w", err)
			}
			tokenKey := bootstrapConfig.K3sConfig.TokenSecretRef.Key
			if tokenKey == "" {
				tokenKey = defaultTokenKey
			}
			token = string(secret.Data[tokenKey])
		}

		cloudInit, err := r.CloudInitGenerator.GenerateK3sCloudInit(
			bootstrapConfig.K3sConfig.ServerURL,
			token,
			nodePool.Spec.Labels,
		)
		if err != nil {
			return "", fmt.Errorf("failed to generate k3s cloud-init: %w", err)
		}
		return cloudInit, nil

	case hcloudv1alpha1.ClusterTypeTalos:
		if bootstrapConfig.TalosConfig == nil {
			return "", fmt.Errorf("talos config is required for talos cluster type")
		}

		// Get machine config from secret
		var machineConfig string
		if bootstrapConfig.TalosConfig.ConfigSecretRef != nil {
			var secret corev1.Secret
			secretKey := client.ObjectKey{
				Name:      bootstrapConfig.TalosConfig.ConfigSecretRef.Name,
				Namespace: nodePool.Namespace,
			}
			if err := r.Get(ctx, secretKey, &secret); err != nil {
				return "", fmt.Errorf("failed to get talos config secret: %w", err)
			}
			configKey := bootstrapConfig.TalosConfig.ConfigSecretRef.Key
			if configKey == "" {
				configKey = "config"
			}
			machineConfig = string(secret.Data[configKey])
		}

		cloudInit, err := r.CloudInitGenerator.GenerateTalosCloudInit(
			bootstrapConfig.TalosConfig.ControlPlaneEndpoint,
			machineConfig,
		)
		if err != nil {
			return "", fmt.Errorf("failed to generate talos cloud-init: %w", err)
		}
		return cloudInit, nil

	case hcloudv1alpha1.ClusterTypeRKE2, hcloudv1alpha1.ClusterTypeRancher:
		if bootstrapConfig.RKE2Config == nil {
			return "", fmt.Errorf("rke2 config is required for rke2/rancher cluster type")
		}

		// Get token from secret
		var token string
		if bootstrapConfig.RKE2Config.TokenSecretRef != nil {
			var secret corev1.Secret
			secretKey := client.ObjectKey{
				Name:      bootstrapConfig.RKE2Config.TokenSecretRef.Name,
				Namespace: nodePool.Namespace,
			}
			if err := r.Get(ctx, secretKey, &secret); err != nil {
				return "", fmt.Errorf("failed to get rke2 token secret: %w", err)
			}
			tokenKey := bootstrapConfig.RKE2Config.TokenSecretRef.Key
			if tokenKey == "" {
				tokenKey = defaultTokenKey
			}
			token = string(secret.Data[tokenKey])
		}

		cloudInit, err := r.CloudInitGenerator.GenerateRancherCloudInit(
			bootstrapConfig.RKE2Config.ServerURL,
			token,
			nodePool.Spec.Labels,
		)
		if err != nil {
			return "", fmt.Errorf("failed to generate rke2 cloud-init: %w", err)
		}
		return cloudInit, nil

	default:
		return "", fmt.Errorf("unsupported cluster type: %s", bootstrapConfig.Type)
	}
}

func (r *NodePoolReconciler) deleteServer(
	ctx context.Context,
	_ *hcloudv1alpha1.NodePool,
	server hetzner.Server,
) error {
	logger := log.FromContext(ctx)

	// Drain node before deletion
	if err := r.drainNode(ctx, server.Name); err != nil {
		logger.Error(err, "Failed to drain node, proceeding with deletion anyway", "node", server.Name)
	}

	// Delete node from cluster
	node := &corev1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Name: server.Name}, node); err == nil {
		if err := r.Delete(ctx, node); err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "Failed to delete node from cluster", "node", server.Name)
		} else {
			logger.Info("Node deleted from cluster", "node", server.Name)
		}
	}

	// Delete from Hetzner Cloud
	if err := r.HCloudClient.DeleteServer(ctx, server.ID); err != nil {
		return fmt.Errorf("failed to delete server: %w", err)
	}

	logger.Info("Server deleted successfully", "server", server.Name, "id", server.ID)
	return nil
}

func (r *NodePoolReconciler) drainNode(ctx context.Context, nodeName string) error {
	// Get the node
	node := &corev1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		if errors.IsNotFound(err) {
			return nil // Node already removed
		}
		return err
	}

	// Cordon the node
	node.Spec.Unschedulable = true
	if err := r.Update(ctx, node); err != nil {
		return err
	}

	// Evict all pods (simplified - in production use proper drain logic)
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.MatchingFields{"spec.nodeName": nodeName}); err != nil {
		return err
	}

	for _, pod := range podList.Items {
		pod := pod // Create a copy to avoid implicit memory aliasing
		if err := r.Delete(ctx, &pod); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (r *NodePoolReconciler) handleDeletion(
	ctx context.Context,
	nodePool *hcloudv1alpha1.NodePool,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if containsString(nodePool.Finalizers, nodePoolFinalizer) {
		switch nodePool.Spec.Provider {
		case hcloudv1alpha1.CloudProviderHetzner:
			// Delete all Hetzner servers
			servers, err := r.HCloudClient.ListServers(ctx, nodePool.Name, nodePool.Namespace)
			if err != nil {
				logger.Error(err, "Failed to list servers during deletion")
				return ctrl.Result{}, err
			}

			for _, server := range servers {
				if err := r.deleteServer(ctx, nodePool, server); err != nil {
					logger.Error(err, "Failed to delete server during cleanup", "server", server.Name)
					return ctrl.Result{}, err
				}
			}

		case hcloudv1alpha1.CloudProviderOVHcloud:
			if r.OVHCloudClient == nil {
				logger.Error(nil, "OVHcloud client not initialized")
				return ctrl.Result{}, fmt.Errorf("OVHcloud client not initialized")
			}

			// Delete all OVHcloud instances
			instances, err := r.OVHCloudClient.ListInstances(ctx, nodePool.Name, nodePool.Namespace)
			if err != nil {
				logger.Error(err, "Failed to list instances during deletion")
				return ctrl.Result{}, err
			}

			logger.Info("Deleting OVHcloud instances", "count", len(instances), "nodePool", nodePool.Name)
			for _, instance := range instances {
				if err := r.deleteOVHInstance(ctx, nodePool, instance); err != nil {
					logger.Error(err, "Failed to delete instance during cleanup", "instance", instance.Name, "id", instance.ID)
					return ctrl.Result{}, err
				}
			}

		default:
			logger.Error(nil, "Unsupported provider during deletion", "provider", nodePool.Spec.Provider)
			return ctrl.Result{}, fmt.Errorf("unsupported provider: %s", nodePool.Spec.Provider)
		}

		// Remove finalizer
		nodePool.Finalizers = removeString(nodePool.Finalizers, nodePoolFinalizer)
		if err := r.Update(ctx, nodePool); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *NodePoolReconciler) scaleDown(ctx context.Context, nodePool *hcloudv1alpha1.NodePool, nodesToRemove int) error {
	switch nodePool.Spec.Provider {
	case hcloudv1alpha1.CloudProviderHetzner:
		return r.scaleDownHetzner(ctx, nodePool, nodesToRemove)
	case hcloudv1alpha1.CloudProviderOVHcloud:
		return r.scaleDownOVHcloud(ctx, nodePool, nodesToRemove)
	default:
		return fmt.Errorf("unsupported provider: %s", nodePool.Spec.Provider)
	}
}

func (r *NodePoolReconciler) scaleDownHetzner(ctx context.Context, nodePool *hcloudv1alpha1.NodePool, nodesToRemove int) error {
	logger := log.FromContext(ctx)
	servers, err := r.HCloudClient.ListServers(ctx, nodePool.Name, nodePool.Namespace)
	if err != nil {
		return err
	}

	for i := 0; i < nodesToRemove && i < len(servers); i++ {
		if err := r.deleteServer(ctx, nodePool, servers[i]); err != nil {
			logger.Error(err, "Failed to delete server")
			return err
		}
	}
	return nil
}

func (r *NodePoolReconciler) scaleDownOVHcloud(ctx context.Context, nodePool *hcloudv1alpha1.NodePool, nodesToRemove int) error {
	logger := log.FromContext(ctx)
	instances, err := r.OVHCloudClient.ListInstances(ctx, nodePool.Name, nodePool.Namespace)
	if err != nil {
		return err
	}

	for i := 0; i < nodesToRemove && i < len(instances); i++ {
		if err := r.deleteOVHInstance(ctx, nodePool, instances[i]); err != nil {
			logger.Error(err, "Failed to delete instance")
			return err
		}
	}
	return nil
}

func (r *NodePoolReconciler) deleteOVHInstance(ctx context.Context, nodePool *hcloudv1alpha1.NodePool, instance ovhcloud.Instance) error {
	logger := log.FromContext(ctx)

	// Drain node before deletion
	if err := r.drainNode(ctx, instance.Name); err != nil {
		logger.Error(err, "Failed to drain node, proceeding with deletion anyway", "node", instance.Name)
	}

	// Delete node from cluster
	node := &corev1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Name: instance.Name}, node); err == nil {
		if err := r.Delete(ctx, node); err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "Failed to delete node from cluster", "node", instance.Name)
		} else {
			logger.Info("Node deleted from cluster", "node", instance.Name)
		}
	}

	// Delete the instance
	if err := r.OVHCloudClient.DeleteInstance(ctx, instance.ID); err != nil {
		return fmt.Errorf("failed to delete instance %s: %w", instance.ID, err)
	}

	logger.Info("Instance deleted successfully", "instance", instance.Name, "id", instance.ID)
	return nil
}

func (r *NodePoolReconciler) getOrCreateOVHSecurityGroup(ctx context.Context, nodePool *hcloudv1alpha1.NodePool) (*ovhcloud.SecurityGroup, error) {
	securityGroupName := fmt.Sprintf("%s-%s", nodePool.Namespace, nodePool.Name)

	// Convert firewall rules to OVHcloud security group rules
	rules := make([]ovhcloud.SecurityRule, 0, len(nodePool.Spec.FirewallRules))
	for _, rule := range nodePool.Spec.FirewallRules {
		// Parse port (assuming single port for now, not ranges)
		var port int
		fmt.Sscanf(rule.Port, "%d", &port)

		rules = append(rules, ovhcloud.SecurityRule{
			Direction:  ovhcloud.DirectionIngress,
			Protocol:   rule.Protocol,
			PortFrom:   port,
			PortTo:     port,
			SourceCIDR: "0.0.0.0/0", // Allow from any source
		})
	}

	return r.OVHCloudClient.GetOrCreateSecurityGroup(ctx, securityGroupName, rules)
}

func (r *NodePoolReconciler) countReadyOVHInstances(instances []ovhcloud.Instance) int {
	ready := 0
	for _, instance := range instances {
		if instance.Status == "ACTIVE" {
			ready++
		}
	}
	return ready
}

func (r *NodePoolReconciler) getOVHInstanceNames(instances []ovhcloud.Instance) []string {
	names := make([]string, len(instances))
	for i, instance := range instances {
		names[i] = instance.Name
	}
	return names
}

func (r *NodePoolReconciler) updateStatus(
	ctx context.Context,
	nodePool *hcloudv1alpha1.NodePool,
	phase, message string,
) {
	nodePool.Status.Phase = phase
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             phase,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
	nodePool.Status.Conditions = append(nodePool.Status.Conditions, condition)
	_ = r.Status().Update(ctx, nodePool)
}

func (r *NodePoolReconciler) countReadyNodes(servers []hetzner.Server) int {
	ready := 0
	for _, server := range servers {
		if server.Status == "running" {
			ready++
		}
	}
	return ready
}

func (r *NodePoolReconciler) getOrCreateFirewall(
	ctx context.Context,
	nodePool *hcloudv1alpha1.NodePool,
) (int64, error) {
	firewallName := fmt.Sprintf("%s-firewall", nodePool.Name)

	// Convert spec firewall rules to Hetzner firewall rules
	var rules []hcloud.FirewallRule
	for _, rule := range nodePool.Spec.FirewallRules {
		protocol := hcloud.FirewallRuleProtocol(rule.Protocol)

		// Validate protocol
		if protocol != hcloud.FirewallRuleProtocolTCP &&
			protocol != hcloud.FirewallRuleProtocolUDP &&
			protocol != hcloud.FirewallRuleProtocolICMP &&
			protocol != hcloud.FirewallRuleProtocolESP &&
			protocol != hcloud.FirewallRuleProtocolGRE {
			protocol = hcloud.FirewallRuleProtocolTCP // default to TCP
		}

		// Create rule for ingress from any source
		rules = append(rules, hcloud.FirewallRule{
			Direction: hcloud.FirewallRuleDirectionIn,
			SourceIPs: []net.IPNet{
				{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)},  // 0.0.0.0/0
				{IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}, // ::/0
			},
			Protocol: protocol,
			Port:     hcloud.Ptr(rule.Port),
		})
	}

	firewall, err := r.HCloudClient.GetOrCreateFirewall(ctx, firewallName, rules)
	if err != nil {
		return 0, fmt.Errorf("failed to get or create firewall: %w", err)
	}

	return firewall.ID, nil
}

func (r *NodePoolReconciler) getServerNames(servers []hetzner.Server) []string {
	names := make([]string, len(servers))
	for i, server := range servers {
		names[i] = server.Name
	}
	return names
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	result := []string{}
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hcloudv1alpha1.NodePool{}).
		Complete(r)
}
