// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package analyze

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/collect"
	"github.com/mertdotcc/kubectl-cron/internal/model"
	"github.com/mertdotcc/kubectl-cron/internal/schedule"
	batchv1 "k8s.io/api/batch/v1"
)

func Diagnose(e collect.Evidence, now time.Time, opts Options) model.Diagnostic {
	cj := Overview([]batchv1.CronJob{e.CronJob}, "cluster", now, opts).Rows()[0]
	d := model.Diagnostic{Namespace: cj.Namespace, Name: cj.Name, Findings: cj.Findings, Evidence: e.Availability}
	if opts.NoFindings {
		return d
	}
	add := func(rule, message string) {
		d.Findings = append(d.Findings, model.Finding{Rule: rule, Message: message})
	}
	if cj.FailedHistoryLimit == 0 {
		add("failed-history-disabled", "The failed Job history limit is zero; this setting requests no retained terminally failed Jobs.")
	}
	if cj.SuccessfulHistoryLimit == 0 {
		add("successful-history-disabled", "The successful Job history limit is zero; this setting requests no retained successful Jobs.")
	}
	runs := Runs(e.Jobs)
	failed, active, manual := []model.Run{}, []model.Run{}, map[string]bool{}
	for _, run := range runs {
		if run.Origin == "manual" {
			manual[run.Name] = true
		}
		if run.State == "failed" {
			failed = append(failed, run)
		}
		if run.ActivePods > 0 && run.State != "complete" && run.State != "failed" {
			active = append(active, run)
		}
		if run.State == "external-controller" {
			add("external-job-controller", fmt.Sprintf("Retained Job %s is managed by %s; built-in Job-controller status interpretation is unavailable.", run.Name, run.ManagedBy))
		}
	}
	if len(failed) > 0 {
		latest := failed[0]
		at := "not recorded"
		if latest.Finished.Time != nil {
			at = latest.Finished.Time.Format(time.RFC3339)
		}
		add("retained-job-failures", fmt.Sprintf("%d retained owned Jobs have terminal Failed=True. Newest by creation: %s (origin %s, terminal observation %s, reason %s). This retained sample does not establish a failure streak.", len(failed), latest.Name, latest.Origin, at, latest.Reason))
	}
	if len(active) > 1 {
		add("concurrent-active-jobs", fmt.Sprintf("%d retained nonterminal owned Jobs report active Pods; current concurrency policy is %s. Owned manual Jobs are included.", len(active), cj.Concurrency))
	}
	// Compare an observed duration to one explicitly identified current-spec gap,
	// rather than invent a fixed interval for uneven or DST-sensitive schedules.
	s, _ := schedule.Evaluate(schedule.Input{Expression: cj.Schedule.Expression, Timezone: cj.Schedule.Timezone, Now: now, Count: 2, Anchor: cj.Schedule.Anchor, AnchorSource: cj.Schedule.AnchorSource})
	if len(s.Occurrences) == 2 {
		gap := s.Occurrences[1].Sub(s.Occurrences[0]).Seconds()
		for _, run := range runs {
			if run.Duration.Seconds != nil && *run.Duration.Seconds >= gap {
				add("duration-exceeds-current-gap", fmt.Sprintf("Retained Job %s has observed duration %.1fs, at least the %.1fs gap between current-spec occurrences %s and %s. Historical schedule settings and skipped slots remain unknown.", run.Name, *run.Duration.Seconds, gap, s.Occurrences[0].Format(time.RFC3339), s.Occurrences[1].Format(time.RFC3339)))
				break
			}
		}
	}
	// Inspection's Event conversion gives one consistent last-observation basis.
	for _, event := range Events(e.Events) {
		rule := ""
		switch event.Reason {
		case "JobAlreadyActive":
			rule = "job-already-active-event"
		case "MissSchedule":
			rule = "missed-schedule-event"
		case "TooManyMissedTimes":
			rule = "too-many-missed-times-event"
		default:
			if event.Type == "Warning" {
				rule = "controller-warning-event"
			}
		}
		if rule == "" {
			continue
		}
		at := "not recorded"
		if event.ObservedAt.Time != nil {
			at = event.ObservedAt.Time.Format(time.RFC3339)
		}
		// The controller records UnexpectedJob for an owned Job missing from its
		// active list, such as one from kubectl create job --from, before adopting it.
		note := ""
		if i := strings.LastIndex(event.Message, ": "); event.Reason == "UnexpectedJob" && i >= 0 && manual[event.Message[i+2:]] {
			note = " The named Job is a retained owned manual Run."
		}
		add(rule, fmt.Sprintf("Retained CronJob Event %s: %s (last observed %s; observations %d).%s Event observations do not identify individual skipped slots or subsequent controller behavior.", event.Reason, event.Message, at, event.Observations, note))
	}
	return d
}

func Diagnostics(evidence []collect.Evidence, source string, now time.Time, opts Options) model.Report {
	rows := make([]model.Diagnostic, 0, len(evidence))
	for _, e := range evidence {
		rows = append(rows, Diagnose(e, now, opts))
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Namespace != rows[j].Namespace {
			return rows[i].Namespace < rows[j].Namespace
		}
		return rows[i].Name < rows[j].Name
	})
	return model.Report{APIVersion: model.APIVersion, Kind: "CronJobDiagnostics", GeneratedAt: now.UTC(), Source: source, Diagnostics: &rows}
}
