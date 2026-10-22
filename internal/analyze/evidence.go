// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package analyze

import (
	"sort"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/collect"
	"github.com/mertdotcc/kubectl-cron/internal/model"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Runs(jobs []batchv1.Job) []model.Run {
	runs := make([]model.Run, 0, len(jobs))
	for _, job := range jobs {
		runs = append(runs, Run(job))
	}
	sort.SliceStable(runs, func(i, j int) bool {
		if c := compareTimes(runs[i].Created.Time, runs[j].Created.Time, true); c != 0 {
			return c < 0
		}
		return runs[i].Name < runs[j].Name
	})
	return runs
}

func Run(job batchv1.Job) model.Run {
	r := model.Run{Name: job.Name, Namespace: job.Namespace, UID: string(job.UID), Origin: "unknown", State: "pending", ManagedBy: batchv1.JobControllerName, Created: recordMeta(&job.CreationTimestamp), Slot: model.Timestamp{Reason: "not-recorded"}, ControllerStart: recordMeta(job.Status.StartTime), Finished: model.Timestamp{Reason: "not-recorded"}, ActivePods: job.Status.Active, SucceededPods: job.Status.Succeeded, FailedPods: job.Status.Failed}
	if job.Annotations["cronjob.kubernetes.io/instantiate"] == "manual" {
		r.Origin = "manual"
	}
	if raw := job.Annotations[batchv1.CronJobScheduledTimestampAnnotation]; raw != "" {
		if at, err := time.Parse(time.RFC3339, raw); err == nil {
			r.Slot = model.Recorded(&at)
			if r.Origin != "manual" {
				r.Origin = "scheduled"
			}
		} else {
			r.Slot.Reason = "invalid-recorded-timestamp"
		}
	}
	r.SchedulingDelay = measure(r.Slot.Time, r.Created.Time, "object-creation-minus-slot")
	if job.Spec.ManagedBy != nil {
		r.ManagedBy = *job.Spec.ManagedBy
	}
	if r.ManagedBy != batchv1.JobControllerName {
		r.State = "external-controller"
		r.Duration = model.Measure{Reason: "external-controller"}
		r.Finished = model.Timestamp{Reason: "external-controller"}
		return r
	}
	var complete, failed *batchv1.JobCondition
	for i := range job.Status.Conditions {
		c := &job.Status.Conditions[i]
		if c.Status != corev1.ConditionTrue {
			continue
		}
		switch c.Type {
		case batchv1.JobComplete:
			complete = c
		case batchv1.JobFailed:
			failed = c
		case batchv1.JobSuspended:
			r.State = "suspended"
		case batchv1.JobFailureTarget, batchv1.JobSuccessCriteriaMet:
			r.State = "terminating"
			r.Reason = c.Reason
			r.Message = c.Message
		}
	}
	if complete != nil && failed != nil {
		r.State = "unknown"
		r.Reason = "conflicting-terminal-conditions"
	} else {
		terminal := complete
		if failed != nil {
			r.State = "failed"
			terminal = failed
		} else if complete != nil {
			r.State = "complete"
		}
		if terminal != nil {
			r.Reason = terminal.Reason
			r.Message = terminal.Message
			r.Finished = recordMeta(&terminal.LastTransitionTime)
			r.FinishSource = "terminal-condition"
			if complete != nil && job.Status.CompletionTime != nil {
				r.Finished = recordMeta(job.Status.CompletionTime)
				r.FinishSource = "completion-time"
			}
		} else if r.State == "pending" && job.Status.Active > 0 {
			r.State = "running"
		}
	}
	r.Duration = measure(r.ControllerStart.Time, r.Finished.Time, "job-controller-observed")
	if r.Duration.Seconds != nil && *r.Duration.Seconds < 0 {
		r.Duration = model.Measure{Reason: "inconsistent-timestamps", Basis: "job-controller-observed"}
	}
	return r
}

func recordMeta(t *metav1.Time) model.Timestamp {
	if t == nil {
		return model.Recorded(nil)
	}
	return model.Recorded(&t.Time)
}
func measure(start, end *time.Time, basis string) model.Measure {
	if start == nil || end == nil {
		return model.Measure{Reason: "not-recorded", Basis: basis}
	}
	seconds := float64(end.Unix()-start.Unix()) + float64(end.Nanosecond()-start.Nanosecond())/1e9
	return model.Measure{Seconds: &seconds, Basis: basis}
}

func Inspection(e collect.Evidence, now time.Time, opts Options, limit int) model.Report {
	base := Overview([]batchv1.CronJob{e.CronJob}, "cluster", now, opts)
	runs := Runs(e.Jobs)
	view := model.Inspection{CronJob: base.Rows()[0], Runs: runs, Events: []model.Event{}, Evidence: e.Availability}
	view.CronJob.Findings = Diagnose(e, now, opts).Findings
	for _, source := range e.Availability {
		if source.Resource == "jobs" && source.State == "available" {
			total := len(runs)
			view.RetainedRuns = &total
		}
	}
	if len(runs) > limit {
		view.Runs = runs[:limit]
		view.RunsTruncated = true
	}
	view.Events = Events(e.Events)
	return model.Report{APIVersion: model.APIVersion, Kind: "CronJobInspection", GeneratedAt: now.UTC(), Source: "cluster", Inspection: &view}
}

func Events(events []corev1.Event) []model.Event {
	result := []model.Event{}
	for _, event := range events {
		at := recordMeta(&event.LastTimestamp)
		observations := event.Count
		if !event.EventTime.IsZero() && (at.Time == nil || event.EventTime.After(*at.Time)) {
			t := event.EventTime.Time
			at = model.Recorded(&t)
		}
		if event.Series != nil {
			if !event.Series.LastObservedTime.IsZero() && (at.Time == nil || event.Series.LastObservedTime.After(*at.Time)) {
				t := event.Series.LastObservedTime.Time
				at = model.Recorded(&t)
			}
			observations = event.Series.Count
		}
		if at.Time == nil {
			at = recordMeta(&event.FirstTimestamp)
		}
		if at.Time == nil {
			at = recordMeta(&event.CreationTimestamp)
		}
		result = append(result, model.Event{Name: event.Name, Type: event.Type, Reason: event.Reason, Message: event.Message, Observations: observations, ObservedAt: at})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if c := compareTimes(result[i].ObservedAt.Time, result[j].ObservedAt.Time, true); c != 0 {
			return c < 0
		}
		return result[i].Name < result[j].Name
	})
	return result
}
