// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"github.com/mertdotcc/kubectl-cron/internal/collect"
	"github.com/spf13/cobra"
	batchv1 "k8s.io/api/batch/v1"
)

func (o *options) objects(cmd *cobra.Command, args []string) ([]batchv1.CronJob, string, error) {
	query := collect.Options{AllNamespaces: o.allNamespaces, Selector: o.selector, Filenames: o.filenames, Stdin: o.stdin, NamespaceExplicit: cmd.Flags().Changed("namespace")}
	if o.config.Namespace != nil {
		query.Namespace = *o.config.Namespace
	}
	if len(args) > 0 {
		name, err := objectName(args[0])
		if err != nil {
			return nil, "", err
		}
		query.Name = name
	}
	if len(o.filenames) > 0 {
		cjs, err := collect.Files(query)
		return cjs, "file", err
	}
	batch, _, ns, err := o.clients()
	if err != nil {
		return nil, "", err
	}
	query.Namespace = ns
	cjs, err := collect.Live(cmd.Context(), batch, query)
	return cjs, "cluster", err
}
