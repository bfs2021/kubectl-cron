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
