// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/cli"
	"github.com/mertdotcc/kubectl-cron/internal/model"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type snapshotWriter struct {
	mu      sync.Mutex
	pending bytes.Buffer
	frames  chan model.Report
}

func (w *snapshotWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending.Write(p)
	for {
		data := w.pending.Bytes()
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		var r model.Report
		if err := json.Unmarshal(data[:i], &r); err != nil {
			return 0, err
		}
		w.pending.Next(i + 1)
		select {
		case w.frames <- r:
		default:
			return 0, fmt.Errorf("test snapshot consumer fell behind")
		}
	}
	return len(p), nil
}
func nextSnapshot(t *testing.T, w *snapshotWriter, match func(model.Report) bool) model.Report {
	t.Helper()
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	for {
		select {
		case r := <-w.frames:
			if match(r) {
				return r
			}
		case <-timer.C:
			t.Fatal("watch did not produce expected snapshot")
			return model.Report{}
		}
	}
}
func startWatch(t *testing.T, config string, out io.Writer, args ...string) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	cmd := cli.New(strings.NewReader(""), out, io.Discard, func() time.Time { return reference }, "")
	cmd.SetArgs(append([]string{"--kubeconfig", config, "--watch", "--watch-interval=250ms"}, args...))
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	t.Cleanup(cancel)
	return cancel, done
}
func stopWatch(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("watch cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watch leaked after cancellation")
	}
}
func listResponse(w http.ResponseWriter, rv string, items ...batchv1.CronJob) {
	json.NewEncoder(w).Encode(batchv1.CronJobList{TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1", Kind: "CronJobList"}, ListMeta: metav1.ListMeta{ResourceVersion: rv}, Items: items})
}
func watchEvent(w http.ResponseWriter, kind string, obj any) {
	json.NewEncoder(w).Encode(map[string]any{"type": kind, "object": obj})
	w.(http.Flusher).Flush()
}

func TestWatchUsesResourceVersionSelectorsAndCachedTicks(t *testing.T) {
	f := fixtureData()
	cj := f.CronJob
	cj.ResourceVersion = "10"
	type event struct {
		kind   string
		object batchv1.CronJob
	}
	events := make(chan event, 4)
	var lists, watches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/apis/batch/v1/namespaces/finance/cronjobs" {
			t.Errorf("unexpected API access %s %s", r.Method, r.URL)
		}
		q := r.URL.Query()
		if q.Get("labelSelector") != "team=ledger" || q.Get("fieldSelector") != "metadata.name=ledger" {
			t.Errorf("selectors lost: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		if q.Get("watch") != "true" {
			lists.Add(1)
			if q.Get("resourceVersion") != "" {
				t.Errorf("initial list may be stale: %s", r.URL)
			}
			listResponse(w, "10", cj)
			return
		}
		watches.Add(1)
		if q.Get("resourceVersion") != "10" || q.Get("allowWatchBookmarks") != "true" {
			t.Errorf("watch not based on the list: %s", r.URL)
		}
		w.(http.Flusher).Flush()
		for {
			select {
			case e := <-events:
				watchEvent(w, e.kind, e.object)
			case <-r.Context().Done():
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	out := &snapshotWriter{frames: make(chan model.Report, 64)}
	cancel, done := startWatch(t, kubeconfig(t, server.URL), out, "ledger", "-l", "team=ledger", "-o", "json")
	nextSnapshot(t, out, func(r model.Report) bool { return len(r.Rows()) == 1 && r.Watch != nil && r.Watch.State == "streaming" })
	changed := *cj.DeepCopy()
	yes := true
	changed.Spec.Suspend = &yes
	changed.ResourceVersion = "11"
	events <- event{"MODIFIED", changed}
	nextSnapshot(t, out, func(r model.Report) bool { return len(r.Rows()) == 1 && r.Rows()[0].Suspended })
	changed.ResourceVersion = "12"
	events <- event{"DELETED", changed}
	nextSnapshot(t, out, func(r model.Report) bool { return len(r.Rows()) == 0 && r.Summary != nil && r.Summary.Total == 0 })
	for i := 0; i < 2; i++ {
		nextSnapshot(t, out, func(r model.Report) bool { return len(r.Rows()) == 0 })
	}
	stopWatch(t, cancel, done)
	if lists.Load() != 1 || watches.Load() != 1 {
		t.Fatalf("countdown polled the API: lists=%d watches=%d", lists.Load(), watches.Load())
	}
}

func TestWatchExpiredVersionRelistsAndRemovesDeletedObjects(t *testing.T) {
	cj := fixtureData().CronJob
	cj.ResourceVersion = "10"
	ghost := *cj.DeepCopy()
	ghost.Name = "removed"
	expire := make(chan struct{})
	var lists, watches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("watch") != "true" {
			if lists.Add(1) == 1 {
				listResponse(w, "10", cj, ghost)
			} else {
				current := *cj.DeepCopy()
				yes := true
				current.Spec.Suspend = &yes
				current.ResourceVersion = "20"
				listResponse(w, "20", current)
			}
			return
		}
		n := watches.Add(1)
		rv := r.URL.Query().Get("resourceVersion")
		if (n == 1 && rv != "10") || (n > 1 && rv != "20") {
			t.Errorf("incorrect resumed RV %q", rv)
		}
		w.(http.Flusher).Flush()
		if n == 1 {
			select {
			case <-expire:
				watchEvent(w, "ERROR", metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: "Failure", Reason: metav1.StatusReasonExpired, Code: 410, Message: "expired test version"})
			case <-r.Context().Done():
			}
			return
		}
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	out := &snapshotWriter{frames: make(chan model.Report, 64)}
	cancel, done := startWatch(t, kubeconfig(t, server.URL), out, "-o", "json")
	nextSnapshot(t, out, func(r model.Report) bool { return len(r.Rows()) == 2 })
	close(expire)
	nextSnapshot(t, out, func(r model.Report) bool {
		return len(r.Rows()) == 1 && r.Rows()[0].Suspended && r.Watch.State == "streaming"
	})
	stopWatch(t, cancel, done)
	if lists.Load() < 2 || watches.Load() < 2 {
		t.Fatalf("expired version did not relist: %d/%d", lists.Load(), watches.Load())
	}
}

func TestWatchAccessErrorsAndOutputFailures(t *testing.T) {
	for _, deny := range []string{"list", "watch", "event", "none"} {
		t.Run(deny, func(t *testing.T) {
			cj := fixtureData().CronJob
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				watch := r.URL.Query().Get("watch") == "true"
				if (deny == "list" && !watch) || (deny == "watch" && watch) {
					w.WriteHeader(http.StatusForbidden)
					json.NewEncoder(w).Encode(metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: "Failure", Reason: metav1.StatusReasonForbidden, Code: 403, Message: "watch test denial"})
					return
				}
				if !watch {
					listResponse(w, "10", cj)
					return
				}
				w.(http.Flusher).Flush()
				if deny == "event" {
					watchEvent(w, "ERROR", metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: "Failure", Reason: metav1.StatusReasonForbidden, Code: 403, Message: "watch test denial"})
					return
				}
				<-r.Context().Done()
			}))
			t.Cleanup(server.Close)
			var buf bytes.Buffer
			var out io.Writer = &buf
			if deny == "none" {
				out = brokenWriter{}
			}
			cancel, done := startWatch(t, kubeconfig(t, server.URL), out, "-o", "json")
			defer cancel()
			select {
			case err := <-done:
				if cli.ExitCode(err) != 1 {
					t.Fatalf("error hidden: %v", err)
				}
				if deny == "none" && !errors.Is(err, io.ErrClosedPipe) {
					t.Fatal(err)
				}
				if deny != "none" && !strings.Contains(err.Error(), "watch test denial") {
					t.Fatal(err)
				}
				if deny == "list" && buf.Len() != 0 {
					t.Fatal("forbidden list printed success", buf.String())
				}
			case <-time.After(4 * time.Second):
				t.Fatal("watch did not stop on fatal error")
			}
		})
	}
}

func TestWatchPipedFormatsAndFindingExit(t *testing.T) {
	cj := fixtureData().CronJob
	cj.Spec.Schedule = "invalid"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("watch") != "true" {
			listResponse(w, "10", cj)
			return
		}
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	config := kubeconfig(t, server.URL)
	for _, format := range []string{"yaml", "table"} {
		t.Run(format, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			out := &cancelWriter{cancel: cancel}
			cmd := cli.New(strings.NewReader(""), out, io.Discard, func() time.Time { return reference }, "")
			cmd.SetArgs([]string{"--watch", "--kubeconfig", config, "-o", format})
			if err := cmd.ExecuteContext(ctx); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			data := out.String()
			if strings.Contains(data, "\x1b[") {
				t.Fatal("piped watch emitted terminal controls", data)
			}
			if format == "yaml" {
				var report model.Report
				if !strings.HasPrefix(data, "---\n") {
					t.Fatal("missing YAML document separator", data)
				}
				if err := yaml.Unmarshal([]byte(data), &report); err != nil || report.Watch == nil || report.Kind != "CronJobReport" {
					t.Fatalf("invalid YAML frame: %v %s", err, data)
				}
			} else if !strings.Contains(data, "Observed CronJobs") || !strings.Contains(data, "ledger") {
				t.Fatal(data)
			}
		})
	}
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := cli.New(strings.NewReader(""), &out, io.Discard, func() time.Time { return reference }, "")
	cmd.SetArgs([]string{"--watch", "--kubeconfig", config, "--summary=only", "--exit-on-findings", "-o", "json"})
	if err := cmd.ExecuteContext(ctx); cli.ExitCode(err) != 2 {
		t.Fatalf("watch did not stop on Findings: %v", err)
	}
	var report model.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.Summary == nil || report.Summary.Total != 1 || report.CronJobs != nil {
		t.Fatalf("invalid summary frame: %v %s", err, out.String())
	}
}

func TestWatchRejectsOfflineInputAndInvalidIntervals(t *testing.T) {
	for _, args := range [][]string{{"--watch", "-f", "../testdata/cronjobs.yaml"}, {"--watch", "--watch-interval=10ms"}, {"--watch", "--watch-interval=2m"}, {"--watch-interval=1s"}} {
		out, _, err := command("", args...)
		if cli.ExitCode(err) != 1 || out != "" {
			t.Fatalf("invalid watch accepted %v: %v %s", args, err, out)
		}
	}
}
