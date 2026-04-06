package discovery

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestDebouncer_CollapsesBurst(t *testing.T) {
	var calls int32
	d := NewDebouncer(func() { atomic.AddInt32(&calls, 1) }, 30*time.Millisecond)
	defer d.Stop()

	for i := 0; i < 10; i++ {
		d.Trigger()
		time.Sleep(2 * time.Millisecond)
	}

	time.Sleep(80 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("burst should collapse to one call, got %d", got)
	}
}

func TestDebouncer_QuiescentWindowsFireSeparately(t *testing.T) {
	var calls int32
	d := NewDebouncer(func() { atomic.AddInt32(&calls, 1) }, 30*time.Millisecond)
	defer d.Stop()

	d.Trigger()
	time.Sleep(80 * time.Millisecond)

	d.Trigger()
	time.Sleep(80 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("two separated triggers should fire twice, got %d", got)
	}
}

func TestDebouncer_StopPreventsFire(t *testing.T) {
	var calls int32
	d := NewDebouncer(func() { atomic.AddInt32(&calls, 1) }, 30*time.Millisecond)

	d.Trigger()
	d.Stop()
	time.Sleep(80 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("stop before delay should suppress fn, got %d calls", got)
	}
}

func TestDebouncer_TriggerAfterStopIsNoop(t *testing.T) {
	var calls int32
	d := NewDebouncer(func() { atomic.AddInt32(&calls, 1) }, 10*time.Millisecond)
	d.Stop()

	d.Trigger()
	time.Sleep(30 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("trigger after stop should be noop, got %d calls", got)
	}
}
