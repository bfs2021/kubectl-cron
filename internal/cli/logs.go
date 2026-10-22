// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"math"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/logs"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func addLogs(root *cobra.Command, parent *options) {
	o := commandOptions(parent)
	var selection logs.Selection
	var container string
	var allContainers, previous, timestamps, prefix bool
	var since time.Duration
	tail, maxRequests := int64(100), 5
	cmd := &cobra.Command{Use: "logs NAME", Short: "Read or follow logs from retained owned executions", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := labels.Parse(o.selector); err != nil {
			return fmt.Errorf("invalid CronJob selector: %w", err)
		}
		if selection.Offset < 0 {
			return fmt.Errorf("--run-offset must be nonnegative")
		}
		if selection.All && cmd.Flags().Changed("run-offset") {
			return fmt.Errorf("--all and --run-offset are mutually exclusive")
		}
		if selection.Run != "" && (selection.All || selection.Failed || cmd.Flags().Changed("run-offset")) {
			return fmt.Errorf("--run cannot be combined with --all, --failed, or --run-offset")
		}
		if container != "" && allContainers {
			return fmt.Errorf("--container and --all-containers are mutually exclusive")
		}
		if tail < -1 || since < 0 || maxRequests < 1 || maxRequests > 20 {
			return fmt.Errorf("--tail must be -1 or nonnegative, --since nonnegative, and --max-log-requests between 1 and 20")
		}
		e, err := o.evidence(cmd.Context(), args[0], false)
		if err != nil {
			return err
		}
		for _, source := range e.Availability {
			if source.Resource == "jobs" && source.State != "available" {
				return fmt.Errorf("read Jobs for logs: %s (%s): %s", source.State, source.Reason, source.Message)
			}
		}
		jobs, err := logs.Select(e.Jobs, selection)
		if err != nil {
			return err
		}
		_, core, _, err := o.clients()
		if err != nil {
			return err
		}
		targets, notes, err := logs.Resolve(cmd.Context(), core, jobs, container, allContainers)
		for _, note := range notes {
			if _, writeErr := fmt.Fprintln(cmd.ErrOrStderr(), note); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			return err
		}
		pod := corev1.PodLogOptions{Follow: selection.Follow, Previous: previous, Timestamps: timestamps, TailLines: &tail}
		if since > 0 {
			seconds := int64(math.Ceil(since.Seconds()))
			pod.SinceSeconds = &seconds
		}
		return logs.Stream(cmd.Context(), core, targets, logs.StreamOptions{Pod: pod, MaxRequests: maxRequests, Prefix: prefix}, cmd.OutOrStdout())
	}}
	f := cmd.Flags()
	f.StringVarP(&o.selector, "selector", "l", "", "Label selector matched against CronJob labels")
	f.StringVar(&selection.Run, "run", "", "Select a retained owned Job by exact name")
	f.IntVar(&selection.Offset, "run-offset", 0, "Select an older retained Job: 0 is newest, 1 is previous")
	f.BoolVar(&selection.Failed, "failed", false, "Select the newest terminally failed Job (or filter --all)")
	f.BoolVar(&selection.All, "all", false, "Read every retained owned Job")
	f.BoolVarP(&selection.Follow, "follow", "f", false, "Follow selected streams; default to the latest active Job")
	f.StringVarP(&container, "container", "c", "", "Container name, including init or ephemeral containers")
	f.BoolVar(&allContainers, "all-containers", false, "Read regular, init, and ephemeral containers")
	f.BoolVar(&previous, "previous", false, "Read the previous container instance in each selected Pod")
	f.Int64Var(&tail, "tail", 100, "Lines per container; -1 reads all available lines")
	f.DurationVar(&since, "since", 0, "Read logs newer than this duration")
	f.BoolVar(&timestamps, "timestamps", false, "Include timestamps supplied by the log API")
	f.BoolVar(&prefix, "prefix", false, "Always prefix lines with namespace/Job/Pod/container (automatic for multiple streams)")
	f.IntVar(&maxRequests, "max-log-requests", 5, "Maximum concurrent streams, 1-20; follow requires room for every stream")
	root.AddCommand(cmd)
}
