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

// Package main is the entry point for the NodePool operator controller manager.
package main

import (
	"context"
	"flag"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	hcloudv1alpha1 "github.com/autokubeio/autokube/api/v1alpha1"
	"github.com/autokubeio/autokube/internal/bootstrap"
	"github.com/autokubeio/autokube/internal/controller"
	"github.com/autokubeio/autokube/internal/hetzner"
	"github.com/autokubeio/autokube/internal/metrics"
	"github.com/autokubeio/autokube/internal/reliability"
	"github.com/autokubeio/autokube/internal/security"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(hcloudv1alpha1.AddToScheme(scheme))
}

//nolint:funlen // Main function coordinates multiple subsystem initializations
func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var hcloudToken string
	var useK8sSecret bool
	var secretNamespace string
	var secretName string
	var encryptionKey string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&hcloudToken, "hcloud-token", os.Getenv("HCLOUD_TOKEN"),
		"Hetzner Cloud API token (can also be set via HCLOUD_TOKEN environment variable)")
	flag.BoolVar(&useK8sSecret, "use-k8s-secret", false,
		"Use Kubernetes Secret for HCLOUD_TOKEN instead of environment variable")
	flag.StringVar(&secretNamespace, "secret-namespace", "default",
		"Namespace for the Kubernetes Secret containing HCLOUD_TOKEN")
	flag.StringVar(&secretName, "secret-name", "hcloud-credentials",
		"Name of the Kubernetes Secret containing HCLOUD_TOKEN")
	flag.StringVar(&encryptionKey, "encryption-key", os.Getenv("ENCRYPTION_KEY"),
		"Encryption key for sensitive data (can also be set via ENCRYPTION_KEY environment variable)")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Validate and retrieve HCLOUD_TOKEN
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tokenValidator := security.NewTokenValidator()

	// Initialize Kubernetes client first (needed for secrets manager)
	kubeConfig := ctrl.GetConfigOrDie()
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		setupLog.Error(err, "unable to create kubernetes client")
		cancel()
		//nolint:gocritic // exitAfterDefer: Early exit is intentional for fatal error
		os.Exit(1)
	}

	// Initialize secrets manager
	var secretsManager *security.SecretsManager
	if encryptionKey != "" {
		secretsManager = security.NewSecretsManager(
			kubeClient,
			secretNamespace,
			security.WithSecretName(secretName),
			security.WithEncryptionKey([]byte(encryptionKey)),
		)
	} else {
		secretsManager = security.NewSecretsManager(
			kubeClient,
			secretNamespace,
			security.WithSecretName(secretName),
		)
	}

	// Get token from K8s secret or environment variable
	if useK8sSecret {
		setupLog.Info("Loading HCLOUD_TOKEN from Kubernetes Secret",
			"namespace", secretNamespace,
			"secret", secretName)

		token, err := secretsManager.GetToken(ctx)
		if err != nil {
			setupLog.Error(err, "Failed to get HCLOUD_TOKEN from Kubernetes Secret",
				"namespace", secretNamespace,
				"secret", secretName,
				"help", "Make sure the secret exists with a 'token' key")
			cancel()
			os.Exit(1)
		}
		hcloudToken = token
	}

	// Validate token
	if hcloudToken == "" {
		setupLog.Error(nil, "HCLOUD_TOKEN must be set",
			"help", "Set HCLOUD_TOKEN environment variable or use --use-k8s-secret flag")
		cancel()
		os.Exit(1)
	}

	// Validate token format and with API
	setupLog.Info("Validating HCLOUD_TOKEN...")
	if err := tokenValidator.Validate(ctx, hcloudToken); err != nil {
		setupLog.Error(err, "HCLOUD_TOKEN validation failed",
			"sanitized_token", tokenValidator.SanitizeToken(hcloudToken),
			"help", "Ensure the token is valid and has not expired")
		cancel()
		os.Exit(1)
	}
	setupLog.Info("HCLOUD_TOKEN validated successfully")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "nodepools.autokube.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		cancel()
		os.Exit(1)
	}

	// Initialize Hetzner Cloud client with circuit breaker
	circuitBreaker := reliability.NewCircuitBreaker(reliability.DefaultCircuitBreakerConfig())
	hcloudClient := hetzner.NewClient(hcloudToken, hetzner.WithCircuitBreaker(circuitBreaker))

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector()

	// Initialize bootstrap manager
	bootstrapManager := bootstrap.NewBootstrapTokenManager(kubeClient)

	// Initialize cloud-init generator with encryption support
	var cloudInitGenerator *bootstrap.CloudInitGenerator
	if encryptionKey != "" {
		cloudInitGenerator = bootstrap.NewCloudInitGenerator(
			bootstrap.WithSecretsManager(secretsManager),
		)
	} else {
		cloudInitGenerator = bootstrap.NewCloudInitGenerator()
	}

	// Initialize dead letter queue for failed operations
	deadLetterQueue := reliability.NewDeadLetterQueue(1000)

	// Add a listener to log failed operations
	deadLetterQueue.AddListener(func(op *reliability.FailedOperation) {
		setupLog.Error(op.Error, "Operation failed and added to dead letter queue",
			"operation_id", op.ID,
			"operation_type", op.OperationType,
			"retry_count", op.RetryCount)
	})

	if err = (&controller.NodePoolReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		HCloudClient:       hcloudClient,
		MetricsClient:      metricsCollector,
		KubeClient:         kubeClient,
		BootstrapManager:   bootstrapManager,
		CloudInitGenerator: cloudInitGenerator,
		DeadLetterQueue:    deadLetterQueue,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NodePool")
		cancel()
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		cancel()
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		cancel()
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		cancel()
		os.Exit(1)
	}
}
