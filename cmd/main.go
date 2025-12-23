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
	"flag"
	"os"

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

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&hcloudToken, "hcloud-token", os.Getenv("HCLOUD_TOKEN"),
		"Hetzner Cloud API token (can also be set via HCLOUD_TOKEN environment variable)")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if hcloudToken == "" {
		setupLog.Error(nil, "HCLOUD_TOKEN must be set")
		os.Exit(1)
	}

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
		os.Exit(1)
	}

	// Initialize Hetzner Cloud client
	hcloudClient := hetzner.NewClient(hcloudToken)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector()

	// Initialize Kubernetes client
	kubeConfig := ctrl.GetConfigOrDie()
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		setupLog.Error(err, "unable to create kubernetes client")
		os.Exit(1)
	}

	// Initialize bootstrap manager
	bootstrapManager := bootstrap.NewBootstrapTokenManager(kubeClient)

	// Initialize cloud-init generator
	cloudInitGenerator := bootstrap.NewCloudInitGenerator()

	if err = (&controller.NodePoolReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		HCloudClient:       hcloudClient,
		MetricsClient:      metricsCollector,
		KubeClient:         kubeClient,
		BootstrapManager:   bootstrapManager,
		CloudInitGenerator: cloudInitGenerator,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NodePool")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
