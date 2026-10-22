// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/analyze"
	"github.com/spf13/cobra"
)

func addCalendar(root *cobra.Command, parent *options) {
	o := commandOptions(parent)
	next, limit := 2*time.Hour, 200
	cmd := &cobra.Command{Use: "calendar [NAME]", Short: "Group upcoming nominal occurrences across selected CronJobs", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := o.validate(cmd, args, false); err != nil {
			return err
		}
		if next <= 0 || next > 365*24*time.Hour {
			return fmt.Errorf("--next must be greater than zero and at most 8760h")
		}
		if limit < 1 || limit > 1000 {
			return fmt.Errorf("--limit must be between 1 and 1000")
		}
		cjs, source, err := o.objects(cmd, args)
		if err != nil {
			return err
		}
		return o.writeReport(cmd, analyze.Calendar(cjs, source, o.now(), next, limit, analyze.Options{AssumeTimezone: o.assumeTimezone, NoFindings: o.noFindings, Count: 1, Sort: "name"}))
	}}
	addCommon(cmd, o)
	cmd.Flags().DurationVar(&next, "next", 2*time.Hour, "Upcoming window, greater than zero and at most 8760h")
	cmd.Flags().IntVar(&limit, "limit", 200, "Maximum distinct nominal times to display, 1-1000")
	root.AddCommand(cmd)
}
