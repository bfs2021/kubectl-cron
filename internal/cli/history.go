// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/analyze"
	"github.com/spf13/cobra"
)

func addHistory(root *cobra.Command, parent *options) {
	o := commandOptions(parent)
	since, limit := 24*time.Hour, 20
	cmd := &cobra.Command{Use: "runs NAME", Short: "Show retained executions and unknown projected slots", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := o.validate(cmd, args, false); err != nil {
			return err
		}
		if len(o.filenames) > 0 {
			return fmt.Errorf("runs requires live related resources; --filename is offline")
		}
		if err := validateHistory(since, limit); err != nil {
			return err
		}
		e, err := o.evidence(cmd.Context(), args[0], false)
		if err != nil {
			return err
		}
		return o.writeReport(cmd, analyze.History(e, o.now(), analyze.Options{AssumeTimezone: o.assumeTimezone, NoFindings: o.noFindings, Count: 1}, since, limit))
	}}
	addCommon(cmd, o)
	historyFlags(cmd, &since, &limit)
	root.AddCommand(cmd)
}

func historyFlags(cmd *cobra.Command, since *time.Duration, limit *int) {
	cmd.Flags().DurationVar(since, "since", *since, "History window, greater than zero and at most 8760h")
	cmd.Flags().IntVar(limit, "limit", *limit, "Maximum history entries to display, 1-1000")
}
func validateHistory(since time.Duration, limit int) error {
	if since <= 0 || since > 365*24*time.Hour {
		return fmt.Errorf("--since must be greater than zero and at most 8760h")
	}
	if limit < 1 || limit > 1000 {
		return fmt.Errorf("--limit must be between 1 and 1000")
	}
	return nil
}
