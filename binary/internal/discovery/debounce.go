package discovery

import (
	"sync"
	"time"
)

// Debouncer coalesces bursts of Trigger() calls into a single fn invocation,
// fired once the input stream has been quiet for `delay`. Used to batch
// discoverer events (rolling deploys, rapid container starts) into one
// cache invalidation + WebSocket push.
type Debouncer struct {
	fn    func()
	delay time.Duration

	mu      sync.Mutex
	timer   *time.Timer
	stopped bool
}

func NewDebouncer(fn func(), delay time.Duration) *Debouncer {
	return &Debouncer{fn: fn, delay: delay}
}

func (d *Debouncer) Trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.delay, func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		if !d.stopped {
			d.fn()
		}
	})
}

func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = true
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
}
