// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

// Package schedule evaluates nominal occurrences, not controller executions.
package schedule

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/mertdotcc/kubectl-cron/internal/model"
	"github.com/robfig/cron/v3"
)

type Input struct {
	Expression   string
	Timezone     model.Timezone
	Now          time.Time
	Anchor       *time.Time
	AnchorSource string
	Count        int
}

// Parse matches the controller's panic-safe wrapper around ParseStandard.
func Parse(expression string) (result cron.Schedule, err error) {
	defer func() {
		if p := recover(); p != nil {
			result = nil
			err = fmt.Errorf("invalid schedule: %v", p)
		}
	}()
	return cron.ParseStandard(expression)
}

func Evaluate(in Input) (model.Schedule, []model.Finding) {
	out := model.Schedule{Expression: in.Expression, Timezone: in.Timezone}
	findings := []model.Finding{}
	add := func(rule, message string) { findings = append(findings, model.Finding{Rule: rule, Message: message}) }
	for _, field := range strings.Fields(in.Expression) {
		if strings.HasPrefix(field, "TZ=") || strings.HasPrefix(field, "CRON_TZ=") {
			out.Next.Reason = "embedded-timezone"
			add("embedded-timezone", "The expression embeds TZ or CRON_TZ, which the CronJob API does not support.")
			return out, findings
		}
	}
	parsed, err := Parse(in.Expression)
	if err != nil {
		out.Next.Reason = "invalid-schedule"
		add("invalid-schedule", fmt.Sprintf("The schedule cannot be parsed: %v", err))
		return out, findings
	}
	out.Description = Describe(in.Expression, parsed)
	if in.Timezone.Source == "controller-unknown" || in.Timezone.Source == "assumed" {
		message := "The CronJob does not declare its controller's timezone; upcoming occurrences are unknown."
		if in.Timezone.Source == "assumed" {
			message = "The CronJob has no configured timezone; calculations use an explicit operator assumption."
		}
		add("timezone-not-set", message)
	}
	if in.Timezone.Name == nil {
		out.Next.Reason = "controller-timezone-unknown"
		return out, findings
	}
	if *in.Timezone.Name == "" || strings.EqualFold(*in.Timezone.Name, "Local") {
		out.Next.Reason = "timezone-unavailable"
		add("timezone-unavailable", "The configured timezone is empty or host-local; it does not identify a portable timezone for this calculation.")
		return out, findings
	}
	loc, err := time.LoadLocation(*in.Timezone.Name)
	if err != nil {
		out.Next.Reason = "timezone-unavailable"
		add("timezone-unavailable", fmt.Sprintf("The timezone %q cannot be loaded by this client; controller behavior is unknown.", *in.Timezone.Name))
		return out, findings
	}
	if spec, ok := parsed.(*cron.SpecSchedule); ok {
		spec.Location = loc
	}
	now := in.Now.In(loc)
	next := time.Time{}
	if relative, ok := parsed.(cron.ConstantDelaySchedule); ok {
		if relative.Delay < time.Minute {
			add("sub-minute-schedule", "The cadence is shorter than one minute, while CronJob-generated Job names have minute precision.")
		}
		if in.Anchor == nil || in.Anchor.IsZero() {
			out.Next.Reason = "relative-anchor-unavailable"
			add("relative-anchor-unavailable", "The relative schedule has no recorded reference point for a dated preview.")
			return out, findings
		}
		anchor := in.Anchor.In(loc)
		out.Anchor = &anchor
		out.AnchorSource = in.AnchorSource
		next = relative.Next(anchor)
		if !next.After(now) {
			// Unix seconds avoid time.Duration overflow for old file timestamps.
			seconds := int64(relative.Delay / time.Second)
			steps := (now.Unix()-next.Unix())/seconds + 1
			next = time.Unix(next.Unix()+steps*seconds, 0).In(loc)
		}
	} else {
		next = parsed.Next(now)
	}
	if next.IsZero() {
		out.Next.Reason = "no-next-occurrence"
		add("no-next-occurrence", "The parser found no upcoming occurrence within its bounded search horizon.")
		return out, findings
	}
	count := in.Count
	if count < 1 {
		count = 1
	}
	for i := 0; i < count && !next.IsZero(); i++ {
		out.Occurrences = append(out.Occurrences, next.In(loc))
		next = parsed.Next(next)
	}
	first := out.Occurrences[0]
	out.Next.Time = &first
	return out, findings
}
