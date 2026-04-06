package kubernetes

import (
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"binary/internal/discovery"
	"binary/internal/graph/edges"
	"binary/internal/graph/health"
	"binary/internal/graph/nodes"
)

// appsWorkload normalises the three appsv1 workload kinds (Deployment,
// StatefulSet, DaemonSet) into a single shape the mapper can iterate over.
type appsWorkload struct {
	Kind     string // "deployment" | "statefulset" | "daemonset"
	NodeType nodes.NodeType
	Meta     metav1.ObjectMeta
	Selector *metav1.LabelSelector

	// Replica status — populated for Deployment/StatefulSet, or ready/desired
	// node counts for DaemonSet. health.go consumes these uniformly.
	DesiredCount int32
	ReadyCount   int32
}

func workloadFromDeployment(d appsv1.Deployment) appsWorkload {
	desired := int32(0)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	return appsWorkload{
		Kind:         "deployment",
		NodeType:     nodes.TypeDeployment,
		Meta:         d.ObjectMeta,
		Selector:     d.Spec.Selector,
		DesiredCount: desired,
		ReadyCount:   d.Status.ReadyReplicas,
	}
}

func workloadFromStatefulSet(s appsv1.StatefulSet) appsWorkload {
	desired := int32(0)
	if s.Spec.Replicas != nil {
		desired = *s.Spec.Replicas
	}
	return appsWorkload{
		Kind:         "statefulset",
		NodeType:     nodes.TypeStatefulSet,
		Meta:         s.ObjectMeta,
		Selector:     s.Spec.Selector,
		DesiredCount: desired,
		ReadyCount:   s.Status.ReadyReplicas,
	}
}

func workloadFromDaemonSet(x appsv1.DaemonSet) appsWorkload {
	return appsWorkload{
		Kind:         "daemonset",
		NodeType:     nodes.TypeDaemonSet,
		Meta:         x.ObjectMeta,
		Selector:     x.Spec.Selector,
		DesiredCount: x.Status.DesiredNumberScheduled,
		ReadyCount:   x.Status.NumberReady,
	}
}

// nodeID builds the canonical Kubernetes node ID: k8s-<namespace>-<kind>-<name>.
// Cluster-scoped resources (namespaces themselves) use empty namespace.
func nodeID(namespace, kind, name string) string {
	if namespace == "" {
		return fmt.Sprintf("k8s-%s-%s", kind, name)
	}
	return fmt.Sprintf("k8s-%s-%s-%s", namespace, kind, name)
}

// mapSnapshot converts a raw cluster snapshot into a flat list of ServiceInfo
// entries — one per Kubernetes resource. Each entry carries its node plus any
// edges that terminate at (or originate from, for services) that node, and
// its health status (aggregated up from children for namespaces and services).
func mapSnapshot(snap *clusterSnapshot) []discovery.ServiceInfo {
	result := make([]discovery.ServiceInfo, 0,
		len(snap.namespaces)+len(snap.pods)+len(snap.services)+
			len(snap.deployments)+len(snap.statefulsets)+len(snap.daemonsets))

	// Index workloads by namespace for pod-ownership edge resolution.
	workloadsByNs := make(map[string][]appsWorkload)
	for _, w := range snap.deployments {
		workloadsByNs[w.Meta.Namespace] = append(workloadsByNs[w.Meta.Namespace], w)
	}
	for _, w := range snap.statefulsets {
		workloadsByNs[w.Meta.Namespace] = append(workloadsByNs[w.Meta.Namespace], w)
	}
	for _, w := range snap.daemonsets {
		workloadsByNs[w.Meta.Namespace] = append(workloadsByNs[w.Meta.Namespace], w)
	}

	// Index pods by namespace for service label-selector resolution, and
	// precompute pod health (consumed by both pod ServiceInfo + service aggregation).
	podsByNs := make(map[string][]corev1.Pod)
	podHealths := make(map[string]health.Status, len(snap.pods))
	for _, p := range snap.pods {
		podsByNs[p.Namespace] = append(podsByNs[p.Namespace], p)
		podHealths[p.Namespace+"/"+p.Name] = podHealth(p)
	}

	// Track per-namespace child healths (workloads + services) for aggregation.
	nsChildHealths := make(map[string][]health.Status)

	for _, w := range snap.deployments {
		h := workloadHealth(w)
		nsChildHealths[w.Meta.Namespace] = append(nsChildHealths[w.Meta.Namespace], h)
		result = append(result, mapWorkload(w, h))
	}
	for _, w := range snap.statefulsets {
		h := workloadHealth(w)
		nsChildHealths[w.Meta.Namespace] = append(nsChildHealths[w.Meta.Namespace], h)
		result = append(result, mapWorkload(w, h))
	}
	for _, w := range snap.daemonsets {
		h := workloadHealth(w)
		nsChildHealths[w.Meta.Namespace] = append(nsChildHealths[w.Meta.Namespace], h)
		result = append(result, mapWorkload(w, h))
	}
	for _, p := range snap.pods {
		result = append(result, mapPod(p, workloadsByNs[p.Namespace], podHealths[p.Namespace+"/"+p.Name]))
	}
	for _, s := range snap.services {
		var matchedHealths []health.Status
		for _, p := range podsByNs[s.Namespace] {
			if podMatchesServiceSelector(s.Spec.Selector, p.Labels) {
				matchedHealths = append(matchedHealths, podHealths[p.Namespace+"/"+p.Name])
			}
		}
		svcH := serviceHealth(s.Spec.Selector, matchedHealths)
		nsChildHealths[s.Namespace] = append(nsChildHealths[s.Namespace], svcH)
		result = append(result, mapService(s, podsByNs[s.Namespace], svcH))
	}

	// Namespaces last — their health aggregates over children collected above.
	nsInfos := make([]discovery.ServiceInfo, 0, len(snap.namespaces))
	for _, ns := range snap.namespaces {
		nsInfos = append(nsInfos, mapNamespace(ns, aggregateHealth(nsChildHealths[ns.Name])))
	}
	return append(nsInfos, result...)
}

func mapNamespace(ns corev1.Namespace, h health.Status) discovery.ServiceInfo {
	id := nodeID("", "namespace", ns.Name)
	node := nodes.Node{
		Id:     id,
		Type:   string(nodes.TypeNamespace),
		Name:   ns.Name,
		Health: string(h),
		Metadata: map[string]any{
			"group":  true,
			"labels": copyLabels(ns.Labels),
		},
	}
	return discovery.ServiceInfo{
		Name:     ns.Name,
		Type:     string(nodes.TypeNamespace),
		Source:   SourceName,
		Nodes:    []nodes.Node{node},
		Health:   h,
		Metadata: map[string]any{"namespace": ns.Name},
	}
}

func mapWorkload(w appsWorkload, h health.Status) discovery.ServiceInfo {
	nsID := nodeID("", "namespace", w.Meta.Namespace)
	id := nodeID(w.Meta.Namespace, w.Kind, w.Meta.Name)

	node := nodes.Node{
		Id:     id,
		Type:   string(w.NodeType),
		Name:   w.Meta.Name,
		Parent: nsID,
		Health: string(h),
		Metadata: map[string]any{
			"namespace": w.Meta.Namespace,
			"labels":    copyLabels(w.Meta.Labels),
			"desired":   w.DesiredCount,
			"ready":     w.ReadyCount,
		},
	}
	edge := edges.Edge{
		Id:     fmt.Sprintf("%s-contains-%s", nsID, id),
		Source: nsID,
		Target: id,
		Type:   "contains",
	}
	return discovery.ServiceInfo{
		Name:   w.Meta.Name,
		Type:   string(w.NodeType),
		Source: SourceName,
		Nodes:  []nodes.Node{node},
		Edges:  []edges.Edge{edge},
		Health: h,
		Metadata: map[string]any{
			"namespace": w.Meta.Namespace,
			"kind":      w.Kind,
		},
	}
}

func mapPod(p corev1.Pod, workloads []appsWorkload, h health.Status) discovery.ServiceInfo {
	id := nodeID(p.Namespace, "pod", p.Name)

	containerNames := make([]string, 0, len(p.Spec.Containers))
	images := make([]string, 0, len(p.Spec.Containers))
	envKeySet := make(map[string]struct{})
	for _, c := range p.Spec.Containers {
		containerNames = append(containerNames, c.Name)
		images = append(images, c.Image)
		for _, e := range c.Env {
			envKeySet[e.Name] = struct{}{}
		}
	}
	envKeys := make([]string, 0, len(envKeySet))
	for k := range envKeySet {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)

	primaryImage := ""
	if len(images) > 0 {
		primaryImage = images[0]
	}

	// Sum restart counts across all containers.
	var restartCount int32
	for _, cs := range p.Status.ContainerStatuses {
		restartCount += cs.RestartCount
	}

	// Derive synthetic phase: if any container is in CrashLoopBackOff or
	// waiting with an error reason, surface that instead of the raw pod phase.
	phase := string(p.Status.Phase)
	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			switch cs.State.Waiting.Reason {
			case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError":
				phase = cs.State.Waiting.Reason
			}
		}
	}

	node := nodes.Node{
		Id:     id,
		Type:   string(nodes.TypePod),
		Name:   p.Name,
		Parent: nodeID("", "namespace", p.Namespace),
		Health: string(h),
		Metadata: map[string]any{
			"namespace":     p.Namespace,
			"labels":        copyLabels(p.Labels),
			"image":         primaryImage,
			"images":        images,
			"containers":    containerNames,
			"env_keys":      envKeys,
			"phase":         phase,
			"restart_count": restartCount,
		},
	}

	var edgeList []edges.Edge
	for _, w := range workloads {
		if !podMatchesWorkloadSelector(w.Selector, p.Labels) {
			continue
		}
		workloadID := nodeID(w.Meta.Namespace, w.Kind, w.Meta.Name)
		edgeList = append(edgeList, edges.Edge{
			Id:     fmt.Sprintf("%s-contains-%s", workloadID, id),
			Source: workloadID,
			Target: id,
			Type:   "contains",
		})
	}

	return discovery.ServiceInfo{
		Name:   p.Name,
		Type:   string(nodes.TypePod),
		Source: SourceName,
		Nodes:  []nodes.Node{node},
		Edges:  edgeList,
		Health: h,
		Metadata: map[string]any{
			"namespace": p.Namespace,
		},
	}
}

func mapService(s corev1.Service, podsInNS []corev1.Pod, h health.Status) discovery.ServiceInfo {
	id := nodeID(s.Namespace, "k8s_service", s.Name)

	// Collect port specs as compact strings (e.g. "80/TCP", "443/TCP").
	ports := make([]string, 0, len(s.Spec.Ports))
	for _, p := range s.Spec.Ports {
		ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
	}

	node := nodes.Node{
		Id:     id,
		Type:   string(nodes.TypeK8sService),
		Name:   s.Name,
		Parent: nodeID("", "namespace", s.Namespace),
		Health: string(h),
		Metadata: map[string]any{
			"namespace":    s.Namespace,
			"labels":       copyLabels(s.Labels),
			"service_type": string(s.Spec.Type),
			"selector":     copyLabels(s.Spec.Selector),
			"cluster_ip":   s.Spec.ClusterIP,
			"ports":        ports,
		},
	}

	var edgeList []edges.Edge
	if len(s.Spec.Selector) > 0 {
		for _, p := range podsInNS {
			if !podMatchesServiceSelector(s.Spec.Selector, p.Labels) {
				continue
			}
			podID := nodeID(p.Namespace, "pod", p.Name)
			edgeList = append(edgeList, edges.Edge{
				Id:     fmt.Sprintf("%s-routes_to-%s", id, podID),
				Source: id,
				Target: podID,
				Type:   edges.TypeRoutesTo,
			})
		}
	}

	return discovery.ServiceInfo{
		Name:   s.Name,
		Type:   string(nodes.TypeK8sService),
		Source: SourceName,
		Nodes:  []nodes.Node{node},
		Edges:  edgeList,
		Health: h,
		Metadata: map[string]any{
			"namespace": s.Namespace,
		},
	}
}

func podMatchesWorkloadSelector(sel *metav1.LabelSelector, podLabels map[string]string) bool {
	if sel == nil {
		return false
	}
	s, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil || s.Empty() {
		return false
	}
	return s.Matches(labels.Set(podLabels))
}

func podMatchesServiceSelector(sel, podLabels map[string]string) bool {
	if len(sel) == 0 {
		return false
	}
	for k, v := range sel {
		if podLabels[k] != v {
			return false
		}
	}
	return true
}

func copyLabels(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
