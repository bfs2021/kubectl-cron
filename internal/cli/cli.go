// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

// Package cli composes input collection, analysis and rendering.
package cli

import (
	"errors"
	goflag "flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	typedbatch "k8s.io/client-go/kubernetes/typed/batch/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"

	"github.com/mertdotcc/kubectl-cron/internal/analyze"
	"github.com/mertdotcc/kubectl-cron/internal/collect"
	"github.com/mertdotcc/kubectl-cron/internal/model"
	"github.com/mertdotcc/kubectl-cron/internal/render"
)

var ErrFindings = errors.New("findings present")

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, ErrFindings) {
		return 2
	}
	return 1
}

type options struct {
	config                                                     *genericclioptions.ConfigFlags
	filenames                                                  []string
	allNamespaces                                              bool
	selector, output, color, sort, summary, assumeTimezone, tz string
	noFindings, problems, exitOnFindings                       bool
	count                                                      int
	stdin                                                      io.Reader
	now                                                        func() time.Time
}

func New(in io.Reader, out, errOut io.Writer, now func() time.Time, version string) *cobra.Command {
	o := &options{config: genericclioptions.NewConfigFlags(true), stdin: in, now: now, sort: "name", summary: "show"}
	root := &cobra.Command{
		Use:     "cron [NAME|cronjob/NAME]",
		Short:   "Inspect CronJob schedules and recorded state (read-only)",
		Long:    "Show CronJob scheduling facts, upcoming nominal occurrences, and configuration findings.\nThe default Overview reads CronJobs only. Richer commands read related evidence.\nThe plugin never creates, changes, or runs cluster resources.",
		Example: "  kubectl cron\n  kubectl cron -A -o wide\n  kubectl cron cronjob/explain\n  kubectl cron -f schedules.yaml --assume-timezone=UTC\n  kubectl cron --summary=only",
		Args:    cobra.MaximumNArgs(1), SilenceUsage: true, SilenceErrors: true, Version: version,
		RunE: func(cmd *cobra.Command, args []string) error { return o.run(cmd, args, false) },
	}
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetVersionTemplate("{{.Version}}\n")
	o.config.AddFlags(root.PersistentFlags())
	addVerbosity(root.PersistentFlags())
	addCommon(root, o)
	root.Flags().StringVar(&o.sort, "sort", "name", "Order rows: name|next|last|findings")
	root.Flags().StringVar(&o.summary, "summary", "show", "Factual Summary: show|only|hide")
	root.Flags().BoolVar(&o.problems, "problems", false, "Show only CronJobs with Findings (not a health verdict)")
	explain := &cobra.Command{
		Use: "explain [EXPRESSION]", Short: "Explain a schedule or CronJob files without a cluster",
		Example: "  kubectl cron explain '15 6 * * 1-5' --tz Europe/Berlin\n  kubectl cron explain -f schedules.yaml --assume-timezone=UTC",
		Args:    cobra.MaximumNArgs(1), SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error { return o.run(cmd, args, true) },
	}
	addCommon(explain, o)
	explain.Flags().StringVar(&o.tz, "tz", "UTC", "Timezone for a bare expression (default UTC)")
	explain.Flags().IntVar(&o.count, "count", 5, "Number of upcoming occurrences, 1-1000")
	root.AddCommand(explain)
	addInspection(root, o)
	addHistory(root, o)
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetUsageTemplate(strings.NewReplacer("{{.UseLine}}", "kubectl {{.UseLine}}", "{{.CommandPath}}", "kubectl {{.CommandPath}}").Replace(root.UsageTemplate()))
	root.SetHelpTemplate(strings.ReplaceAll(root.HelpTemplate(), "{{.CommandPath}}", "kubectl {{.CommandPath}}"))
	return root
}

// Command-local shorthand leaves -c/-f available to future logs commands.
func addCommon(cmd *cobra.Command, o *options) {
	f := cmd.Flags()
	f.BoolVarP(&o.allNamespaces, "all-namespaces", "A", false, "Select CronJobs in all namespaces")
	f.StringVarP(&o.selector, "selector", "l", "", "Label selector matched against CronJob labels")
	f.StringSliceVarP(&o.filenames, "filename", "f", nil, "Read local CronJob files offline; repeatable, '-' for stdin")
	f.StringVarP(&o.output, "output", "o", "table", "Output: table|wide|json|yaml")
	f.StringVarP(&o.color, "color", "c", "auto", "Color: auto|always|never")
	f.StringVar(&o.assumeTimezone, "assume-timezone", "", "Explicit timezone assumption for objects with no timeZone")
	f.BoolVar(&o.noFindings, "no-findings", false, "Suppress Findings in every output format")
	f.BoolVar(&o.exitOnFindings, "exit-on-findings", false, "Exit 2 when Findings are present; operational errors remain 1")
}

func addVerbosity(flags *pflag.FlagSet) {
	goFlags := goflag.NewFlagSet("logging", goflag.ContinueOnError)
	klog.InitFlags(goFlags)
	flags.AddGoFlag(goFlags.Lookup("v"))
	flags.Lookup("v").Shorthand = "v"
}

func (o *options) validate(cmd *cobra.Command, args []string, explain bool) error {
	if !oneOf(o.output, "table", "wide", "json", "yaml") {
		return fmt.Errorf("--output must be table|wide|json|yaml")
	}
	if !oneOf(o.color, "auto", "always", "never") {
		return fmt.Errorf("--color must be auto|always|never")
	}
	if !oneOf(o.sort, "name", "next", "last", "findings") {
		return fmt.Errorf("--sort must be name|next|last|findings")
	}
	if !oneOf(o.summary, "show", "only", "hide") {
		return fmt.Errorf("--summary must be show|only|hide")
	}
	if o.noFindings && (o.problems || o.exitOnFindings) {
		return fmt.Errorf("--no-findings cannot be combined with --problems or --exit-on-findings")
	}
	if o.allNamespaces && cmd.Flags().Changed("namespace") {
		return fmt.Errorf("--all-namespaces and --namespace are mutually exclusive")
	}
	if _, err := labels.Parse(o.selector); err != nil {
		return fmt.Errorf("invalid CronJob selector: %w", err)
	}
	if o.assumeTimezone != "" {
		if strings.EqualFold(o.assumeTimezone, "Local") {
			return fmt.Errorf("--assume-timezone must name a portable timezone, not Local")
		}
		if _, err := time.LoadLocation(o.assumeTimezone); err != nil {
			return fmt.Errorf("--assume-timezone: %w", err)
		}
	}
	if cmd.Flags().Changed("assume-timezone") && o.assumeTimezone == "" {
		return fmt.Errorf("--assume-timezone requires a timezone name")
	}
	if explain {
		if o.count < 1 || o.count > 1000 {
			return fmt.Errorf("--count must be between 1 and 1000")
		}
		if len(o.filenames) == 0 && len(args) != 1 {
			return fmt.Errorf("provide an expression or --filename")
		}
		if len(o.filenames) > 0 && len(args) > 0 {
			return fmt.Errorf("an expression and --filename are mutually exclusive")
		}
		if len(o.filenames) > 0 && cmd.Flags().Changed("tz") {
			return fmt.Errorf("--tz applies only to bare expressions; use the manifest timezone or --assume-timezone")
		}
		if len(o.filenames) == 0 {
			if o.assumeTimezone != "" {
				return fmt.Errorf("--assume-timezone applies to objects; use --tz for an expression")
			}
			if o.allNamespaces || o.selector != "" || cmd.Flags().Changed("namespace") {
				return fmt.Errorf("object selection flags do not apply to a bare expression")
			}
			if o.tz == "" || strings.EqualFold(o.tz, "Local") {
				return fmt.Errorf("--tz requires a portable timezone name")
			}
			if _, err := time.LoadLocation(o.tz); err != nil {
				return fmt.Errorf("--tz: %w", err)
			}
		}
	} else if o.allNamespaces && len(args) > 0 {
		return fmt.Errorf("a named CronJob requires a namespace; omit --all-namespaces")
	}
	return nil
}

func (o *options) run(cmd *cobra.Command, args []string, explain bool) error {
	if err := o.validate(cmd, args, explain); err != nil {
		return err
	}
	var report model.Report
	if explain && len(o.filenames) == 0 {
		report = analyze.Expression(args[0], o.tz, cmd.Flags().Changed("tz"), o.now(), o.count, o.noFindings)
	} else {
		selection := collect.Options{AllNamespaces: o.allNamespaces, Selector: o.selector, Filenames: o.filenames, Stdin: o.stdin, NamespaceExplicit: cmd.Flags().Changed("namespace")}
		if o.config.Namespace != nil {
			selection.Namespace = *o.config.Namespace
		}
		if len(args) > 0 {
			name, err := objectName(args[0])
			if err != nil {
				return err
			}
			selection.Name = name
		}
		var err error
		source := "file"
		if len(o.filenames) > 0 {
			jobs, readErr := collect.Files(selection)
			if readErr != nil {
				return readErr
			}
			count := 1
			if explain {
				count = o.count
			}
			report = analyze.Overview(jobs, source, o.now(), analyze.Options{AssumeTimezone: o.assumeTimezone, NoFindings: o.noFindings, Problems: o.problems, Sort: o.sort, Count: count})
		} else {
			source = "cluster"
			selection.Namespace, _, err = o.config.ToRawKubeConfigLoader().Namespace()
			if err != nil {
				return err
			}
			config, err := o.config.ToRESTConfig()
			if err != nil {
				return err
			}
			config.UserAgent = "kubectl-cron"
			client, err := typedbatch.NewForConfig(config)
			if err != nil {
				return err
			}
			jobs, err := collect.Live(cmd.Context(), client, selection)
			if err != nil {
				return fmt.Errorf("read CronJobs: %w", err)
			}
			report = analyze.Overview(jobs, source, o.now(), analyze.Options{AssumeTimezone: o.assumeTimezone, NoFindings: o.noFindings, Problems: o.problems, Sort: o.sort, Count: 1})
		}
		if explain {
			report.Kind = "ScheduleExplanation"
			report.Summary = nil
		}
	}
	hasFindings := report.HasFindings()
	if !explain {
		switch o.summary {
		case "hide":
			report.Summary = nil
		case "only":
			report.CronJobs = nil
		}
	}
	var err error
	if o.output == "json" || o.output == "yaml" {
		err = report.Encode(cmd.OutOrStdout(), o.output)
	} else {
		err = render.Human(cmd.OutOrStdout(), cmd.ErrOrStderr(), report, render.Options{Wide: o.output == "wide", Namespaces: o.allNamespaces, NoFindings: o.noFindings, Explain: explain, Color: colorEnabled(o.color, cmd.OutOrStdout())})
	}
	if err != nil {
		return err
	}
	if o.exitOnFindings && hasFindings {
		return ErrFindings
	}
	return nil
}

func objectName(arg string) (string, error) {
	if !strings.Contains(arg, "/") {
		return arg, nil
	}
	parts := strings.Split(arg, "/")
	if len(parts) != 2 || parts[0] != "cronjob" || parts[1] == "" {
		return "", fmt.Errorf("expected NAME or cronjob/NAME, got %q", arg)
	}
	return parts[1], nil
}
func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}
func colorEnabled(mode string, w io.Writer) bool {
	if mode == "always" {
		return true
	}
	if mode == "never" {
		return false
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := w.(interface{ Fd() uintptr })
	return ok && (isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd()))
}
