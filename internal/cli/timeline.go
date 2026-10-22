// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/analyze"
	"github.com/spf13/cobra"
)

func addTimeline(root *cobra.Command, parent *options) {
	o := commandOptions(parent)
	since, limit := 24*time.Hour, 200
	cmd := &cobra.Command{Use: "timeline NAME", Short: "Draw observed execution states and unknown projected slots", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := o.validate(cmd, args, false); err != nil {
			return err
		}
		if len(o.filenames) > 0 {
			return fmt.Errorf("timeline requires live related resources; --filename is offline")
		}
		if err := validateHistory(since, limit); err != nil {
			return err
		}
		e, err := o.evidence(cmd.Context(), args[0], false)
		if err != nil {
			return err
		}
		r := analyze.History(e, o.now(), analyze.Options{AssumeTimezone: o.assumeTimezone, NoFindings: o.noFindings, Count: 1}, since, limit)
		r.Kind = "CronJobTimeline"
		return o.writeReport(cmd, r)
	}}
	addCommon(cmd, o)
	historyFlags(cmd, &since, &limit)
	root.AddCommand(cmd)
}
