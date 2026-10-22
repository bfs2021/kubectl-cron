// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

// Package render presents a Report without adding observations of its own.
package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/fatih/color"
	"github.com/mertdotcc/kubectl-cron/internal/model"
)

type Options struct {
	Wide       bool
	Namespaces bool
	NoFindings bool
	Explain    bool
	Color      bool
}

func Human(out, notes io.Writer, report model.Report, opts Options) error {
	if report.History != nil {
		return history(out, notes, *report.History, opts)
	}
	if report.Inspection != nil {
		return inspection(out, notes, *report.Inspection, report.GeneratedAt, opts)
	}
	if report.Explanation != nil {
		return explanation(out, notes, "", report.Explanation.Schedule, report.Explanation.Findings, report.GeneratedAt, opts)
	}
	rows := report.Rows()
	if opts.Explain {
		if len(rows) == 0 {
			_, err := fmt.Fprintln(notes, "No CronJobs found.")
			return err
		}
		for i, row := range rows {
			if i > 0 {
				if _, err := fmt.Fprintln(out); err != nil {
					return err
				}
			}
			if err := explanation(out, notes, identity(row), row.Schedule, row.Findings, report.GeneratedAt, opts); err != nil {
				return err
			}
		}
		return nil
	}
	if report.CronJobs != nil && len(rows) == 0 {
		_, err := fmt.Fprintln(notes, "No CronJobs found.")
		return err
	}
	showNS := opts.Namespaces
	for _, row := range rows {
		if row.Namespace != "" && len(rows) > 0 && row.Namespace != rows[0].Namespace {
			showNS = true
		}
	}
	if report.CronJobs != nil {
		t := table{}
		header := []cell{plain("NAME"), plain("SCHEDULE"), plain("NEXT"), plain("LAST SCHEDULE"), plain("LAST SUCCESS"), plain("ACTIVE"), plain("SUSPEND")}
		if showNS {
			header = append([]cell{plain("NAMESPACE")}, header...)
		}
		if !opts.NoFindings {
			header = append(header, plain("FINDINGS"))
		}
		if opts.Wide {
			for _, h := range []string{"TIMEZONE", "CONCURRENCY", "DEADLINE", "HISTORY", "DESCRIPTION"} {
				header = append(header, plain(h))
			}
		}
		t.add(header...)
		assumed, unknown := false, false
		for _, row := range rows {
			name := row.Name
			if name == "" {
				name = "<unnamed>"
			}
			fields := []cell{plain(safe(name)), plain(safe(row.Schedule.Expression)), nextCell(row.Schedule, report.GeneratedAt, opts), plain(age(row.LastSchedule.Time, report.GeneratedAt)), plain(age(row.LastSuccess.Time, report.GeneratedAt)), plain(strconv.Itoa(row.ActiveReferences)), plain(strconv.FormatBool(row.Suspended))}
			if showNS {
				ns := row.Namespace
				if ns == "" {
					ns = "<unspecified>"
				}
				fields = append([]cell{plain(safe(ns))}, fields...)
			}
			if !opts.NoFindings {
				fields = append(fields, tint(strconv.Itoa(len(row.Findings)), opts.Color && len(row.Findings) > 0, color.FgYellow))
			}
			if opts.Wide {
				deadline := "—"
				if row.StartingDeadlineSeconds != nil {
					deadline = fmt.Sprintf("%ds", *row.StartingDeadlineSeconds)
				}
				fields = append(fields, plain(timezoneLabel(row.Schedule.Timezone)), plain(safe(row.Concurrency)), plain(deadline), plain(fmt.Sprintf("%d/%d", row.SuccessfulHistoryLimit, row.FailedHistoryLimit)), plain(safe(row.Schedule.Description)))
			}
			t.add(fields...)
			assumed = assumed || row.Schedule.Timezone.Source == "assumed"
			unknown = unknown || (row.Schedule.Next.Time == nil && row.Schedule.Next.Reason != "suspended")
		}
		if err := t.write(out); err != nil {
			return err
		}
		if assumed {
			if _, err := fmt.Fprintln(notes, "* Timezone supplied by --assume-timezone; controller timezone is unverified."); err != nil {
				return err
			}
		}
		if unknown {
			if _, err := fmt.Fprintln(notes, "? Upcoming occurrence unavailable; see timezone and calculation reasons in wide or structured output."); err != nil {
				return err
			}
		}
	}
	if report.Summary != nil {
		if report.CronJobs != nil {
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
		}
		s := report.Summary
		if _, err := fmt.Fprintf(out, "Summary\n  CronJobs: %d; suspended: %d; with active Job references: %d\n  Timezone configured: %d; unspecified: %d; explicitly assumed: %d\n", s.Total, s.Suspended, s.WithActiveReferences, s.TimezoneConfigured, s.TimezoneUnspecified, s.TimezoneAssumed); err != nil {
			return err
		}
	}
	if !opts.NoFindings {
		for _, row := range rows {
			if len(row.Findings) > 0 {
				if _, err := fmt.Fprintln(out); err != nil {
					return err
				}
				if err := findings(out, identity(row), row.Findings, opts); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func explanation(out, notes io.Writer, name string, s model.Schedule, items []model.Finding, now time.Time, opts Options) error {
	if name != "" {
		if _, err := fmt.Fprintf(out, "CronJob %s\n", safe(name)); err != nil {
			return err
		}
	}
	description := s.Description
	if description == "" {
		description = "unavailable"
	}
	if _, err := fmt.Fprintf(out, "%s  →  %s\nTimezone: %s\n", safe(s.Expression), safe(description), timezoneLabel(s.Timezone)); err != nil {
		return err
	}
	if s.Anchor != nil {
		if _, err := fmt.Fprintf(out, "Reference: %s (%s)\n", s.Anchor.Format(time.RFC3339), safe(s.AnchorSource)); err != nil {
			return err
		}
	}
	if len(s.Occurrences) == 0 {
		if _, err := fmt.Fprintf(out, "Next occurrence: unavailable (%s)\n", safe(s.Next.Reason)); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(out, "\nNext %d occurrences:\n", len(s.Occurrences)); err != nil {
			return err
		}
		for _, at := range s.Occurrences {
			if _, err := fmt.Fprintf(out, "  %s  (%s)\n", at.Format("Mon 2006-01-02 15:04:05 -07:00"), until(at, now)); err != nil {
				return err
			}
		}
	}
	if s.Timezone.Source == "assumed" {
		if _, err := fmt.Fprintln(notes, "* Timezone is an explicit assumption, not a discovered controller setting."); err != nil {
			return err
		}
	}
	if !opts.NoFindings && len(items) > 0 {
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		return findings(out, "", items, opts)
	}
	return nil
}

func findings(out io.Writer, name string, items []model.Finding, opts Options) error {
	heading := "Findings"
	if name != "" {
		heading += " — " + safe(name)
	}
	if _, err := fmt.Fprintln(out, heading); err != nil {
		return err
	}
	for _, f := range items {
		rule := f.Rule
		if opts.Color {
			c := color.New(color.FgYellow)
			c.EnableColor()
			rule = c.Sprint(rule)
		}
		if _, err := fmt.Fprintf(out, "  %s  %s\n", rule, safe(f.Message)); err != nil {
			return err
		}
	}
	return nil
}

func nextCell(s model.Schedule, now time.Time, opts Options) cell {
	if s.Next.Reason == "suspended" {
		return tint("SUSPENDED", opts.Color, color.FgHiBlack)
	}
	if s.Next.Time == nil {
		return tint("?", opts.Color, color.FgHiBlack)
	}
	text := until(*s.Next.Time, now)
	if s.Timezone.Source == "assumed" {
		text += "*"
	}
	return plain(text)
}

func tint(text string, enabled bool, attribute color.Attribute) cell {
	if !enabled {
		return plain(text)
	}
	c := color.New(attribute)
	c.EnableColor()
	return cell{text: text, colored: c.Sprint(text)}
}
func timezoneLabel(t model.Timezone) string {
	if t.Name == nil {
		return "controller-local (unknown)"
	}
	label := safe(*t.Name)
	switch t.Source {
	case "assumed":
		label += " (assumed)"
	case "offline-default":
		label += " (offline default)"
	case "argument":
		label += " (argument)"
	}
	return label
}
func identity(r model.CronJob) string {
	if r.Namespace == "" {
		return r.Name
	}
	return r.Namespace + "/" + r.Name
}
func until(at, now time.Time) string {
	if at.Before(now) {
		return duration(now.Sub(at)) + " ago"
	}
	return "in " + duration(at.Sub(now))
}
func age(at *time.Time, now time.Time) string {
	if at == nil {
		return "—"
	}
	if at.After(now) {
		return "in " + duration(at.Sub(now))
	}
	return duration(now.Sub(*at)) + " ago"
}
func duration(d time.Duration) string {
	if d < time.Second {
		return "<1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int64(d/time.Second))
	}
	if d < time.Hour {
		if d/time.Second%60 == 0 {
			return fmt.Sprintf("%dm", int64(d/time.Minute))
		}
		return fmt.Sprintf("%dm%ds", int64(d/time.Minute), int64(d/time.Second)%60)
	}
	if d < 24*time.Hour {
		if d/time.Minute%60 == 0 {
			return fmt.Sprintf("%dh", int64(d/time.Hour))
		}
		return fmt.Sprintf("%dh%dm", int64(d/time.Hour), int64(d/time.Minute)%60)
	}
	if d/time.Hour%24 == 0 {
		return fmt.Sprintf("%dd", int64(d/(24*time.Hour)))
	}
	return fmt.Sprintf("%dd%dh", int64(d/(24*time.Hour)), int64(d/time.Hour)%24)
}
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
