// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"

	"github.com/mertdotcc/kubectl-cron/internal/analyze"
	"github.com/mertdotcc/kubectl-cron/internal/collect"
	"github.com/mertdotcc/kubectl-cron/internal/model"
	"github.com/spf13/cobra"
)

func addDiagnostics(root *cobra.Command, parent *options) {
	o := commandOptions(parent)
	cmd := &cobra.Command{Use: "diagnose [NAME]", Short: "Show Findings from configuration and retained runtime evidence", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := o.validate(cmd, args, false); err != nil {
			return err
		}
		if o.requireEvidence && len(o.filenames) > 0 {
			return fmt.Errorf("--require-evidence uses live related resources and cannot be combined with offline --filename")
		}
		cjs, source, err := o.objects(cmd, args)
		if err != nil {
			return err
		}
		evidence := []collect.Evidence{}
		if source == "file" {
			for _, cj := range cjs {
				evidence = append(evidence, collect.Evidence{CronJob: cj, Availability: []model.Availability{{Resource: "jobs", State: "not-requested", Reason: "offline-source"}, {Resource: "events", State: "not-requested", Reason: "offline-source"}}})
			}
		} else {
			batch, core, _, err := o.clients()
			if err != nil {
				return err
			}
			evidence, err = collect.RelatedMany(cmd.Context(), batch, core, cjs)
			if err != nil {
				return err
			}
		}
		return o.writeReport(cmd, analyze.Diagnostics(evidence, source, o.now(), analyze.Options{AssumeTimezone: o.assumeTimezone, NoFindings: o.noFindings, Count: 1}))
	}}
	addCommon(cmd, o)
	addRequireEvidence(cmd, o)
	root.AddCommand(cmd)
}
