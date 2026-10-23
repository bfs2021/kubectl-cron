// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/mertdotcc/kubectl-cron/internal/analyze"
	"github.com/mertdotcc/kubectl-cron/internal/model"
	"github.com/mertdotcc/kubectl-cron/internal/render"
	"github.com/spf13/cobra"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

// Prefer classic list-then-watch consistently across supported servers and
// client-go feature defaults; the reflector still manages pagination and RVs.
type cronListWatch struct{ *cache.ListWatch }

func (*cronListWatch) IsWatchListSemanticsUnSupported() bool { return true }

type connectionState struct {
	mu    sync.Mutex
	value model.WatchState
	// Counts streaming connections, including any between display ticks.
	streams int
}

func (s *connectionState) set(state string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value = model.WatchState{Mode: "api", State: state}
	if state == "streaming" {
		s.streams++
	}
	if err != nil {
		s.value.LastError = err.Error()
	}
}
func (s *connectionState) get() model.WatchState { s.mu.Lock(); defer s.mu.Unlock(); return s.value }
func (s *connectionState) snapshot() (model.WatchState, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value, s.streams
}

func (o *options) runWatch(cmd *cobra.Command, args []string) error {
	if len(o.filenames) > 0 {
		return fmt.Errorf("--watch requires live CronJobs; --filename is offline")
	}
	if o.watchInterval < 250*time.Millisecond || o.watchInterval > time.Minute {
		return fmt.Errorf("--watch-interval must be between 250ms and 1m")
	}
	batch, _, ns, err := o.clients()
	if err != nil {
		return err
	}
	if o.allNamespaces {
		ns = metav1.NamespaceAll
	}
	name := ""
	if len(args) > 0 {
		name, err = objectName(args[0])
		if err != nil {
			return err
		}
	}
	selector := ""
	if name != "" {
		selector = fields.OneTermEqualSelector("metadata.name", name).String()
	}
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	state := &connectionState{}
	state.set("connecting", nil)
	var streamed atomic.Bool
	fatal := make(chan error, 1)
	fail := func(err error) {
		select {
		case fatal <- fmt.Errorf("watch CronJobs: %w", err):
		default:
		}
		cancel()
	}
	query := func(opts metav1.ListOptions) metav1.ListOptions {
		opts.LabelSelector = o.selector
		opts.FieldSelector = selector
		return opts
	}
	lw := &cronListWatch{&cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			state.set("resyncing", nil)
			opts = query(opts)
			switch opts.ResourceVersion {
			case "0":
				opts.ResourceVersion = ""
			case "":
			default:
				// A paginated list at a specific RV is an Exact read. After an
				// expired watch it returns the same stale RV, so the relist never
				// advances; unpaginated, it is at least as fresh as that RV.
				opts.Limit = 0
			}
			return batch.CronJobs(ns).List(ctx, opts)
		},
		WatchFuncWithContext: func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
			state.set("reconnecting", nil)
			opts = query(opts)
			opts.AllowWatchBookmarks = true
			stream, err := batch.CronJobs(ns).Watch(ctx, opts)
			if err == nil {
				streamed.Store(true)
				state.set("streaming", nil)
				// Keep a stream's error visible while the reflector backs off.
				var streamErr error
				stream = observeWatch(ctx, stream, func(event watch.Event) {
					if event.Type == watch.Error {
						streamErr = apierrors.FromObject(event.Object)
						state.set("reconnecting", streamErr)
						if fatalWatchError(streamErr, streamed.Load()) {
							fail(streamErr)
						}
					}
				}, func() { state.set("reconnecting", streamErr) })
			} else {
				state.set("reconnecting", err)
				if fatalWatchError(err, streamed.Load()) {
					fail(err)
				}
			}
			return stream, err
		},
	}}
	informer := cache.NewSharedInformer(lw, &batchv1.CronJob{}, 0)
	if err := informer.SetWatchErrorHandlerWithContext(func(ctx context.Context, _ *cache.Reflector, err error) {
		if ctx.Err() != nil {
			return
		}
		state.set("reconnecting", err)
		if fatalWatchError(err, streamed.Load()) {
			fail(err)
		}
	}); err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Go(func() { informer.RunWithContext(ctx) })
	ready := make(chan struct{})
	wg.Go(func() {
		if cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
			close(ready)
		}
	})
	defer func() { cancel(); wg.Wait() }()
	ticker := time.NewTicker(o.watchInterval)
	defer ticker.Stop()
	synced := false
	// Reconnect cycles clear the error between attempts, so a notice repeats
	// only for a different error or after the watch has streamed again.
	lastNotice, noticeStreams := "", 0
	draw := func() error {
		cjs := []batchv1.CronJob{}
		for _, obj := range informer.GetStore().List() {
			if cj, ok := obj.(*batchv1.CronJob); ok {
				cjs = append(cjs, *cj.DeepCopy())
			}
		}
		now := o.now()
		r := analyze.Overview(cjs, "cluster", now, analyze.Options{AssumeTimezone: o.assumeTimezone, NoFindings: o.noFindings, Problems: o.problems, Sort: o.sort, Count: 1})
		hasFindings := r.HasFindings()
		switch o.summary {
		case "hide":
			r.Summary = nil
		case "only":
			r.CronJobs = nil
		}
		connection := state.get()
		r.Watch = &connection
		if err := o.watchFrame(cmd, r); err != nil {
			return err
		}
		if o.exitOnFindings && hasFindings {
			return ErrFindings
		}
		return nil
	}
	for {
		select {
		case err := <-fatal:
			return err
		case <-ctx.Done():
			select {
			case err := <-fatal:
				return err
			default:
				return ctx.Err()
			}
		case <-ready:
			synced = true
			ready = nil
			if err := draw(); err != nil {
				return err
			}
		case <-ticker.C:
			connection, streams := state.snapshot()
			if streams != noticeStreams {
				lastNotice, noticeStreams = "", streams
			}
			if connection.LastError != "" && connection.LastError != lastNotice {
				if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "CronJob watch reconnecting: %s\n", connection.LastError); err != nil {
					return err
				}
				lastNotice = connection.LastError
			}
			if synced {
				if err := draw(); err != nil {
					return err
				}
			}
		}
	}
}

// Before the first stream, an access error means missing permission. Afterwards
// it can come from an API server that is still syncing authorization after a
// restart, so it is retried and reported like other connection errors.
func fatalWatchError(err error, streamed bool) bool {
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return !streamed
	}
	return apierrors.IsBadRequest(err) || apierrors.IsInvalid(err) || apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err)
}

func (o *options) watchFrame(cmd *cobra.Command, r model.Report) error {
	out := cmd.OutOrStdout()
	switch o.output {
	case "json":
		e := json.NewEncoder(out)
		e.SetEscapeHTML(false)
		return e.Encode(r)
	case "yaml":
		if _, err := io.WriteString(out, "---\n"); err != nil {
			return err
		}
		return r.Encode(out, "yaml")
	default:
		if redrawTerminal(out) {
			if _, err := io.WriteString(out, "\x1b[H\x1b[2J"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(out, "Observed CronJobs | %s | watch %s\n", r.GeneratedAt.Format(time.RFC3339), r.Watch.State); err != nil {
			return err
		}
		if r.Watch.LastError != "" {
			if _, err := fmt.Fprintf(out, "Connection: %s\n", r.Watch.LastError); err != nil {
				return err
			}
		}
		return render.Human(out, cmd.ErrOrStderr(), r, render.Options{Wide: o.output == "wide", Namespaces: o.allNamespaces, NoFindings: o.noFindings, Color: colorEnabled(o.color, out)})
	}
}

func redrawTerminal(out io.Writer) bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := out.(interface{ Fd() uintptr })
	return ok && (isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd()))
}
