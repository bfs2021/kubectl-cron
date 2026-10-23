// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package analyze

import (
	"strings"
	"testing"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/collect"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDiagnosticsDoNotInferFailureOrReplacementFromWeakEvidence(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 12, 0, 0, time.UTC)
	zone := "UTC"
	cj := batchv1.CronJob{Spec: batchv1.CronJobSpec{Schedule: "*/10 * * * *", TimeZone: &zone, ConcurrencyPolicy: batchv1.ReplaceConcurrent}}
	job := batchv1.Job{Status: batchv1.JobStatus{Failed: 3, Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}}}
	e := collect.Evidence{CronJob: cj, Jobs: []batchv1.Job{job}, Events: []corev1.Event{{Reason: "SuccessfulDelete", Type: "Normal"}}}
	d := Diagnose(e, now, Options{})
	if len(d.Findings) != 0 {
		t.Fatalf("invented failure/replacement: %+v", d)
	}
	e.Events = append(e.Events, corev1.Event{Reason: "TooManyMissedTimes", Type: "Warning", Message: "many missed start times", LastTimestamp: metav1.NewTime(now)})
	d = Diagnose(e, now, Options{})
	if len(d.Findings) != 1 || d.Findings[0].Rule != "too-many-missed-times-event" || strings.Contains(d.Findings[0].Message, "gave up") {
		t.Fatalf("controller behavior invented: %+v", d)
	}
}

func TestUnexpectedJobEventNotesOwnedManualRuns(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 12, 0, 0, time.UTC)
	zone := "UTC"
	cj := batchv1.CronJob{Spec: batchv1.CronJobSpec{Schedule: "*/10 * * * *", TimeZone: &zone}}
	manual := batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "held", Annotations: map[string]string{"cronjob.kubernetes.io/instantiate": "manual"}}}
	scheduled := batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "ledger-29000000", Annotations: map[string]string{batchv1.CronJobScheduledTimestampAnnotation: now.Add(-10 * time.Minute).Format(time.RFC3339)}}}
	unexpected := func(job string, minutes int) corev1.Event {
		return corev1.Event{Reason: "UnexpectedJob", Type: "Warning", Message: "Saw a job that the controller did not create or forgot: " + job, LastTimestamp: metav1.NewTime(now.Add(-time.Duration(minutes) * time.Minute))}
	}
	e := collect.Evidence{CronJob: cj, Jobs: []batchv1.Job{manual, scheduled}, Events: []corev1.Event{unexpected("held", 1), unexpected("ledger-29000000", 2), unexpected("gone", 3)}}
	d := Diagnose(e, now, Options{})
	if len(d.Findings) != 3 {
		t.Fatalf("%+v", d.Findings)
	}
	for i, finding := range d.Findings {
		noted := strings.Contains(finding.Message, "The named Job is a retained owned manual Run.")
		if finding.Rule != "controller-warning-event" || noted != (i == 0) {
			t.Fatalf("finding %d: %+v", i, finding)
		}
	}
}
