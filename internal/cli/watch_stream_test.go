// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type stoppableWatch struct {
	events  chan watch.Event
	stopped chan struct{}
	once    sync.Once
}

func (w *stoppableWatch) ResultChan() <-chan watch.Event { return w.events }
func (w *stoppableWatch) Stop()                          { w.once.Do(func() { close(w.stopped) }) }

func TestObservedWatchCancelsWithoutAConsumer(t *testing.T) {
	upstream := &stoppableWatch{events: make(chan watch.Event, 1), stopped: make(chan struct{})}
	upstream.events <- watch.Event{Type: watch.Error, Object: &metav1.Status{Code: 403}}
	observed := make(chan struct{})
	w := observeWatch(context.Background(), upstream, func(watch.Event) { close(observed) }, func() {})
	select {
	case <-observed:
	case <-time.After(time.Second):
		t.Fatal("event was not observed")
	}
	w.Stop()
	// Do not read ResultChan: Stop must unblock the forwarding send itself.
	select {
	case <-upstream.stopped:
	case <-time.After(time.Second):
		t.Fatal("watch leaked while its consumer was stopped")
	}
}
