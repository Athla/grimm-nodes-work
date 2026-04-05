package kubernetes

import (
	"time"

	"binary/internal/discovery"
)

// NewDebouncer creates a shared Debouncer. Exported for testing.
func NewDebouncer(fn func(), delay time.Duration) *discovery.Debouncer {
	return discovery.NewDebouncer(fn, delay)
}

// newDebouncer is the internal alias used within this package.
func newDebouncer(fn func(), delay time.Duration) *discovery.Debouncer {
	return discovery.NewDebouncer(fn, delay)
}
