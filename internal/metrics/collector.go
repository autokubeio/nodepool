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

// Package metrics provides Prometheus metrics for NodePool operations.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	nodePoolSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hcloud_operator_nodepool_size",
			Help: "Current size of the node pool",
		},
		[]string{"nodepool", "namespace", "status"},
	)

	nodePoolScaleUps = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hcloud_operator_nodepool_scale_ups_total",
			Help: "Total number of scale up operations",
		},
		[]string{"nodepool", "namespace"},
	)

	nodePoolScaleDowns = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hcloud_operator_nodepool_scale_downs_total",
			Help: "Total number of scale down operations",
		},
		[]string{"nodepool", "namespace"},
	)

	reconcileErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hcloud_operator_reconcile_errors_total",
			Help: "Total number of reconciliation errors",
		},
		[]string{"nodepool", "namespace"},
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		nodePoolSize,
		nodePoolScaleUps,
		nodePoolScaleDowns,
		reconcileErrors,
	)
}

// Collector handles Prometheus metrics collection
type Collector struct{}

// NewCollector creates a new metrics collector
func NewCollector() *Collector {
	return &Collector{}
}

// RecordNodePoolSize records the current size of a node pool
func (c *Collector) RecordNodePoolSize(nodePool, namespace string, current, ready int) {
	nodePoolSize.WithLabelValues(nodePool, namespace, "current").Set(float64(current))
	nodePoolSize.WithLabelValues(nodePool, namespace, "ready").Set(float64(ready))
}

// RecordScaleUp records a scale up operation
func (c *Collector) RecordScaleUp(nodePool, namespace string, count int) {
	nodePoolScaleUps.WithLabelValues(nodePool, namespace).Add(float64(count))
}

// RecordScaleDown records a scale down operation
func (c *Collector) RecordScaleDown(nodePool, namespace string, count int) {
	nodePoolScaleDowns.WithLabelValues(nodePool, namespace).Add(float64(count))
}

// RecordReconcileError records a reconciliation error
func (c *Collector) RecordReconcileError(nodePool, namespace string) {
	reconcileErrors.WithLabelValues(nodePool, namespace).Inc()
}
