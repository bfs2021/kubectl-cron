// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"io"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/model"
)

func inspection(out, notes io.Writer, v model.Inspection, now time.Time, opts Options) error {
	cj := v.CronJob
	if _, err := fmt.Fprintf(out, "CronJob %s\n  Schedule: %s (%s)\n  Timezone: %s\n  Next: %s\n  Concurrency: %s; suspended: %t\n  History limits (successful/failed): %d/%d\n", safe(identity(cj)), safe(cj.Schedule.Expression), safe(cj.Schedule.Description), timezoneLabel(cj.Schedule.Timezone), nextCell(cj.Schedule, now, opts).print(), safe(cj.Concurrency), cj.Suspended, cj.SuccessfulHistoryLimit, cj.FailedHistoryLimit); err != nil {
		return err
	}
	deadline := "unset"
	if cj.StartingDeadlineSeconds != nil {
		deadline = fmt.Sprintf("%ds", *cj.StartingDeadlineSeconds)
	}
	if _, err := fmt.Fprintf(out, "  Starting deadline: %s\n  Last scheduled: %s; last success: %s\n  Active Job references: %d\n", deadline, timestamp(cj.LastSchedule), timestamp(cj.LastSuccess), cj.ActiveReferences); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "\nRetained Jobs"); err != nil {
		return err
	}
	if v.RetainedRuns == nil {
		if _, err := fmt.Fprintln(out, "  Unavailable; see evidence reads below."); err != nil {
			return err
		}
	} else if len(v.Runs) == 0 {
		if _, err := fmt.Fprintln(out, "  None returned. This does not establish a complete execution history."); err != nil {
			return err
		}
	} else {
		if err := runTable(out, v.Runs); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(notes, "Delay is Job creation minus recorded slot; duration is Job-controller time, not container runtime."); err != nil {
			return err
		}
		if v.RunsTruncated {
			if _, err := fmt.Fprintf(notes, "Showing %d of %d retained Jobs.\n", len(v.Runs), *v.RetainedRuns); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(out, "\nRetained CronJob Events (best-effort evidence)"); err != nil {
		return err
	}
	if !sourceAvailable(v.Evidence, "events") {
		if _, err := fmt.Fprintln(out, "  Unavailable; see evidence reads below."); err != nil {
			return err
		}
	} else if len(v.Events) == 0 {
		if _, err := fmt.Fprintln(out, "  No retained Events returned; absence is not evidence of no problems."); err != nil {
			return err
		}
	}
	for _, event := range v.Events {
		if _, err := fmt.Fprintf(out, "  %s  %s  %s  %s (observations: %d)\n", age(event.ObservedAt.Time, now), safe(event.Type), safe(event.Reason), safe(event.Message), event.Observations); err != nil {
			return err
		}
	}
	if err := availability(out, v.Evidence); err != nil {
		return err
	}
	if !opts.NoFindings && len(cj.Findings) > 0 {
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		return findings(out, "", cj.Findings, opts)
	}
	return nil
}

func runTable(out io.Writer, runs []model.Run) error {
	t := table{}
	t.add(plain("JOB"), plain("ORIGIN"), plain("SLOT (UTC)"), plain("CREATED (UTC)"), plain("RESULT"), plain("DELAY"), plain("DURATION"))
	for _, r := range runs {
		state := r.State
		if r.Reason != "" {
			state += " (" + r.Reason + ")"
		}
		if r.State == "external-controller" {
			state += " (" + r.ManagedBy + ")"
		}
		t.add(plain(safe(r.Name)), plain(r.Origin), plain(timestamp(r.Slot)), plain(timestamp(r.Created)), plain(safe(state)), plain(measureLabel(r.SchedulingDelay)), plain(measureLabel(r.Duration)))
	}
	return t.write(out)
}

func sourceAvailable(sources []model.Availability, resource string) bool {
	for _, source := range sources {
		if source.Resource == resource {
			return source.State == "available"
		}
	}
	return false
}

func timestamp(t model.Timestamp) string {
	if t.Time == nil {
		return "?"
	}
	return t.Time.UTC().Format("2006-01-02 15:04:05Z")
}
func measureLabel(m model.Measure) string {
	if m.Seconds == nil {
		return "?"
	}
	return fmt.Sprintf("%.1fs", *m.Seconds)
}
func availability(out io.Writer, sources []model.Availability) error {
	if _, err := fmt.Fprintln(out, "\nEvidence reads"); err != nil {
		return err
	}
	for _, source := range sources {
		if _, err := fmt.Fprintf(out, "  %s: %s", safe(source.Resource), safe(source.State)); err != nil {
			return err
		}
		if source.Reason != "" {
			if _, err := fmt.Fprintf(out, " (%s)", safe(source.Reason)); err != nil {
				return err
			}
		}
		if source.Message != "" {
			if _, err := fmt.Fprintf(out, " — %s", safe(source.Message)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	return nil
}
