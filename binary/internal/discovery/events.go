package discovery

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"

	"binary/internal/adapters"
)

// EventWatcher subscribes to Docker container start/stop/die events
// and triggers cache invalidation on the registry.
type EventWatcher struct {
	client   *client.Client
	registry adapters.Registry
	cancel   context.CancelFunc
}

// NewEventWatcher creates a new EventWatcher.
func NewEventWatcher(cli *client.Client, reg adapters.Registry) *EventWatcher {
	return &EventWatcher{
		client:   cli,
		registry: reg,
	}
}

// Start begins watching for Docker container events in a background goroutine.
// It automatically reconnects with exponential backoff if the event stream disconnects.
func (w *EventWatcher) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	go func() {
		backoff := time.Second
		const maxBackoff = 30 * time.Second

		for {
			if ctx.Err() != nil {
				return
			}

			eventFilter := filters.NewArgs(
				filters.Arg("type", string(events.ContainerEventType)),
				filters.Arg("event", "start"),
				filters.Arg("event", "stop"),
				filters.Arg("event", "die"),
			)
			msgCh, errCh := w.client.Events(ctx, events.ListOptions{
				Filters: eventFilter,
			})

			start := time.Now()
			err := w.consumeEvents(ctx, msgCh, errCh)
			if time.Since(start) > 10*time.Second {
				backoff = time.Second // only reset if stream was stable
			}
			if ctx.Err() != nil {
				return
			}

			log.Printf("WARNING: Docker event stream error: %v (reconnecting in %v)", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}()
}

func (w *EventWatcher) consumeEvents(ctx context.Context, msgCh <-chan events.Message, errCh <-chan error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-msgCh:
			log.Printf("Docker event: %s %s (container: %s)",
				msg.Action, msg.Type, truncateID(msg.Actor.ID))
			w.registry.InvalidateCache()
		case err := <-errCh:
			if err != nil {
				return err
			}
			return fmt.Errorf("event stream closed")
		}
	}
}

// Stop cancels the event watcher.
func (w *EventWatcher) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}
