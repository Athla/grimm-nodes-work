package docker

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/docker/api/types/events"
	"go.uber.org/zap"
)

var nopLogger = zap.NewNop().Sugar()

func TestConsumeEvents_NormalEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgCh := make(chan events.Message, 1)
	errCh := make(chan error, 1)
	var calls int32

	msgCh <- events.Message{Action: "start", Type: "container"}

	// Cancel after the event is consumed so consumeEvents returns.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_ = consumeEvents(ctx, msgCh, errCh, func() {
		atomic.AddInt32(&calls, 1)
	}, nopLogger)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected onChange called once, got %d", got)
	}
}

func TestConsumeEvents_ChannelClosed(t *testing.T) {
	ctx := context.Background()
	msgCh := make(chan events.Message)
	errCh := make(chan error, 1)

	close(msgCh)

	err := consumeEvents(ctx, msgCh, errCh, func() {}, nopLogger)
	if err == nil || !strings.Contains(err.Error(), "event stream closed") {
		t.Errorf("expected 'event stream closed' error, got %v", err)
	}
}

func TestConsumeEvents_ErrorOnErrCh(t *testing.T) {
	ctx := context.Background()
	msgCh := make(chan events.Message)
	errCh := make(chan error, 1)

	errCh <- context.DeadlineExceeded

	err := consumeEvents(ctx, msgCh, errCh, func() {}, nopLogger)
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestConsumeEvents_NilErrorOnErrCh(t *testing.T) {
	ctx := context.Background()
	msgCh := make(chan events.Message)
	errCh := make(chan error, 1)

	errCh <- nil

	err := consumeEvents(ctx, msgCh, errCh, func() {}, nopLogger)
	if err == nil || !strings.Contains(err.Error(), "event stream closed") {
		t.Errorf("expected 'event stream closed' error, got %v", err)
	}
}

func TestConsumeEvents_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	msgCh := make(chan events.Message)
	errCh := make(chan error)

	err := consumeEvents(ctx, msgCh, errCh, func() {}, nopLogger)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
