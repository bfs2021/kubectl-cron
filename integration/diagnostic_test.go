// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/mertdotcc/kubectl-cron/internal/cli"
	"github.com/mertdotcc/kubectl-cron/internal/model"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestDiagnosticsShareJobReadsAndScopeEvents(t *testing.T) {
	f := fixtureData()
	other := f.CronJob.DeepCopy()
	other.Name = "other"
	other.UID = "cj-other"
	third := f.CronJob.DeepCopy()
	third.Name = "third"
	third.UID = "cj-third"
	var mu sync.Mutex
	eventReads := map[string]int{}
	f.Handle = func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/apis/batch/v1/namespaces/finance/cronjobs":
			json.NewEncoder(w).Encode(batchv1.CronJobList{Items: []batchv1.CronJob{f.CronJob, *other, *third}})
			return true
		case "/api/v1/namespaces/finance/events":
			uid := strings.TrimPrefix(r.URL.Query().Get("fieldSelector"), "involvedObject.uid=")
			mu.Lock()
			eventReads[uid]++
			mu.Unlock()
			json.NewEncoder(w).Encode(corev1.EventList{Items: f.Events})
			return true
		}
		return false
	}
	config := serveEvidence(t, f, "")
	out, _, err := command("", "diagnose", "--kubeconfig", config, "-o", "json", "--exit-on-findings")
	if cli.ExitCode(err) != 2 {
		t.Fatalf("%v\n%s", err, out)
	}
	var report model.Report
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Kind != "CronJobDiagnostics" || report.Diagnostics == nil || len(*report.Diagnostics) != 3 {
		t.Fatal(out)
	}
	rows := *report.Diagnostics
	if len(rows[0].Findings) != 2 || rows[0].Findings[0].Rule != "retained-job-failures" || rows[0].Findings[1].Rule != "job-already-active-event" || len(rows[1].Findings) != 0 || len(rows[2].Findings) != 0 {
		t.Fatal("UID evidence leaked or terminal state lost", out)
	}
	jobReads := 0
	for _, path := range f.Paths {
		if strings.HasSuffix(path, "/jobs") {
			jobReads++
		}
	}
	if jobReads != 1 || len(eventReads) != 3 || eventReads["cj-current"] != 1 || eventReads["cj-other"] != 1 || eventReads["cj-third"] != 1 {
		t.Fatalf("read cost incorrect jobs=%d events=%v", jobReads, eventReads)
	}
}

func TestDiagnosticsOfflineAndPartialEvidence(t *testing.T) {
	t.Setenv("KUBECONFIG", "/definitely/unavailable/config")
	out, _, err := command("", "diagnose", "-f", "../testdata/cronjobs.yaml", "-o", "yaml")
	if err != nil || !strings.Contains(out, "offline-source") || !strings.Contains(out, "source: file") {
		t.Fatalf("offline diagnosis failed: %v\n%s", err, out)
	}
	config := serveEvidence(t, fixtureData(), "jobs")
	out, _, err = command("", "diagnose", "ledger", "--kubeconfig", config, "--no-findings", "-o", "json")
	if err != nil || strings.Contains(out, `"findings"`) || !strings.Contains(out, `"state": "forbidden"`) {
		t.Fatalf("partial evidence hidden: %v\n%s", err, out)
	}
	out, notes, err := command("", "diagnose", "ledger", "--kubeconfig", config, "--no-findings")
	if err != nil || out != "" || !strings.Contains(notes, "permission-denied") || !strings.Contains(notes, "remains unknown") {
		t.Fatalf("empty output implied health: %v %s %s", err, out, notes)
	}
}

func TestRequireEvidenceFailsOnIncompleteReads(t *testing.T) {
	for _, deny := range []string{"jobs", "events"} {
		config := serveEvidence(t, fixtureData(), deny)
		for _, args := range [][]string{{"diagnose", "ledger"}, {"inspect", "ledger"}} {
			out, _, err := command("", append(args, "--kubeconfig", config, "--require-evidence", "--exit-on-findings", "-o", "json")...)
			if cli.ExitCode(err) != 1 || !strings.Contains(err.Error(), deny+" forbidden (permission-denied)") {
				t.Fatalf("%v with %s denied: %v", args, deny, err)
			}
			if !strings.Contains(out, `"state": "forbidden"`) {
				t.Fatalf("report withheld: %s", out)
			}
		}
	}
	config := serveEvidence(t, fixtureData(), "")
	for _, args := range [][]string{{"diagnose", "ledger"}, {"inspect", "ledger"}} {
		if _, _, err := command("", append(args, "--kubeconfig", config, "--require-evidence")...); err != nil {
			t.Fatalf("complete evidence rejected: %v", err)
		}
		if _, _, err := command("", append(args, "--kubeconfig", config, "--require-evidence", "--exit-on-findings")...); cli.ExitCode(err) != 2 {
			t.Fatalf("Findings exit lost: %v", err)
		}
	}
	t.Setenv("KUBECONFIG", "/definitely/unavailable/config")
	out, _, err := command("", "diagnose", "-f", "../testdata/cronjobs.yaml", "--require-evidence")
	if cli.ExitCode(err) != 1 || out != "" {
		t.Fatalf("offline input cannot satisfy required evidence: %v %s", err, out)
	}
}
