package kubernetes_test

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/guilherme-grimm/graph-go/internal/discovery"
	"github.com/guilherme-grimm/graph-go/internal/discovery/kubernetes"
)

// seedObjects returns the runtime.Objects that back the mapper test suite:
// one namespace "app" with a Deployment (2 replicas, 2 matching pods), a
// StatefulSet (1 replica, 1 matching pod), and a Service routing to the
// Deployment's pods via label selector.
func seedObjects() []runtime.Object {
	two := int32(2)
	one := int32(1)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app"}}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "app", Labels: map[string]string{"tier": "backend"}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &two,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 2},
	}
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "app"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "db"}},
		},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: 1},
	}

	apiPod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "app", Labels: map[string]string{"app": "api"}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "web", Image: "ghcr.io/acme/api:v1",
			Env: []corev1.EnvVar{{Name: "DB_HOST"}, {Name: "LOG_LEVEL"}},
		}}},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
		},
	}
	apiPod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api-2", Namespace: "app", Labels: map[string]string{"app": "api"}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "web", Image: "ghcr.io/acme/api:v1"}}},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
		},
	}
	dbPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "db-0", Namespace: "app", Labels: map[string]string{"app": "db"}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "pg", Image: "postgres:16"}}},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "app"},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			Selector:  map[string]string{"app": "api"},
			ClusterIP: "10.0.0.5",
		},
	}

	return []runtime.Object{ns, deploy, sts, apiPod1, apiPod2, dbPod, svc}
}

// runDiscover builds a Discovery against a fake clientset seeded with
// seedObjects() and returns the ServiceInfo entries Discover() produces.
func runDiscover(t *testing.T) []discovery.ServiceInfo {
	t.Helper()
	client := fake.NewSimpleClientset(seedObjects()...)
	d := kubernetes.NewWithClient(client, kubernetes.Config{})
	t.Cleanup(func() { _ = d.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	infos, err := d.Discover(ctx)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	return infos
}

func TestDiscover_NodeCountsAndIDs(t *testing.T) {
	result := runDiscover(t)

	// 1 ns + 1 deploy + 1 sts + 3 pods + 1 svc = 7 ServiceInfo entries, each
	// carrying one node.
	if len(result) != 7 {
		t.Fatalf("want 7 ServiceInfo, got %d", len(result))
	}

	byID := map[string]string{}
	for _, si := range result {
		for _, n := range si.Nodes {
			byID[n.Id] = n.Type
		}
	}

	expect := map[string]string{
		"k8s-namespace-app":       "namespace",
		"k8s-app-deployment-api":  "deployment",
		"k8s-app-statefulset-db":  "statefulset",
		"k8s-app-pod-api-1":       "pod",
		"k8s-app-pod-api-2":       "pod",
		"k8s-app-pod-db-0":        "pod",
		"k8s-app-k8s_service-api": "k8s_service",
	}
	for id, typ := range expect {
		got, ok := byID[id]
		if !ok {
			t.Errorf("missing node id %q", id)
			continue
		}
		if got != typ {
			t.Errorf("node %q: want type %q, got %q", id, typ, got)
		}
	}
}

func TestDiscover_NamespaceIsGroup(t *testing.T) {
	result := runDiscover(t)
	for _, si := range result {
		if si.Type != "namespace" {
			continue
		}
		if len(si.Nodes) != 1 {
			t.Fatalf("namespace ServiceInfo should have 1 node")
		}
		meta := si.Nodes[0].Metadata
		if g, ok := meta["group"].(bool); !ok || !g {
			t.Errorf(`namespace node missing metadata["group"] = true, got %v`, meta["group"])
		}
	}
}

func TestDiscover_ContainsEdges(t *testing.T) {
	result := runDiscover(t)

	var containsEdges []string
	for _, si := range result {
		for _, e := range si.Edges {
			if e.Type == "contains" {
				containsEdges = append(containsEdges, e.Source+"→"+e.Target)
			}
		}
	}

	// Expected: ns→deployment, ns→statefulset, deployment→2 pods, statefulset→1 pod = 5 contains edges.
	wantContain := []string{
		"k8s-namespace-app→k8s-app-deployment-api",
		"k8s-namespace-app→k8s-app-statefulset-db",
		"k8s-app-deployment-api→k8s-app-pod-api-1",
		"k8s-app-deployment-api→k8s-app-pod-api-2",
		"k8s-app-statefulset-db→k8s-app-pod-db-0",
	}
	if len(containsEdges) != len(wantContain) {
		t.Fatalf("want %d contains edges, got %d: %v", len(wantContain), len(containsEdges), containsEdges)
	}
	for _, w := range wantContain {
		found := false
		for _, got := range containsEdges {
			if got == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing contains edge %q (have: %v)", w, containsEdges)
		}
	}
}

func TestDiscover_ServiceRoutesToMatchingPods(t *testing.T) {
	result := runDiscover(t)

	var routesEdges []string
	for _, si := range result {
		if si.Type != "k8s_service" {
			continue
		}
		for _, e := range si.Edges {
			if e.Type != "routes_to" {
				continue
			}
			routesEdges = append(routesEdges, e.Source+"→"+e.Target)
		}
	}

	// Service "api" selects app=api; matches api-1 + api-2 but NOT db-0.
	if len(routesEdges) != 2 {
		t.Fatalf("want 2 routes_to edges, got %d: %v", len(routesEdges), routesEdges)
	}
	for _, e := range routesEdges {
		if !strings.HasPrefix(e, "k8s-app-k8s_service-api→") {
			t.Errorf("unexpected routes_to source: %q", e)
		}
		if strings.HasSuffix(e, "k8s-app-pod-db-0") {
			t.Errorf("service should not route to db-0: %q", e)
		}
	}
}

func TestDiscover_PodMetadataRetained(t *testing.T) {
	result := runDiscover(t)

	var meta map[string]any
	for _, si := range result {
		if si.Type == "pod" && si.Name == "api-1" {
			meta = si.Nodes[0].Metadata
			break
		}
	}
	if meta == nil {
		t.Fatal("api-1 pod not found")
	}

	if got := meta["image"].(string); got != "ghcr.io/acme/api:v1" {
		t.Errorf("image = %q", got)
	}
	envKeys := meta["env_keys"].([]string)
	if len(envKeys) != 2 || envKeys[0] != "DB_HOST" || envKeys[1] != "LOG_LEVEL" {
		t.Errorf("env_keys = %v", envKeys)
	}
	if got := meta["labels"].(map[string]string)["app"]; got != "api" {
		t.Errorf("labels[app] = %q", got)
	}
}

func TestPodMatchesServiceSelector(t *testing.T) {
	tests := []struct {
		name      string
		selector  map[string]string
		podLabels map[string]string
		want      bool
	}{
		{"nil selector", nil, map[string]string{"a": "1"}, false},
		{"empty selector", map[string]string{}, map[string]string{"a": "1"}, false},
		{"full match", map[string]string{"app": "api"}, map[string]string{"app": "api", "tier": "be"}, true},
		{"multi-key match", map[string]string{"app": "api", "env": "prod"}, map[string]string{"app": "api", "env": "prod"}, true},
		{"partial mismatch", map[string]string{"app": "api", "env": "prod"}, map[string]string{"app": "api", "env": "dev"}, false},
		{"missing key", map[string]string{"app": "api"}, map[string]string{"other": "x"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kubernetes.PodMatchesServiceSelector(tt.selector, tt.podLabels); got != tt.want {
				t.Errorf("podMatchesServiceSelector: want %v, got %v", tt.want, got)
			}
		})
	}
}
