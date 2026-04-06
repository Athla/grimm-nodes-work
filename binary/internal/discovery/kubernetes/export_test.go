package kubernetes

// Test seams. These aliases expose package internals only to the blackbox
// `kubernetes_test` package. Kept in this file so the production surface stays
// minimal — anything exported here is explicitly test-only.

import (
	"k8s.io/client-go/kubernetes"
)

// NewWithClient constructs a Discovery bound to a user-supplied clientset
// (typically fake.NewSimpleClientset in tests). Exposed so blackbox tests can
// drive Discover()/Watch() without real kubeconfig.
func NewWithClient(client kubernetes.Interface, cfg Config) *Discovery {
	return newWithClient(client, cfg)
}

// Test-only aliases for internal primitives that have no public surface.
// They are pure helpers with many table-driven cases; exercising them through
// Discover() would obscure the contract being tested.

type AppsWorkload = appsWorkload

var (
	PodHealth                 = podHealth
	WorkloadHealth            = workloadHealth
	ServiceHealth             = serviceHealth
	AggregateHealth           = aggregateHealth
	PodMatchesServiceSelector = podMatchesServiceSelector
)
