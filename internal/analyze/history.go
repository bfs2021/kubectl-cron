// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package analyze

import (
	"sort"
	"strings"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/collect"
	"github.com/mertdotcc/kubectl-cron/internal/model"
	"github.com/mertdotcc/kubectl-cron/internal/schedule"
	batchv1 "k8s.io/api/batch/v1"
)

func History(e collect.Evidence, now time.Time, opts Options, since time.Duration, limit int) model.Report {
	cj := Overview([]batchv1.CronJob{e.CronJob}, "cluster", now, opts).Rows()[0]
	start := now.Add(-since)
	h := model.History{CronJob: cj, WindowStart: start.UTC(), WindowEnd: now.UTC(), ProjectionBasis: "current-spec-only", Entries: []model.HistoryEntry{}, Evidence: e.Availability}
	matched := map[int64]bool{}
	total := 0
	for _, run := range Runs(e.Jobs) {
		at, basis := run.Slot, "recorded-slot"
		if run.Origin == "manual" || at.Time == nil {
			at, basis = run.Created, "job-creation"
		}
		if at.Time != nil && (at.Time.Before(start) || at.Time.After(now)) {
			continue
		}
		total++
		h.Entries = append(h.Entries, model.HistoryEntry{At: at, Basis: basis, State: run.State, Run: &run})
		if run.Origin != "manual" && run.Slot.Time != nil {
			matched[run.Slot.Time.UnixNano()] = true
		}
	}
	for _, source := range e.Availability {
		if source.Resource == "jobs" && source.State == "available" {
			h.RetainedRuns = &total
		}
	}
	projectionStart := start
	if !e.CronJob.CreationTimestamp.IsZero() && e.CronJob.CreationTimestamp.After(start) {
		projectionStart = e.CronJob.CreationTimestamp.Time
	}
	if strings.HasPrefix(strings.TrimSpace(cj.Schedule.Expression), "@every") {
		h.ProjectionUnavailable = "relative-past-phase-unknown"
	} else {
		var dates []time.Time
		dates, h.ProjectionUnavailable, h.ProjectionTruncated = schedule.Window(schedule.Input{Expression: cj.Schedule.Expression, Timezone: cj.Schedule.Timezone}, projectionStart, now, 10000, true)
		for _, at := range dates {
			if !matched[at.UnixNano()] {
				h.Entries = append(h.Entries, model.HistoryEntry{At: model.Timestamp{Time: &at}, Basis: "current-spec-projection", State: "unknown"})
			}
		}
	}
	sort.SliceStable(h.Entries, func(i, j int) bool {
		if c := compareTimes(h.Entries[i].At.Time, h.Entries[j].At.Time, true); c != 0 {
			return c < 0
		}
		name := func(v model.HistoryEntry) string {
			if v.Run != nil {
				return v.Run.Name
			}
			return ""
		}
		return name(h.Entries[i]) < name(h.Entries[j])
	})
	if len(h.Entries) > limit {
		h.Entries = h.Entries[:limit]
		h.EntriesTruncated = true
	}
	return model.Report{APIVersion: model.APIVersion, Kind: "CronJobRunHistory", GeneratedAt: now.UTC(), Source: "cluster", History: &h}
}
