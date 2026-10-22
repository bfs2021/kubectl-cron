// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"io"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/model"
)

func history(out, notes io.Writer, h model.History, opts Options) error {
	if _, err := fmt.Fprintf(out, "CronJob %s\nHistory %s to %s\n", safe(identity(h.CronJob)), h.WindowStart.Format(time.RFC3339), h.WindowEnd.Format(time.RFC3339)); err != nil {
		return err
	}
	t := table{}
	t.add(plain("SLOT / CREATED (UTC)"), plain("JOB"), plain("ORIGIN"), plain("RESULT"), plain("DELAY"), plain("DURATION"))
	for _, entry := range h.Entries {
		if entry.Run == nil {
			t.add(plain(timestamp(entry.At)), plain("?"), plain("?"), plain("unknown"), plain("?"), plain("?"))
			continue
		}
		r := entry.Run
		state := r.State
		if r.Reason != "" {
			state += " (" + r.Reason + ")"
		}
		t.add(plain(timestamp(entry.At)), plain(safe(r.Name)), plain(r.Origin), plain(safe(state)), plain(measureLabel(r.SchedulingDelay)), plain(measureLabel(r.Duration)))
	}
	if err := t.write(out); err != nil {
		return err
	}
	if err := historyNotes(notes, h); err != nil {
		return err
	}
	if err := availability(out, h.Evidence); err != nil {
		return err
	}
	if !opts.NoFindings && len(h.CronJob.Findings) > 0 {
		return findings(out, identity(h.CronJob), h.CronJob.Findings, opts)
	}
	return nil
}

func historyNotes(out io.Writer, h model.History) error {
	if _, err := fmt.Fprintln(out, "Unknown slots project the current schedule only; past configuration, suspension, deletion, and skip causes are unproven.\nDelay is Job creation minus recorded slot; duration is Job-controller time. Rows without a recorded slot use creation time."); err != nil {
		return err
	}
	if h.EntriesTruncated || h.ProjectionTruncated {
		if _, err := fmt.Fprintln(out, "History display is truncated to the newest bounded entries; counts do not describe complete execution history."); err != nil {
			return err
		}
	}
	if h.ProjectionUnavailable != "" {
		if _, err := fmt.Fprintf(out, "Past slot projection unavailable: %s.\n", safe(h.ProjectionUnavailable)); err != nil {
			return err
		}
	}
	return nil
}
