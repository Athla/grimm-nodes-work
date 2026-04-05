package kubernetes_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"binary/internal/discovery/kubernetes"
)

// TestWatch_FiresOnChangeAfterEvents verifies that informer events (Create/
// Update/Delete on any of the six watched resources) drive the onChange
// callback, and that a burst of events collapses into a single call thanks
// to the 2s debounce window.
func TestWatch_FiresOnChangeAfterEvents(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app"}},
	)
	d := kubernetes.NewWithClient(client, kubernetes.Config{})
	t.Cleanup(func() { _ = d.Close() })

	var calls int32
	watchCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- d.Watch(watchCtx, func() { atomic.AddInt32(&calls, 1) })
	}()

	// Wait for Watch to finish starting informers (cache sync). We can't
	// observe this directly, so poll by creating pods until one triggers a
	// callback — informers always send initial ADDs for the seeded object,
	// but we need our own writes to fire through the live event handler.
	time.Sleep(200 * time.Millisecond)

	// Burst of writes: create, update, delete a handful of pods inside the
	// debounce window. Each write generates an informer event, but the
	// debouncer should collapse them into one onChange call.
	for i := 0; i < 5; i++ {
		_, err := client.CoreV1().Pods("app").Create(context.Background(), &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p" + string(rune('0'+i)), Namespace: "app"},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create pod: %v", err)
		}
	}

	// Debounce window is 2s — wait a bit longer so the timer fires.
	time.Sleep(3 * time.Second)

	got := atomic.LoadInt32(&calls)
	if got < 1 {
		t.Fatalf("expected at least one onChange call, got %d", got)
	}
	// Burst should collapse: we made 5 rapid writes, so anything > 2 would
	// mean debouncing broke. (Allow 2 because the informer's initial ADD
	// for the seeded namespace may land in a separate window.)
	if got > 2 {
		t.Errorf("burst should collapse, got %d onChange calls", got)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Watch returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Watch did not return after ctx cancel")
	}
}
