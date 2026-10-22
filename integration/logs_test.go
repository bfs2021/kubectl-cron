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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/cli"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func logFixture(t *testing.T, f *evidenceFixture, stream func(http.ResponseWriter, *http.Request)) string {
	t.Helper()
	f.Handle = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v1/namespaces/finance/pods" {
			uid := strings.TrimPrefix(r.URL.Query().Get("labelSelector"), "batch.kubernetes.io/controller-uid=")
			pods := []corev1.Pod{}
			for _, job := range f.Jobs {
				if string(job.UID) == uid {
					controller := true
					pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: job.Name + "-pod", Namespace: job.Namespace, Annotations: map[string]string{"kubectl.kubernetes.io/default-container": "worker"}, OwnerReferences: []metav1.OwnerReference{{Kind: "Job", UID: job.UID, Controller: &controller}}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "worker"}, {Name: "sidecar"}}, InitContainers: []corev1.Container{{Name: "setup"}}}}
					pods = append(pods, pod)
					if job.Name == "ledger-failed" {
						retry := pod.DeepCopy()
						retry.Name += "-retry"
						pods = append(pods, *retry)
					}
					foreign := pod.DeepCopy()
					foreign.Name = "foreign-pod"
					foreign.OwnerReferences[0].UID = "another-job"
					pods = append(pods, *foreign)
				}
			}
			if len(pods) == 0 {
				t.Errorf("unexpected Pod selector: %q", r.URL.Query().Get("labelSelector"))
			}
			if err := json.NewEncoder(w).Encode(corev1.PodList{Items: pods}); err != nil {
				t.Error(err)
			}
			return true
		}
		if strings.HasSuffix(r.URL.Path, "/log") {
			if strings.Contains(r.URL.Path, "foreign-pod") {
				t.Error("unowned Pod logs requested")
			}
			w.Header().Set("Content-Type", "text/plain")
			stream(w, r)
			return true
		}
		return false
	}
	return serveEvidence(t, f, "")
}

func TestLogRunAndContainerSelection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		flags     []string
		jobs      []string
		container string
		previous  bool
	}{
		{"latest", nil, []string{"ledger-active"}, "worker", false},
		{"older", []string{"--run-offset=1"}, []string{"ad-hoc"}, "worker", false},
		{"failed", []string{"--failed"}, []string{"ledger-failed"}, "worker", false},
		{"explicit", []string{"--run=ledger-complete", "-c", "setup"}, []string{"ledger-complete"}, "setup", false},
		{"all", []string{"--all", "--max-log-requests=1"}, []string{"ledger-complete", "ledger-failed", "ad-hoc", "ledger-active"}, "worker", false},
		{"previous-container", []string{"-f", "--previous", "-c", "sidecar"}, []string{"ledger-active"}, "sidecar", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			seen := map[string]int{}
			config := logFixture(t, fixtureData(), func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("container") != tc.container || (q.Get("previous") == "true") != tc.previous || q.Get("tailLines") != "100" || q.Get("sinceSeconds") != "2" {
					t.Errorf("incorrect options: %s", r.URL)
				}
				mu.Lock()
				seen[r.URL.Path]++
				mu.Unlock()
				fmt.Fprintln(w, "payload")
			})
			args := append([]string{"logs", "ledger", "--kubeconfig", config, "--since=1500ms"}, tc.flags...)
			out, _, err := command("", args...)
			if err != nil || !strings.Contains(out, "payload") {
				t.Fatalf("%v\n%s", err, out)
			}
			for _, job := range tc.jobs {
				if seen["/api/v1/namespaces/finance/pods/"+job+"-pod/log"] != 1 {
					t.Fatalf("missing %s: %v", job, seen)
				}
			}
			want := len(tc.jobs)
			for _, job := range tc.jobs {
				if job == "ledger-failed" {
					want++
				}
			}
			if len(seen) != want {
				t.Fatalf("wrong executions: %v", seen)
			}
			if want > 1 && !strings.Contains(out, "[finance/") {
				t.Fatal("multiplexed output missing identity", out)
			}
		})
	}
}

func TestLogFailuresAreExplicit(t *testing.T) {
	config := logFixture(t, fixtureData(), func(w http.ResponseWriter, r *http.Request) { t.Error("log stream opened for invalid selection") })
	for _, flags := range [][]string{{"--run-offset=-1"}, {"--run-offset=999"}, {"--all", "--run-offset=1"}, {"--run=ledger-active", "--failed"}, {"--container=missing"}, {"--all-containers", "-c", "worker"}, {"--tail=-2"}, {"--since=-1m"}, {"--max-log-requests=0"}, {"--all-containers", "-f", "--max-log-requests=1"}} {
		out, _, err := command("", append([]string{"logs", "ledger", "--kubeconfig", config}, flags...)...)
		if err == nil || out != "" {
			t.Fatalf("invalid log request accepted: %v %v %s", flags, err, out)
		}
	}
	config = serveEvidence(t, fixtureData(), "jobs")
	out, _, err := command("", "logs", "ledger", "--kubeconfig", config)
	if err == nil || out != "" || !strings.Contains(err.Error(), "permission-denied") {
		t.Fatalf("denial hidden: %v %s", err, out)
	}
}

type cancelWriter struct {
	bytes.Buffer
	cancel context.CancelFunc
}

func (w *cancelWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	w.cancel()
	return n, err
}
func (w *cancelWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

func TestMissingPodsAndLogDenial(t *testing.T) {
	f := fixtureData()
	f.Handle = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v1/namespaces/finance/pods" {
			json.NewEncoder(w).Encode(corev1.PodList{Items: []corev1.Pod{}})
			return true
		}
		return false
	}
	config := serveEvidence(t, f, "")
	out, notes, err := command("", "logs", "ledger", "--kubeconfig", config)
	if err == nil || out != "" || !strings.Contains(notes, "cause is not recorded") || !strings.Contains(err.Error(), "no retained owned Pods") {
		t.Fatalf("missing Pods hidden: %v %s %s", err, out, notes)
	}
	config = logFixture(t, fixtureData(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","message":"logs denied","code":403}`)
	})
	out, _, err = command("", "logs", "ledger", "--kubeconfig", config)
	if err == nil || out != "" || !strings.Contains(err.Error(), "logs denied") {
		t.Fatalf("log denial hidden: %v %s", err, out)
	}
}

func TestLogCancellationAndOutputFailure(t *testing.T) {
	config := logFixture(t, fixtureData(), func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "first line")
		w.(http.Flusher).Flush()
		if r.URL.Query().Get("follow") == "true" {
			<-r.Context().Done()
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out := &cancelWriter{cancel: cancel}
	cmd := cli.New(strings.NewReader(""), out, io.Discard, func() time.Time { return reference }, "")
	cmd.SetArgs([]string{"logs", "ledger", "--kubeconfig", config, "-f"})
	if err := cmd.ExecuteContext(ctx); !errors.Is(err, context.Canceled) || !strings.Contains(out.String(), "first line") {
		t.Fatalf("cancellation failed: %v %s", err, out.String())
	}
	cmd = cli.New(strings.NewReader(""), brokenWriter{}, io.Discard, func() time.Time { return reference }, "")
	cmd.SetArgs([]string{"logs", "ledger", "--kubeconfig", config})
	if err := cmd.Execute(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write failure lost: %v", err)
	}
}
