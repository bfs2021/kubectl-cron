// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mertdotcc/kubectl-cron/internal/model"
)

func calendar(out, notes io.Writer, c model.Calendar, opts Options) error {
	if _, err := fmt.Fprintf(out, "Nominal occurrences after %s through %s\n", c.WindowStart.Format(time.RFC3339), c.WindowEnd.Format(time.RFC3339)); err != nil {
		return err
	}
	t := table{}
	t.add(plain("OCCURRENCE (UTC)"), plain("COUNT"), plain("CRONJOBS"))
	for _, slot := range c.Slots {
		names := []string{}
		for _, member := range slot.CronJobs {
			name := member.Name
			if member.Namespace != "" {
				name = member.Namespace + "/" + name
			}
			if opts.Wide {
				name += " (" + member.Expression + "; " + timezoneLabel(member.Timezone) + "; " + member.LocalTime.Format(time.RFC3339) + ")"
			}
			names = append(names, safe(name))
		}
		t.add(plain(slot.At.Format("2006-01-02 15:04:05Z")), tint(strconv.Itoa(slot.Count), opts.Color && slot.Count > 1, color.FgYellow), plain(strings.Join(names, ", ")))
	}
	if err := t.write(out); err != nil {
		return err
	}
	if len(c.Slots) == 0 {
		if _, err := fmt.Fprintln(out, "No nominal occurrences computed in this window."); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(notes, "Counts describe nominal scheduling intent among selected CronJobs, not observed simultaneous executions."); err != nil {
		return err
	}
	if c.Truncated {
		if _, err := fmt.Fprintln(notes, "Calendar is truncated to the earliest displayed times; displayed groups include all selected CronJobs at those times."); err != nil {
			return err
		}
	}
	for _, excluded := range c.Excluded {
		if _, err := fmt.Fprintf(notes, "%s: excluded from calculation (%s).\n", safe(identity(model.CronJob{Namespace: excluded.Namespace, Name: excluded.Name})), safe(excluded.Reason)); err != nil {
			return err
		}
	}
	if !opts.NoFindings {
		if len(c.Findings) > 0 {
			if err := findings(out, "", c.Findings, opts); err != nil {
				return err
			}
		}
		for _, cj := range c.CronJobs {
			if len(cj.Findings) > 0 {
				if err := findings(out, identity(cj), cj.Findings, opts); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
