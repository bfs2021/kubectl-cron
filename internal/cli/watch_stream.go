// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"

	"k8s.io/apimachinery/pkg/watch"
)

type observedWatch struct {
	result chan watch.Event
	cancel context.CancelFunc
}

func (w *observedWatch) ResultChan() <-chan watch.Event { return w.result }
func (w *observedWatch) Stop()                          { w.cancel() }

// Every forwarding send is cancellable: a fatal Event can cancel the consumer
// before it reads that Event, and must not strand the forwarding goroutine.
func observeWatch(ctx context.Context, upstream watch.Interface, observe func(watch.Event), closed func()) watch.Interface {
	ctx, cancel := context.WithCancel(ctx)
	w := &observedWatch{result: make(chan watch.Event), cancel: cancel}
	go func() {
		defer close(w.result)
		defer upstream.Stop()
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-upstream.ResultChan():
				if !ok {
					if ctx.Err() == nil {
						closed()
					}
					return
				}
				observe(event)
				select {
				case w.result <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return w
}
