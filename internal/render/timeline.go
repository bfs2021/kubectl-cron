// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/model"
)

func timeline(out, notes io.Writer, h model.History, opts Options) error {
	if _, err := fmt.Fprintf(out, "CronJob %s\nHistory %s to %s\nOldest to newest; one symbol per entry, not proportional elapsed time.\n\n", safe(identity(h.CronJob)), h.WindowStart.Format(time.RFC3339), h.WindowEnd.Format(time.RFC3339)); err != nil {
		return err
	}
	counts := map[string]int{}
	for _, entry := range h.Entries {
		counts[entry.State]++
	}
	for from := len(h.Entries) - 1; from >= 0; {
		to := from - 79
		if to < 0 {
			to = 0
		}
		var line strings.Builder
		for i := from; i >= to; i-- {
			line.WriteByte(stateSymbol(h.Entries[i].State))
		}
		if _, err := fmt.Fprintf(out, "%s |%s| %s\n", timestamp(h.Entries[from].At), line.String(), timestamp(h.Entries[to].At)); err != nil {
			return err
		}
		from = to - 1
	}
	if len(h.Entries) == 0 {
		if _, err := fmt.Fprintln(out, "No entries in this window."); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(out, "\n+ complete   x failed   > running   ~ terminating   . pending   | suspended   E external   ? unknown"); err != nil {
		return err
	}
	if _, err := fmt.Fprint(out, "Shown entries:"); err != nil {
		return err
	}
	for _, state := range []string{"complete", "failed", "running", "terminating", "pending", "suspended", "external-controller", "unknown"} {
		if counts[state] > 0 {
			if _, err := fmt.Fprintf(out, " %s=%d", state, counts[state]); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(out); err != nil {
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

func stateSymbol(state string) byte {
	switch state {
	case "complete":
		return '+'
	case "failed":
		return 'x'
	case "running":
		return '>'
	case "terminating":
		return '~'
	case "pending":
		return '.'
	case "suspended":
		return '|'
	case "external-controller":
		return 'E'
	default:
		return '?'
	}
}
