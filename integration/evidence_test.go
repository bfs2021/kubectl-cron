// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/cli"
	"github.com/mertdotcc/kubectl-cron/internal/model"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type evidenceFixture struct {
	CronJob batchv1.CronJob
	Jobs    []batchv1.Job
	Events  []corev1.Event
	mu      sync.Mutex
	Paths   []string
	Handle  func(http.ResponseWriter, *http.Request) bool
}

func timeMeta(t time.Time) *metav1.Time { v := metav1.NewTime(t); return &v }

func fixtureData() *evidenceFixture {
	zone, controller := "UTC", true
	cj := batchv1.CronJob{TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1", Kind: "CronJob"}, ObjectMeta: metav1.ObjectMeta{Name: "ledger", Namespace: "finance", UID: "cj-current", CreationTimestamp: *timeMeta(reference.Add(-48 * time.Hour))}, Spec: batchv1.CronJobSpec{Schedule: "*/10 * * * *", TimeZone: &zone, ConcurrencyPolicy: batchv1.ForbidConcurrent}}
	makeJob := func(name string, uid types.UID, slot time.Time) batchv1.Job {
		return batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "finance", UID: uid, CreationTimestamp: *timeMeta(slot.Add(2 * time.Second)), Annotations: map[string]string{batchv1.CronJobScheduledTimestampAnnotation: slot.Format(time.RFC3339)}, OwnerReferences: []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "CronJob", Name: "ledger", UID: cj.UID, Controller: &controller}}}, Status: batchv1.JobStatus{StartTime: timeMeta(slot.Add(3 * time.Second))}}
	}
	complete := makeJob("ledger-complete", "job-complete", reference.Add(-22*time.Minute))
	complete.Status.Failed = 2
	complete.Status.CompletionTime = timeMeta(reference.Add(-21 * time.Minute))
	complete.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue, LastTransitionTime: *complete.Status.CompletionTime}}
	failed := makeJob("ledger-failed", "job-failed", reference.Add(-12*time.Minute))
	failed.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded", LastTransitionTime: *timeMeta(reference.Add(-9 * time.Minute))}}
	manual := makeJob("ad-hoc", "job-manual", reference.Add(-7*time.Minute))
	manual.Annotations = map[string]string{"cronjob.kubernetes.io/instantiate": "manual"}
	manual.Status.CompletionTime = timeMeta(reference.Add(-6 * time.Minute))
	manual.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue, LastTransitionTime: *manual.Status.CompletionTime}}
	active := makeJob("ledger-active", "job-active", reference.Add(-2*time.Minute))
	active.Status.Active = 1
	old := makeJob("ledger-old-owner", "old-job", reference.Add(-time.Minute))
	old.OwnerReferences[0].UID = "cj-old"
	unowned := makeJob("ledger-unowned", "unowned-job", reference.Add(-time.Minute))
	unowned.OwnerReferences = nil
	cj.Status.Active = []corev1.ObjectReference{{Name: active.Name, Namespace: active.Namespace, UID: active.UID}}
	cj.Status.LastScheduleTime = timeMeta(reference.Add(-2 * time.Minute))
	cj.Status.LastSuccessfulTime = manual.Status.CompletionTime
	event := corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "active-event", Namespace: "finance"}, InvolvedObject: corev1.ObjectReference{Kind: "CronJob", UID: cj.UID}, Type: "Normal", Reason: "JobAlreadyActive", Message: "Not starting job because prior execution is running", Count: 4, LastTimestamp: *timeMeta(reference.Add(-time.Minute)), FirstTimestamp: *timeMeta(reference.Add(-5 * time.Minute)), EventTime: metav1.NewMicroTime(reference.Add(-5 * time.Minute))}
	oldEvent := event.DeepCopy()
	oldEvent.Name = "old-event"
	oldEvent.InvolvedObject.UID = "cj-old"
	return &evidenceFixture{CronJob: cj, Jobs: []batchv1.Job{complete, failed, manual, active, old, unowned}, Events: []corev1.Event{event, *oldEvent}}
}

func serveEvidence(t *testing.T, f *evidenceFixture, deny string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.Paths = append(f.Paths, r.URL.Path)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method != "GET" {
			t.Errorf("unexpected write: %s", r.Method)
		}
		if deny != "" && strings.HasSuffix(r.URL.Path, "/"+deny) {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","message":"evidence access denied","code":403}`)
			return
		}
		if f.Handle != nil && f.Handle(w, r) {
			return
		}
		var data any
		switch r.URL.Path {
		case "/apis/batch/v1/namespaces/finance/cronjobs/ledger":
			data = f.CronJob
		case "/apis/batch/v1/namespaces/finance/cronjobs":
			data = batchv1.CronJobList{Items: []batchv1.CronJob{f.CronJob}}
		case "/apis/batch/v1/namespaces/finance/jobs":
			data = batchv1.JobList{Items: f.Jobs}
		case "/api/v1/namespaces/finance/events":
			if r.URL.Query().Get("fieldSelector") != "involvedObject.uid=cj-current" {
				t.Errorf("event query must be UID-scoped: %s", r.URL)
			}
			data = corev1.EventList{Items: f.Events}
		default:
			t.Errorf("unexpected access: %s", r.URL)
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(data); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	return kubeconfig(t, server.URL)
}

func kubeconfig(t *testing.T, serverURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	data := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: test\n  cluster:\n    server: %s\ncontexts:\n- name: test\n  context:\n    cluster: test\n    user: test\n    namespace: finance\ncurrent-context: test\nusers:\n- name: test\n  user: {}\n", serverURL)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInspectionOwnsByUIDAndPreservesEvidence(t *testing.T) {
	f := fixtureData()
	config := serveEvidence(t, f, "")
	out, _, err := command("", "inspect", "ledger", "--kubeconfig", config, "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var r model.Report
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	v := r.Inspection
	if r.Kind != "CronJobInspection" || v == nil || len(v.Runs) != 4 || v.RetainedRuns == nil || *v.RetainedRuns != 4 || len(v.Events) != 1 {
		t.Fatal(out)
	}
	if v.Events[0].ObservedAt.Time == nil || !v.Events[0].ObservedAt.Time.Equal(reference.Add(-time.Minute)) {
		t.Fatal("latest observation lost", out)
	}
	for _, run := range v.Runs {
		if run.Name == "ad-hoc" && (run.Origin != "manual" || run.Slot.Time != nil) {
			t.Fatal("owned manual run lost", out)
		}
		if run.Name == "ledger-complete" && (run.State != "complete" || run.FailedPods != 2) {
			t.Fatal("retry Pods treated as failed Job", out)
		}
	}
	out, notes, err := command("", "inspect", "ledger", "--kubeconfig", config, "--limit=2", "--color=never")
	if err != nil || !strings.Contains(notes, "Showing 2 of 4") || !strings.Contains(out, "Retained Jobs") {
		t.Fatalf("%v\n%s\n%s", err, out, notes)
	}
	out, _, err = command("", "inspect", "ledger", "-f", "../testdata/cronjobs.yaml")
	if cli.ExitCode(err) != 1 || out != "" {
		t.Fatalf("offline input hydrated: %v %s", err, out)
	}
}

func TestInspectionUnavailableEvidenceIsNotZero(t *testing.T) {
	for _, resource := range []string{"jobs", "events"} {
		t.Run(resource, func(t *testing.T) {
			config := serveEvidence(t, fixtureData(), resource)
			out, _, err := command("", "inspect", "ledger", "--kubeconfig", config, "-o", "yaml")
			if err != nil || !strings.Contains(out, "state: forbidden") || (resource == "jobs" && !strings.Contains(out, "retainedRuns: null")) {
				t.Fatalf("%v\n%s", err, out)
			}
			out, _, err = command("", "inspect", "ledger", "--kubeconfig", config)
			if err != nil || !strings.Contains(out, "Unavailable; see evidence reads below.") {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
}
