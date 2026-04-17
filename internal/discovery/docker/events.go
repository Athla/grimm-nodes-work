package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

// watchEvents subscribes to Docker container start/stop/die events and
// invokes onChange for each one. It reconnects with exponential backoff
// and returns only when ctx is cancelled.
func watchEvents(ctx context.Context, cli *client.Client, onChange func(), logger *zap.SugaredLogger) error {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		if ctx.Err() != nil {
			return nil
		}

		eventFilter := filters.NewArgs(
			filters.Arg("type", string(events.ContainerEventType)),
			filters.Arg("event", "start"),
			filters.Arg("event", "stop"),
			filters.Arg("event", "die"),
		)
		msgCh, errCh := cli.Events(ctx, events.ListOptions{
			Filters: eventFilter,
		})

		start := time.Now()
		err := consumeEvents(ctx, msgCh, errCh, onChange, logger)
		if time.Since(start) > 10*time.Second {
			backoff = time.Second // only reset if stream was stable
		}
		if ctx.Err() != nil {
			return nil
		}

		logger.Warnw("docker event stream error", "err", err, "reconnect_in", backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func consumeEvents(ctx context.Context, msgCh <-chan events.Message, errCh <-chan error, onChange func(), logger *zap.SugaredLogger) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgCh:
			if !ok {
				return fmt.Errorf("event stream closed")
			}
			logger.Debugw("docker event", "action", msg.Action, "type", msg.Type, "container", truncateID(msg.Actor.ID))
			onChange()
		case err := <-errCh:
			if err != nil {
				return err
			}
			return fmt.Errorf("event stream closed")
		}
	}
}
