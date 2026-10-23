// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package analyze

import (
	"fmt"
	"sort"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/model"
	"github.com/mertdotcc/kubectl-cron/internal/schedule"
	batchv1 "k8s.io/api/batch/v1"
)

func Calendar(cjs []batchv1.CronJob, source string, now time.Time, next time.Duration, limit int, opts Options) model.Report {
	rows := Overview(cjs, source, now, opts).Rows()
	c := model.Calendar{WindowStart: now.UTC(), WindowEnd: now.Add(next).UTC(), CronJobs: rows, Slots: []model.CalendarSlot{}, Excluded: []model.CalendarExclusion{}}
	groups := map[string]*model.CalendarSlot{}
	for _, cj := range rows {
		if cj.Schedule.Next.Time == nil {
			c.Excluded = append(c.Excluded, model.CalendarExclusion{Namespace: cj.Namespace, Name: cj.Name, Reason: cj.Schedule.Next.Reason})
			continue
		}
		in := schedule.Input{Expression: cj.Schedule.Expression, Timezone: cj.Schedule.Timezone, Anchor: cj.Schedule.Anchor, AnchorSource: cj.Schedule.AnchorSource}
		// Each CronJob can contribute at most one occurrence to a group. Its
		// first limit+1 occurrences therefore cover every displayed group fully.
		dates, reason, truncated := schedule.Window(in, now.Add(time.Nanosecond), c.WindowEnd, limit+1, false)
		c.Truncated = c.Truncated || truncated
		if reason != "" {
			c.Excluded = append(c.Excluded, model.CalendarExclusion{Namespace: cj.Namespace, Name: cj.Name, Reason: reason})
		}
		for _, at := range dates {
			key := at.UTC().Format(time.RFC3339Nano)
			group := groups[key]
			if group == nil {
				group = &model.CalendarSlot{At: at.UTC(), CronJobs: []model.CalendarMember{}}
				groups[key] = group
			}
			group.CronJobs = append(group.CronJobs, model.CalendarMember{Namespace: cj.Namespace, Name: cj.Name, Expression: cj.Schedule.Expression, Timezone: cj.Schedule.Timezone, LocalTime: at})
			group.Count++
		}
	}
	for _, group := range groups {
		c.Slots = append(c.Slots, *group)
	}
	sort.Slice(c.Slots, func(i, j int) bool { return c.Slots[i].At.Before(c.Slots[j].At) })
	if len(c.Slots) > limit {
		c.Slots = c.Slots[:limit]
		c.Truncated = true
	}
	shared, peak, peakAt := 0, 0, time.Time{}
	for _, slot := range c.Slots {
		if slot.Count > 1 {
			shared++
			if slot.Count > peak {
				peak, peakAt = slot.Count, slot.At
			}
		}
	}
	if shared > 0 && !opts.NoFindings {
		sharedWord := "times are"
		if shared == 1 {
			sharedWord = "time is"
		}
		c.Findings = []model.Finding{{Rule: "coincident-schedules", Message: fmt.Sprintf("%d displayed nominal %s shared by selected CronJobs; largest group is %d at %s. This describes scheduling intent; actual simultaneous execution is unverified.", shared, sharedWord, peak, peakAt.Format(time.RFC3339))}}
	}
	return model.Report{APIVersion: model.APIVersion, Kind: "CronJobCalendar", GeneratedAt: now.UTC(), Source: source, Calendar: &c}
}
