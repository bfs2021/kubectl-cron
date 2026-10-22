// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

// Package analyze derives the Report from CronJobs without fetching evidence.
package analyze

import (
	"sort"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/model"
	"github.com/mertdotcc/kubectl-cron/internal/schedule"
	batchv1 "k8s.io/api/batch/v1"
)

type Options struct {
	AssumeTimezone string
	NoFindings     bool
	Problems       bool
	Sort           string
	Count          int
}

func Overview(jobs []batchv1.CronJob, source string, now time.Time, opts Options) model.Report {
	rows := make([]model.CronJob, 0, len(jobs))
	for _, job := range jobs {
		zone := model.Timezone{Source: "controller-unknown"}
		if job.Spec.TimeZone != nil {
			zone.Name = job.Spec.TimeZone
			zone.Source = "spec"
		} else if opts.AssumeTimezone != "" {
			name := opts.AssumeTimezone
			zone.Name = &name
			zone.Source = "assumed"
		}
		var anchor *time.Time
		anchorSource := ""
		if job.Status.LastScheduleTime != nil {
			t := job.Status.LastScheduleTime.Time
			anchor = &t
			anchorSource = "last-schedule"
		} else if !job.CreationTimestamp.IsZero() {
			t := job.CreationTimestamp.Time
			anchor = &t
			anchorSource = "creation"
		}
		calculation, findings := schedule.Evaluate(schedule.Input{Expression: job.Spec.Schedule, Timezone: zone, Now: now, Anchor: anchor, AnchorSource: anchorSource, Count: opts.Count})
		suspended := job.Spec.Suspend != nil && *job.Spec.Suspend
		if suspended {
			calculation.Next = model.Timestamp{Reason: "suspended"}
			calculation.Occurrences = nil
		}
		if opts.NoFindings {
			findings = nil
		}
		if opts.Problems && len(findings) == 0 {
			continue
		}
		var lastSchedule, lastSuccess *time.Time
		if job.Status.LastScheduleTime != nil {
			t := job.Status.LastScheduleTime.Time
			lastSchedule = &t
		}
		if job.Status.LastSuccessfulTime != nil {
			t := job.Status.LastSuccessfulTime.Time
			lastSuccess = &t
		}
		concurrency := string(job.Spec.ConcurrencyPolicy)
		if concurrency == "" {
			concurrency = "Allow"
		}
		success, failed := int32(3), int32(1)
		if job.Spec.SuccessfulJobsHistoryLimit != nil {
			success = *job.Spec.SuccessfulJobsHistoryLimit
		}
		if job.Spec.FailedJobsHistoryLimit != nil {
			failed = *job.Spec.FailedJobsHistoryLimit
		}
		rows = append(rows, model.CronJob{Namespace: job.Namespace, Name: job.Name, Schedule: calculation, LastSchedule: model.Recorded(lastSchedule), LastSuccess: model.Recorded(lastSuccess), ActiveReferences: len(job.Status.Active), Suspended: suspended, Concurrency: concurrency, StartingDeadlineSeconds: job.Spec.StartingDeadlineSeconds, SuccessfulHistoryLimit: success, FailedHistoryLimit: failed, Findings: findings})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch opts.Sort {
		case "next":
			if c := compareTimes(a.Schedule.Next.Time, b.Schedule.Next.Time, false); c != 0 {
				return c < 0
			}
		case "last":
			if c := compareTimes(a.LastSchedule.Time, b.LastSchedule.Time, true); c != 0 {
				return c < 0
			}
		case "findings":
			if len(a.Findings) != len(b.Findings) {
				return len(a.Findings) > len(b.Findings)
			}
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name
	})
	summary := model.Summary{Total: len(rows)}
	for _, row := range rows {
		if row.Suspended {
			summary.Suspended++
		}
		if row.ActiveReferences > 0 {
			summary.WithActiveReferences++
		}
		if row.Schedule.Timezone.Source == "spec" {
			summary.TimezoneConfigured++
		} else {
			summary.TimezoneUnspecified++
		}
		if row.Schedule.Timezone.Source == "assumed" {
			summary.TimezoneAssumed++
		}
	}
	return model.Report{APIVersion: model.APIVersion, Kind: "CronJobReport", GeneratedAt: now.UTC(), Source: source, CronJobs: &rows, Summary: &summary}
}

func Expression(expression, zone string, explicit bool, now time.Time, count int, noFindings bool) model.Report {
	source := "offline-default"
	if explicit {
		source = "argument"
	}
	calculation, findings := schedule.Evaluate(schedule.Input{Expression: expression, Timezone: model.Timezone{Name: &zone, Source: source}, Now: now, Anchor: &now, AnchorSource: "evaluation", Count: count})
	if noFindings {
		findings = nil
	}
	return model.Report{APIVersion: model.APIVersion, Kind: "ScheduleExplanation", GeneratedAt: now.UTC(), Source: "expression", Explanation: &model.Explanation{Schedule: calculation, Findings: findings}}
}

func compareTimes(a, b *time.Time, descending bool) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return 1
	}
	if b == nil {
		return -1
	}
	if a.Equal(*b) {
		return 0
	}
	before := a.Before(*b)
	if descending {
		before = !before
	}
	if before {
		return -1
	}
	return 1
}
