package kubernetes_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/guilherme-grimm/graph-go/internal/discovery/kubernetes"
	"github.com/guilherme-grimm/graph-go/internal/graph/health"
)

func pod(phase corev1.PodPhase, containerReady ...bool) corev1.Pod {
	statuses := make([]corev1.ContainerStatus, len(containerReady))
	for i, r := range containerReady {
		statuses[i] = corev1.ContainerStatus{Ready: r}
	}
	return corev1.Pod{
		Status: corev1.PodStatus{Phase: phase, ContainerStatuses: statuses},
	}
}

func TestPodHealth(t *testing.T) {
	tests := []struct {
		name string
		pod  corev1.Pod
		want health.Status
	}{
		{"running all ready", pod(corev1.PodRunning, true, true), health.Healthy},
		{"running one not ready", pod(corev1.PodRunning, true, false), health.Degraded},
		{"running no statuses yet", pod(corev1.PodRunning), health.Degraded},
		{"pending", pod(corev1.PodPending), health.Unhealthy},
		{"failed", pod(corev1.PodFailed), health.Unhealthy},
		{"succeeded", pod(corev1.PodSucceeded), health.Healthy},
		{"unknown phase", pod(corev1.PodUnknown), health.Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kubernetes.PodHealth(tt.pod); got != tt.want {
				t.Errorf("podHealth = %q, want %q", got, tt.want)
			}
		})
	}
}

func workload(desired, ready int32) kubernetes.AppsWorkload {
	return kubernetes.AppsWorkload{
		Meta:         metav1.ObjectMeta{Name: "w", Namespace: "ns"},
		DesiredCount: desired,
		ReadyCount:   ready,
	}
}

func TestWorkloadHealth(t *testing.T) {
	tests := []struct {
		name string
		w    kubernetes.AppsWorkload
		want health.Status
	}{
		{"all ready", workload(3, 3), health.Healthy},
		{"partial ready", workload(3, 2), health.Degraded},
		{"none ready", workload(3, 0), health.Unhealthy},
		{"scaled to zero", workload(0, 0), health.Healthy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kubernetes.WorkloadHealth(tt.w); got != tt.want {
				t.Errorf("workloadHealth = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServiceHealth(t *testing.T) {
	selector := map[string]string{"app": "api"}
	tests := []struct {
		name     string
		selector map[string]string
		matched  []health.Status
		want     health.Status
	}{
		{"no selector", nil, nil, health.Unknown},
		{"empty selector", map[string]string{}, nil, health.Unknown},
		{"selector matches nothing", selector, nil, health.Unhealthy},
		{"all healthy", selector, []health.Status{health.Healthy, health.Healthy}, health.Healthy},
		{"one degraded", selector, []health.Status{health.Healthy, health.Degraded}, health.Degraded},
		{"one unhealthy", selector, []health.Status{health.Healthy, health.Unhealthy}, health.Unhealthy},
		{"all unhealthy", selector, []health.Status{health.Unhealthy, health.Unhealthy}, health.Unhealthy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kubernetes.ServiceHealth(tt.selector, tt.matched); got != tt.want {
				t.Errorf("serviceHealth = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAggregateHealth(t *testing.T) {
	tests := []struct {
		name string
		in   []health.Status
		want health.Status
	}{
		{"empty", nil, health.Unknown},
		{"all unknown", []health.Status{health.Unknown, health.Unknown}, health.Unknown},
		{"all healthy", []health.Status{health.Healthy, health.Healthy}, health.Healthy},
		{"healthy + unknown", []health.Status{health.Healthy, health.Unknown}, health.Healthy},
		{"any degraded beats healthy", []health.Status{health.Healthy, health.Degraded}, health.Degraded},
		{"any unhealthy beats degraded", []health.Status{health.Degraded, health.Unhealthy}, health.Unhealthy},
		{"unhealthy dominates", []health.Status{health.Unhealthy, health.Healthy, health.Degraded}, health.Unhealthy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kubernetes.AggregateHealth(tt.in); got != tt.want {
				t.Errorf("aggregateHealth = %q, want %q", got, tt.want)
			}
		})
	}
}
