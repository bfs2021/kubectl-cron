// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"

	"github.com/mertdotcc/kubectl-cron/internal/analyze"
	"github.com/mertdotcc/kubectl-cron/internal/collect"
	"github.com/mertdotcc/kubectl-cron/internal/model"
	"github.com/mertdotcc/kubectl-cron/internal/render"
	"github.com/spf13/cobra"
	typedbatch "k8s.io/client-go/kubernetes/typed/batch/v1"
	typedcore "k8s.io/client-go/kubernetes/typed/core/v1"
)

func commandOptions(parent *options) *options {
	return &options{config: parent.config, stdin: parent.stdin, now: parent.now, sort: "name", summary: "hide"}
}

func addInspection(root *cobra.Command, parent *options) {
	o := commandOptions(parent)
	limit := 20
	cmd := &cobra.Command{Use: "inspect NAME", Short: "Inspect retained owned Jobs and CronJob Events", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := o.validate(cmd, args, false); err != nil {
			return err
		}
		if len(o.filenames) > 0 {
			return fmt.Errorf("inspect uses live related resources and cannot be combined with offline --filename")
		}
		if limit < 1 || limit > 1000 {
			return fmt.Errorf("--limit must be between 1 and 1000")
		}
		e, err := o.evidence(cmd.Context(), args[0], true)
		if err != nil {
			return err
		}
		r := analyze.Inspection(e, o.now(), analyze.Options{AssumeTimezone: o.assumeTimezone, NoFindings: o.noFindings, Count: 1}, limit)
		return o.writeReport(cmd, r)
	}}
	addCommon(cmd, o)
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum retained Jobs to display, 1-1000")
	root.AddCommand(cmd)
}

func (o *options) clients() (typedbatch.BatchV1Interface, typedcore.CoreV1Interface, string, error) {
	ns, _, err := o.config.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return nil, nil, "", err
	}
	config, err := o.config.ToRESTConfig()
	if err != nil {
		return nil, nil, "", err
	}
	config.UserAgent = "kubectl-cron"
	batch, err := typedbatch.NewForConfig(config)
	if err != nil {
		return nil, nil, "", err
	}
	core, err := typedcore.NewForConfig(config)
	if err != nil {
		return nil, nil, "", err
	}
	return batch, core, ns, nil
}

func (o *options) evidence(ctx context.Context, arg string, events bool) (collect.Evidence, error) {
	name, err := objectName(arg)
	if err != nil {
		return collect.Evidence{}, err
	}
	batch, core, ns, err := o.clients()
	if err != nil {
		return collect.Evidence{}, err
	}
	items, err := collect.Live(ctx, batch, collect.Options{Namespace: ns, Name: name, Selector: o.selector})
	if err != nil {
		return collect.Evidence{}, err
	}
	if len(items) != 1 {
		return collect.Evidence{}, fmt.Errorf("no CronJob matches the requested name and selector")
	}
	return collect.Related(ctx, batch, core, items[0], events)
}

func (o *options) writeReport(cmd *cobra.Command, r model.Report) error {
	var err error
	if o.output == "json" || o.output == "yaml" {
		err = r.Encode(cmd.OutOrStdout(), o.output)
	} else {
		err = render.Human(cmd.OutOrStdout(), cmd.ErrOrStderr(), r, render.Options{Wide: o.output == "wide", Namespaces: o.allNamespaces, NoFindings: o.noFindings, Color: colorEnabled(o.color, cmd.OutOrStdout())})
	}
	if err != nil {
		return err
	}
	if o.exitOnFindings && r.HasFindings() {
		return ErrFindings
	}
	return nil
}
