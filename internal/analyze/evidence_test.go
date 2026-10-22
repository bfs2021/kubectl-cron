// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package analyze

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRunUsesJobConditionsAndRecordedTimes(t *testing.T) {
	slot := time.Date(2026, 11, 5, 8, 0, 0, 0, time.UTC)
	start := metav1.NewTime(slot.Add(3 * time.Second))
	end := metav1.NewTime(start.Add(90 * time.Second))
	job := batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "arbitrary", CreationTimestamp: metav1.NewTime(slot.Add(2 * time.Second)), Annotations: map[string]string{batchv1.CronJobScheduledTimestampAnnotation: slot.Format(time.RFC3339)}}, Status: batchv1.JobStatus{StartTime: &start, Active: 1, Failed: 2}}
	r := Run(job)
	if r.State != "running" || r.Duration.Seconds != nil || r.SchedulingDelay.Seconds == nil || *r.SchedulingDelay.Seconds != 2 || r.Origin != "scheduled" {
		t.Fatalf("running with failed retry Pods: %+v", r)
	}
	job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailureTarget, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded", LastTransitionTime: end}}
	r = Run(job)
	if r.State != "terminating" || r.Finished.Time != nil {
		t.Fatalf("FailureTarget is not terminal: %+v", r)
	}
	job.Status.Conditions[0].Type = batchv1.JobFailed
	r = Run(job)
	if r.State != "failed" || r.Duration.Seconds == nil || *r.Duration.Seconds != 90 || r.FinishSource != "terminal-condition" {
		t.Fatalf("failed Job without CompletionTime: %+v", r)
	}
	job.Status.Conditions[0].Type = batchv1.JobComplete
	job.Status.CompletionTime = &end
	job.Annotations["cronjob.kubernetes.io/instantiate"] = "manual"
	r = Run(job)
	if r.State != "complete" || r.Origin != "manual" || r.FailedPods != 2 || r.Duration.Seconds == nil || *r.Duration.Seconds != 90 || r.FinishSource != "completion-time" {
		t.Fatalf("successful manual Job with failed retries: %+v", r)
	}
	manager := "example.com/controller"
	job.Spec.ManagedBy = &manager
	r = Run(job)
	if r.State != "external-controller" || r.Duration.Seconds != nil || r.Finished.Time != nil {
		t.Fatalf("external controller interpreted as built-in: %+v", r)
	}
}
