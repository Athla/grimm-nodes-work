// Package kubernetes implements discovery.Discoverer against a Kubernetes
// cluster. It discovers Tier 1 resources (namespaces, workloads, pods,
// services) and produces topology nodes/edges for them. Authentication
// follows the standard in-cluster → kubeconfig fallback; if neither is
// available, New returns (nil, nil) so discovery simply does not activate.
package kubernetes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"binary/internal/discovery"
)

// SourceName is the Source tag applied to ServiceInfo produced by this discoverer.
const SourceName = "kubernetes"

// debounceWindow is the quiet period that collapses bursts of informer events
// into a single onChange invocation. Rolling deploys and scale events fire
// many events in quick succession; this batches them.
const debounceWindow = 2 * time.Second

// Config holds settings for Kubernetes discovery.
type Config struct {
	// KubeconfigPath overrides the default kubeconfig lookup. Empty = default
	// (~/.kube/config or KUBECONFIG env var).
	KubeconfigPath string
	// Context selects a specific kubeconfig context. Empty = current context.
	Context string
	// Namespaces restricts discovery to the listed namespaces. Empty = all.
	Namespaces []string
}

// Discovery implements discovery.Discoverer against a Kubernetes cluster.
type Discovery struct {
	client kubernetes.Interface
	cfg    Config

	mu      sync.Mutex
	factory informers.SharedInformerFactory
	stopCh  chan struct{}

	// readyCh is closed once informer caches have synced. All callers of
	// Discover/Watch wait on this before reading listers.
	readyCh  chan struct{}
	startErr error // non-nil if start failed
	once     sync.Once

	namespaces   corelisters.NamespaceLister
	pods         corelisters.PodLister
	services     corelisters.ServiceLister
	deployments  appslisters.DeploymentLister
	statefulsets appslisters.StatefulSetLister
	daemonsets   appslisters.DaemonSetLister
}

// Compile-time check that Discovery satisfies discovery.Discoverer.
var _ discovery.Discoverer = (*Discovery)(nil)

// New creates a Kubernetes discoverer. It tries in-cluster config first, then
// falls back to a kubeconfig file. If neither works it returns (nil, nil) so
// callers can skip Kubernetes discovery gracefully.
func New(cfg Config) (*Discovery, error) {
	restCfg, err := loadRestConfig(cfg)
	if err != nil {
		return nil, err
	}
	if restCfg == nil {
		return nil, nil
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: build clientset: %w", err)
	}

	return newWithClient(clientset, cfg), nil
}

// newWithClient is the internal constructor used by New and by tests.
func newWithClient(client kubernetes.Interface, cfg Config) *Discovery {
	return &Discovery{
		client:  client,
		cfg:     cfg,
		readyCh: make(chan struct{}),
	}
}

// Name returns the discoverer identifier used for logging.
func (d *Discovery) Name() string { return SourceName }

// Discover reads the current Tier 1 resource state from the informer cache
// and maps it into discovery.ServiceInfo entries. It lazily starts the
// informers on first call, waiting for cache sync within ctx.
func (d *Discovery) Discover(ctx context.Context) ([]discovery.ServiceInfo, error) {
	if err := d.start(ctx); err != nil {
		return nil, err
	}
	snap, err := d.snapshotFromCache()
	if err != nil {
		return nil, err
	}
	return mapSnapshot(snap), nil
}

// Watch attaches event handlers to the Tier 1 informers and invokes onChange
// whenever a change is detected, debounced to collapse bursts. It blocks
// until ctx is cancelled. Close() tears down the informers.
func (d *Discovery) Watch(ctx context.Context, onChange func()) error {
	if err := d.start(ctx); err != nil {
		return err
	}

	deb := newDebouncer(onChange, debounceWindow)
	defer deb.Stop()

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc:    func(_ interface{}) { deb.Trigger() },
		UpdateFunc: func(_, _ interface{}) { deb.Trigger() },
		DeleteFunc: func(_ interface{}) { deb.Trigger() },
	}

	informerAccessors := []func() cache.SharedIndexInformer{
		func() cache.SharedIndexInformer { return d.factory.Core().V1().Namespaces().Informer() },
		func() cache.SharedIndexInformer { return d.factory.Core().V1().Pods().Informer() },
		func() cache.SharedIndexInformer { return d.factory.Core().V1().Services().Informer() },
		func() cache.SharedIndexInformer { return d.factory.Apps().V1().Deployments().Informer() },
		func() cache.SharedIndexInformer { return d.factory.Apps().V1().StatefulSets().Informer() },
		func() cache.SharedIndexInformer { return d.factory.Apps().V1().DaemonSets().Informer() },
	}
	for _, acc := range informerAccessors {
		if _, err := acc().AddEventHandler(handler); err != nil {
			return fmt.Errorf("kubernetes: attach informer handler: %w", err)
		}
	}

	<-ctx.Done()
	return nil
}

// Close stops the shared informer factory and releases its goroutines.
// Safe to call multiple times.
func (d *Discovery) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopCh != nil {
		close(d.stopCh)
		d.stopCh = nil
	}
	return nil
}

// start lazily initialises the shared informer factory and blocks until all
// caches have synced. sync.Once ensures exactly one goroutine performs
// initialisation; all concurrent callers wait on readyCh.
func (d *Discovery) start(ctx context.Context) error {
	d.once.Do(func() {
		factory := informers.NewSharedInformerFactory(d.client, 0)

		// Register each informer by calling its accessor; these must exist
		// before factory.Start or they won't be started.
		nsInformer := factory.Core().V1().Namespaces()
		podInformer := factory.Core().V1().Pods()
		svcInformer := factory.Core().V1().Services()
		depInformer := factory.Apps().V1().Deployments()
		stsInformer := factory.Apps().V1().StatefulSets()
		dsInformer := factory.Apps().V1().DaemonSets()

		// Touch Informer() so the factory knows to start them.
		nsInformer.Informer()
		podInformer.Informer()
		svcInformer.Informer()
		depInformer.Informer()
		stsInformer.Informer()
		dsInformer.Informer()

		stopCh := make(chan struct{})
		factory.Start(stopCh)

		d.mu.Lock()
		d.factory = factory
		d.stopCh = stopCh
		d.namespaces = nsInformer.Lister()
		d.pods = podInformer.Lister()
		d.services = svcInformer.Lister()
		d.deployments = depInformer.Lister()
		d.statefulsets = stsInformer.Lister()
		d.daemonsets = dsInformer.Lister()
		d.mu.Unlock()

		// Wait for cache sync, then signal all waiters.
		factory.WaitForCacheSync(stopCh)
		close(d.readyCh)
	})

	// All callers (including concurrent ones) wait here until caches are synced.
	select {
	case <-d.readyCh:
		return d.startErr
	case <-ctx.Done():
		return fmt.Errorf("kubernetes: informer cache sync: %w", ctx.Err())
	}
}

// loadRestConfig tries in-cluster config first, then kubeconfig. Returns
// (nil, nil) if neither is available.
func loadRestConfig(cfg Config) (*rest.Config, error) {
	if restCfg, err := rest.InClusterConfig(); err == nil {
		return restCfg, nil
	}

	kubeconfigPath := resolveKubeconfigPath(cfg.KubeconfigPath)
	if kubeconfigPath == "" {
		return nil, nil
	}
	if _, err := os.Stat(kubeconfigPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("kubernetes: stat kubeconfig: %w", err)
	}

	loader := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	overrides := &clientcmd.ConfigOverrides{CurrentContext: cfg.Context}
	clientCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loader, overrides)
	restCfg, err := clientCfg.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kubernetes: load kubeconfig %q: %w", kubeconfigPath, err)
	}
	return restCfg, nil
}

func resolveKubeconfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if env := os.Getenv("KUBECONFIG"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".kube", "config")
}

// clusterSnapshot captures the raw Tier 1 resources read for a single
// Discover call. The mapper consumes this struct — keeping the fetch and
// mapping concerns separate makes both easier to test.
type clusterSnapshot struct {
	namespaces   []corev1.Namespace
	pods         []corev1.Pod
	services     []corev1.Service
	deployments  []appsWorkload
	statefulsets []appsWorkload
	daemonsets   []appsWorkload
}

// snapshotFromCache reads all Tier 1 resources from the informer listers,
// honouring the namespace scope filter in cfg.
func (d *Discovery) snapshotFromCache() (*clusterSnapshot, error) {
	snap := &clusterSnapshot{}

	nss, err := d.namespaces.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list namespaces: %w", err)
	}
	for _, ns := range nss {
		if !d.namespaceAllowed(ns.Name) {
			continue
		}
		snap.namespaces = append(snap.namespaces, *ns)
	}

	pods, err := d.pods.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list pods: %w", err)
	}
	for _, p := range pods {
		if !d.namespaceAllowed(p.Namespace) {
			continue
		}
		snap.pods = append(snap.pods, *p)
	}

	svcs, err := d.services.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list services: %w", err)
	}
	for _, s := range svcs {
		if !d.namespaceAllowed(s.Namespace) {
			continue
		}
		snap.services = append(snap.services, *s)
	}

	deps, err := d.deployments.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list deployments: %w", err)
	}
	for _, dep := range deps {
		if !d.namespaceAllowed(dep.Namespace) {
			continue
		}
		snap.deployments = append(snap.deployments, workloadFromDeployment(*dep))
	}

	stss, err := d.statefulsets.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list statefulsets in: %w", err)
	}
	for _, s := range stss {
		if !d.namespaceAllowed(s.Namespace) {
			continue
		}
		snap.statefulsets = append(snap.statefulsets, workloadFromStatefulSet(*s))
	}

	dss, err := d.daemonsets.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list daemonsets: %w", err)
	}
	for _, x := range dss {
		if !d.namespaceAllowed(x.Namespace) {
			continue
		}
		snap.daemonsets = append(snap.daemonsets, workloadFromDaemonSet(*x))
	}

	return snap, nil
}

func (d *Discovery) namespaceAllowed(ns string) bool {
	if len(d.cfg.Namespaces) == 0 {
		return true
	}
	for _, n := range d.cfg.Namespaces {
		if n == ns {
			return true
		}
	}
	return false
}
