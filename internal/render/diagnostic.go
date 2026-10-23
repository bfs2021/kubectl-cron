// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"io"

	"github.com/mertdotcc/kubectl-cron/internal/model"
)

func diagnostics(out, notes io.Writer, rows []model.Diagnostic, opts Options) error {
	found := false
	for _, row := range rows {
		name := row.Name
		if row.Namespace != "" {
			name = row.Namespace + "/" + name
		}
		if !opts.NoFindings && len(row.Findings) > 0 {
			found = true
			if err := findings(out, name, row.Findings, opts); err != nil {
				return err
			}
		}
		for _, source := range row.Evidence {
			if source.State != "available" {
				line := fmt.Sprintf("%s: %s evidence %s (%s).", safe(name), safe(source.Resource), safe(source.State), safe(source.Reason))
				if source.Message != "" {
					line += " " + safe(source.Message)
				}
				if _, err := fmt.Fprintln(notes, line); err != nil {
					return err
				}
			}
		}
	}
	if !found {
		if _, err := fmt.Fprintln(notes, "No Findings in the inspected evidence; missing or expired evidence remains unknown."); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(notes, "Jobs are a retained sample; CronJob Events are temporary, best-effort observations.")
	return err
}
