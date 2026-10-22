// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mertdotcc/kubectl-cron/internal/cli"
	"github.com/mertdotcc/kubectl-cron/internal/model"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCalendarGroupsInstantsAndPreservesCompleteBoundaryGroups(t *testing.T) {
	t.Setenv("KUBECONFIG", "/unavailable/offline/config")
	f := fixtureData()
	f.CronJob.Spec.Schedule = "* * * * *"
	other := f.CronJob.DeepCopy()
	other.Name = "tokyo"
	zone := "Asia/Tokyo"
	other.Spec.TimeZone = &zone
	unknown := f.CronJob.DeepCopy()
	unknown.Name = "unknown"
	unknown.Spec.TimeZone = nil
	suspended := f.CronJob.DeepCopy()
	suspended.Name = "paused"
	yes := true
	suspended.Spec.Suspend = &yes
	data, err := json.Marshal(batchv1.CronJobList{TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1", Kind: "CronJobList"}, Items: []batchv1.CronJob{f.CronJob, *other, *unknown, *suspended}})
	if err != nil {
		t.Fatal(err)
	}
	var report model.Report
	for _, limit := range []string{"1", "100"} {
		out, _, err := command(string(data), "calendar", "-f", "-", "--next=2m", "--limit="+limit, "--exit-on-findings", "-o", "json")
		if cli.ExitCode(err) != 2 {
			t.Fatalf("collision exit missing: %v\n%s", err, out)
		}
		if err := json.Unmarshal([]byte(out), &report); err != nil {
			t.Fatal(err)
		}
		c := report.Calendar
		if report.Kind != "CronJobCalendar" || c == nil || len(c.Excluded) != 2 || c.Slots[0].Count != 2 || !c.Slots[0].At.Equal(reference.Add(60e9)) {
			t.Fatal(out)
		}
		members := c.Slots[0].CronJobs
		if members[0].Timezone.Name == nil || *members[0].Timezone.Name != "UTC" || *members[1].Timezone.Name != "Asia/Tokyo" || members[0].LocalTime.Format("15:04 -07:00") == members[1].LocalTime.Format("15:04 -07:00") {
			t.Fatal("local timezone provenance lost", out)
		}
		if limit == "1" && (!c.Truncated || len(c.Slots) != 1) {
			t.Fatal("truncation hidden", out)
		}
		if limit == "100" && (c.Truncated || len(c.Slots) != 2) {
			t.Fatal("window end is not inclusive", out)
		}
	}
	out, notes, err := command(string(data), "calendar", "-f", "-", "--next=2m", "--limit=1", "-o", "wide")
	if err != nil || !strings.Contains(out, "Asia/Tokyo") || !strings.Contains(notes, "truncated") || !strings.Contains(notes, "controller-timezone-unknown") {
		t.Fatalf("%v\n%s\n%s", err, out, notes)
	}
	out, _, err = command(string(data), "calendar", "-f", "-", "--next=2m", "--no-findings", "-o", "yaml")
	if err != nil || strings.Contains(out, "findings:") || !strings.Contains(out, "controller-timezone-unknown") {
		t.Fatalf("suppression changed uncertainty: %v\n%s", err, out)
	}
}

func TestCalendarReadsOnlyCronJobsAndValidatesBounds(t *testing.T) {
	f := fixtureData()
	config := serveEvidence(t, f, "")
	out, _, err := command("", "calendar", "--kubeconfig", config, "-o", "json")
	if err != nil || !strings.Contains(out, `"source": "cluster"`) {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(f.Paths) != 1 || !strings.HasSuffix(f.Paths[0], "/cronjobs") {
		t.Fatalf("calendar read related evidence: %v", f.Paths)
	}
	for _, flags := range [][]string{{"--next=0"}, {"--next=-1h"}, {"--next=8761h"}, {"--limit=0"}, {"--limit=1001"}} {
		out, _, err := command("", append([]string{"calendar", "--kubeconfig", config}, flags...)...)
		if cli.ExitCode(err) != 1 || out != "" {
			t.Fatalf("invalid bounds accepted: %v %v %s", flags, err, out)
		}
	}
}
