package kubernetes

import (
	corev1 "k8s.io/api/core/v1"

	"binary/internal/graph/health"
)

// podHealth maps a Pod's phase + container readiness to health.Status.
// Running + all containers ready → Healthy; Running with any container
// not ready → Degraded; Pending/Failed (including CrashLoopBackOff) → Unhealthy.
func podHealth(p corev1.Pod) health.Status {
	switch p.Status.Phase {
	case corev1.PodRunning:
		if allContainersReady(p) {
			return health.Healthy
		}
		return health.Degraded
	case corev1.PodSucceeded:
		return health.Healthy
	case corev1.PodPending, corev1.PodFailed:
		return health.Unhealthy
	default:
		return health.Unknown
	}
}

func allContainersReady(p corev1.Pod) bool {
	if len(p.Status.ContainerStatuses) == 0 {
		return false
	}
	for _, cs := range p.Status.ContainerStatuses {
		if !cs.Ready {
			return false
		}
	}
	return true
}

// workloadHealth maps a workload's desired vs ready replica counts to a Status.
// Desired=0 (scaled down) is treated as Healthy since nothing is failing.
func workloadHealth(w appsWorkload) health.Status {
	if w.DesiredCount == 0 {
		return health.Healthy
	}
	if w.ReadyCount == 0 {
		return health.Unhealthy
	}
	if w.ReadyCount < w.DesiredCount {
		return health.Degraded
	}
	return health.Healthy
}

// serviceHealth aggregates the health of the pods a Service routes to.
// A Service with no selector (ExternalName, manual endpoints) returns Unknown.
// A Service whose selector matches no pods returns Unhealthy ("no healthy
// target pods").
func serviceHealth(selector map[string]string, matchedPodHealths []health.Status) health.Status {
	if len(selector) == 0 {
		return health.Unknown
	}
	if len(matchedPodHealths) == 0 {
		return health.Unhealthy
	}
	return aggregateHealth(matchedPodHealths)
}

// aggregateHealth returns the weakest-link status: Unhealthy dominates
// Degraded, which dominates Healthy. Unknown is returned only when the input
// is empty or contains only Unknown statuses.
func aggregateHealth(statuses []health.Status) health.Status {
	if len(statuses) == 0 {
		return health.Unknown
	}
	var sawUnhealthy, sawDegraded, sawHealthy bool
	for _, s := range statuses {
		switch s {
		case health.Unhealthy:
			sawUnhealthy = true
		case health.Degraded:
			sawDegraded = true
		case health.Healthy:
			sawHealthy = true
		}
	}
	switch {
	case sawUnhealthy:
		return health.Unhealthy
	case sawDegraded:
		return health.Degraded
	case sawHealthy:
		return health.Healthy
	default:
		return health.Unknown
	}
}
