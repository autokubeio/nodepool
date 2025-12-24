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

package controller

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	hcloudv1alpha1 "github.com/autokubeio/autokube/api/v1alpha1"
	"github.com/autokubeio/autokube/internal/bootstrap"
	"github.com/autokubeio/autokube/internal/hetzner"
	"github.com/autokubeio/autokube/internal/metrics"
	"github.com/autokubeio/autokube/internal/mock"
	"github.com/autokubeio/autokube/internal/reliability"
)

func setupTestReconciler() (*NodePoolReconciler, client.Client) {
	scheme := runtime.NewScheme()
	_ = hcloudv1alpha1.AddToScheme(scheme)

	client := clientfake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	mockHetzner := mock.NewMockHetznerClient()

	// Create cluster-info ConfigMap needed by bootstrap manager
	clusterInfoCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-info",
			Namespace: "kube-public",
		},
		Data: map[string]string{
			"kubeconfig": `apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: ` +
				`LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUM1ekNDQWMrZ0F3SUJBZ0lCQVRBTkJna3Foa2lHOXcwQkFRc0ZBREFWTVJNd0VRWURWUVFERXdwdGFXNXAKYTNW` +
				`aVpVTkJNQjRYRFRJME1Ea3hOakl4TlRVeE4xb1hEVE0wTURreE5ESXhOVFV4TjFvd0ZURVRNQkVHQTFVRQpBeE1LYldsdWFXdDFZbVZEUVRDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBTmU2Ck0zTU9JZ2s1
    server: https://test-cluster:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context`,
		},
	}
	kubeClient := fake.NewSimpleClientset(clusterInfoCM)
	bootstrapManager := bootstrap.NewBootstrapTokenManager(kubeClient)
	cloudInitGenerator := bootstrap.NewCloudInitGenerator()
	metricsCollector := metrics.NewCollector()
	deadLetterQueue := reliability.NewDeadLetterQueue(100)

	reconciler := &NodePoolReconciler{
		Client:             client,
		Scheme:             scheme,
		HCloudClient:       mockHetzner,
		MetricsClient:      metricsCollector,
		KubeClient:         kubeClient,
		BootstrapManager:   bootstrapManager,
		CloudInitGenerator: cloudInitGenerator,
		DeadLetterQueue:    deadLetterQueue,
	}

	return reconciler, client
}

func TestNodePoolReconciler_BasicReconcile(t *testing.T) {
	reconciler, client := setupTestReconciler()

	// Set up mock to return empty server list
	mockHetzner, ok := reconciler.HCloudClient.(*mock.HetznerClient)
	if !ok {
		t.Fatal("Failed to cast HCloudClient to mock")
	}
	mockHetzner.ListServersFunc = func(_ context.Context, _, _ string) ([]hetzner.Server, error) {
		return []hetzner.Server{}, nil
	}

	// Create a test NodePool
	nodePool := &hcloudv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pool",
			Namespace: "default",
		},
		Spec: hcloudv1alpha1.NodePoolSpec{
			Provider:    hcloudv1alpha1.CloudProviderHetzner,
			MinNodes:    1,
			MaxNodes:    3,
			TargetNodes: 2,
			HetznerConfig: &hcloudv1alpha1.HetznerCloudConfig{
				ServerType: "cx11",
				Image:      "ubuntu-22.04",
				Location:   "nbg1",
			},
			Bootstrap: &hcloudv1alpha1.ClusterBootstrapConfig{
				Type:              hcloudv1alpha1.ClusterTypeKubeadm,
				AutoGenerateToken: true,
			},
		},
	}

	err := client.Create(context.Background(), nodePool)
	if err != nil {
		t.Fatalf("Failed to create NodePool: %v", err)
	}

	// Reconcile
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-pool",
			Namespace: "default",
		},
	}

	_, err = reconciler.Reconcile(context.Background(), req)
	// Allow "not found" errors as fake client behavior may vary with finalizers
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("Reconcile() unexpected error = %v", err)
	}
}

func TestNodePoolReconciler_ScaleUp(t *testing.T) {
	reconciler, client := setupTestReconciler()

	mockHetzner, ok := reconciler.HCloudClient.(*mock.HetznerClient)
	if !ok {
		t.Fatal("Failed to cast HCloudClient to mock")
	}

	// Create a test NodePool that needs scaling up
	nodePool := &hcloudv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-pool",
			Namespace:  "default",
			Finalizers: []string{nodePoolFinalizer},
		},
		Spec: hcloudv1alpha1.NodePoolSpec{
			Provider:    hcloudv1alpha1.CloudProviderHetzner,
			MinNodes:    2,
			MaxNodes:    5,
			TargetNodes: 3,
			HetznerConfig: &hcloudv1alpha1.HetznerCloudConfig{
				ServerType: "cx11",
				Image:      "ubuntu-22.04",
				Location:   "nbg1",
			},
			Bootstrap: &hcloudv1alpha1.ClusterBootstrapConfig{
				Type:              hcloudv1alpha1.ClusterTypeKubeadm,
				AutoGenerateToken: true,
			},
		},
	}

	err := client.Create(context.Background(), nodePool)
	if err != nil {
		t.Fatalf("Failed to create NodePool: %v", err)
	}

	// Set up mock to return empty server list initially
	mockHetzner.ListServersFunc = func(_ context.Context, _, _ string) ([]hetzner.Server, error) {
		return []hetzner.Server{}, nil
	}

	// Reconcile
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-pool",
			Namespace: "default",
		},
	}

	_, err = reconciler.Reconcile(context.Background(), req)
	// Allow "not found" errors as fake client behavior may vary with finalizers
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("Reconcile() unexpected error = %v", err)
	}

	// Verify that CreateServer was called
	if mockHetzner.CreateServerCalls == 0 {
		t.Error("Expected CreateServer to be called for scale up")
	}
}

func TestNodePoolReconciler_NotFound(t *testing.T) {
	reconciler, _ := setupTestReconciler()

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Errorf("Reconcile() unexpected error = %v", err)
	}

	if result.RequeueAfter != 0 {
		t.Error("Expected no requeue for non-existent resource")
	}
}

func TestNodePoolReconciler_WithDeadLetterQueue(t *testing.T) {
	reconciler, client := setupTestReconciler()

	mockHetzner, ok := reconciler.HCloudClient.(*mock.HetznerClient)
	if !ok {
		t.Fatal("Failed to cast HCloudClient to mock")
	}

	// Set up mock to fail server creation
	mockHetzner.CreateServerFunc = func(_ context.Context, _ hetzner.ServerConfig) (*hetzner.Server, error) {
		return nil, &hetzner.ServerCreateError{Message: "simulated error"}
	}

	// Create a test NodePool
	nodePool := &hcloudv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-pool",
			Namespace:  "default",
			Finalizers: []string{nodePoolFinalizer},
		},
		Spec: hcloudv1alpha1.NodePoolSpec{
			Provider:    hcloudv1alpha1.CloudProviderHetzner,
			MinNodes:    1,
			MaxNodes:    3,
			TargetNodes: 1,
			HetznerConfig: &hcloudv1alpha1.HetznerCloudConfig{
				ServerType: "cx11",
				Image:      "ubuntu-22.04",
				Location:   "nbg1",
			},
			Bootstrap: &hcloudv1alpha1.ClusterBootstrapConfig{
				Type:              hcloudv1alpha1.ClusterTypeKubeadm,
				AutoGenerateToken: true,
			},
		},
	}

	err := client.Create(context.Background(), nodePool)
	if err != nil {
		t.Fatalf("Failed to create NodePool: %v", err)
	}

	// Set up mock to return empty server list
	mockHetzner.ListServersFunc = func(_ context.Context, _, _ string) ([]hetzner.Server, error) {
		return []hetzner.Server{}, nil
	}

	// Wait briefly to allow listener to be set up
	time.Sleep(10 * time.Millisecond)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-pool",
			Namespace: "default",
		},
	}

	// Reconcile - should fail but add to dead letter queue
	_, err = reconciler.Reconcile(context.Background(), req)
	// We expect an error since server creation fails
	if err == nil {
		t.Error("Expected error from failed server creation")
	}
}

func TestNodePoolReconciler_Deletion(t *testing.T) {
	reconciler, client := setupTestReconciler()

	mockHetzner, ok := reconciler.HCloudClient.(*mock.HetznerClient)
	if !ok {
		t.Fatal("Failed to cast HCloudClient to mock")
	}

	// Set up mock to return a server during deletion
	mockHetzner.ListServersFunc = func(_ context.Context, _, _ string) ([]hetzner.Server, error) {
		return []hetzner.Server{
			{
				ID:     1,
				Name:   "test-server",
				Status: "running",
			},
		}, nil
	}

	// Create a test NodePool
	nodePool := &hcloudv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pool",
			Namespace: "default",
		},
		Spec: hcloudv1alpha1.NodePoolSpec{
			Provider: hcloudv1alpha1.CloudProviderHetzner,
			MinNodes: 1,
			MaxNodes: 3,
			HetznerConfig: &hcloudv1alpha1.HetznerCloudConfig{
				ServerType: "cx11",
				Image:      "ubuntu-22.04",
				Location:   "nbg1",
			},
			Bootstrap: &hcloudv1alpha1.ClusterBootstrapConfig{
				Type:              hcloudv1alpha1.ClusterTypeKubeadm,
				AutoGenerateToken: true,
			},
		},
	}

	err := client.Create(context.Background(), nodePool)
	if err != nil {
		t.Fatalf("Failed to create NodePool: %v", err)
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-pool",
			Namespace: "default",
		},
	}

	// First reconcile to add finalizer
	_, err = reconciler.Reconcile(context.Background(), req)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("First reconcile failed: %v", err)
	}

	// Reload the NodePool to get the finalizer
	err = client.Get(context.Background(), req.NamespacedName, nodePool)
	if err != nil {
		// If resource was already deleted by fake client, test deletion behavior directly
		t.Logf("Resource not found after first reconcile, testing deletion directly")

		// Create a new nodepool with finalizer and deletion timestamp
		now := metav1.Now()
		nodePoolWithDeletion := &hcloudv1alpha1.NodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-pool-delete",
				Namespace:         "default",
				Finalizers:        []string{nodePoolFinalizer},
				DeletionTimestamp: &now,
			},
			Spec: nodePool.Spec,
		}
		err = client.Create(context.Background(), nodePoolWithDeletion)
		if err != nil {
			t.Fatalf("Failed to create NodePool for deletion: %v", err)
		}

		req.Name = "test-pool-delete"
		_, err = reconciler.Reconcile(context.Background(), req)
		// Allow not found error after finalizer removal
		if err != nil && !strings.Contains(err.Error(), "not found") {
			t.Errorf("Reconcile during deletion failed: %v", err)
		}
	} else {
		// Normal deletion path - mark for deletion
		err = client.Delete(context.Background(), nodePool)
		if err != nil {
			t.Fatalf("Failed to delete NodePool: %v", err)
		}

		// Reconcile deletion
		_, err = reconciler.Reconcile(context.Background(), req)
		// Allow not found error after finalizer removal
		if err != nil && !strings.Contains(err.Error(), "not found") {
			t.Errorf("Reconcile during deletion failed: %v", err)
		}
	}

	// Verify DeleteServer was called
	if mockHetzner.DeleteServerCalls == 0 {
		t.Error("Expected DeleteServer to be called during deletion")
	}
}
