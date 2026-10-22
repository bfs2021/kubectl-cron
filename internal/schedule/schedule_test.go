// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package schedule

import (
	"strings"
	"testing"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/model"
)

func instant(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
func zone(name string) model.Timezone { return model.Timezone{Name: &name, Source: "spec"} }

func TestCalendarBoundaries(t *testing.T) {
	cases := []struct{ name, expression, zone, now, want string }{
		{"strictly after now", "0 * * * *", "UTC", "2026-11-05T08:00:00Z", "2026-11-05T09:00:00Z"},
		{"spring gap skipped", "30 2 * * *", "Europe/Berlin", "2026-03-28T12:00:00Z", "2026-03-30T02:30:00+02:00"},
		{"calendar descriptor", "@monthly", "UTC", "2026-11-05T08:00:00Z", "2026-12-01T00:00:00Z"},
		{"day OR weekday", "0 0 31 2 MON", "UTC", "2026-02-01T00:00:00Z", "2026-02-02T00:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, findings := Evaluate(Input{Expression: tc.expression, Timezone: zone(tc.zone), Now: instant(tc.now), Count: 2})
			if len(findings) != 0 {
				t.Fatalf("unexpected findings: %+v", findings)
			}
			if got.Next.Time == nil || !got.Next.Time.Equal(instant(tc.want)) {
				t.Fatalf("next=%+v, want %s", got.Next, tc.want)
			}
		})
	}
}

func TestRepeatedWallClockTimesRetainOffsets(t *testing.T) {
	got, _ := Evaluate(Input{Expression: "30 2 * * *", Timezone: zone("Europe/Berlin"), Now: instant("2026-10-25T00:00:00Z"), Count: 2})
	if len(got.Occurrences) != 2 {
		t.Fatal(got)
	}
	if a, b := got.Occurrences[0].Format(time.RFC3339), got.Occurrences[1].Format(time.RFC3339); a != "2026-10-25T02:30:00+02:00" || b != "2026-10-25T02:30:00+01:00" {
		t.Fatalf("lost repeated occurrence: %s, %s", a, b)
	}
}

func TestUnavailableAndInvalid(t *testing.T) {
	cases := []struct {
		name, expression string
		timezone         model.Timezone
		reason, rule     string
	}{
		{"unknown zone", "0 * * * *", model.Timezone{Source: "controller-unknown"}, "controller-timezone-unknown", "timezone-not-set"},
		{"unloadable zone", "0 * * * *", zone("No/Such_Zone"), "timezone-unavailable", "timezone-unavailable"},
		{"empty zone", "0 * * * *", zone(""), "timezone-unavailable", "timezone-unavailable"},
		{"host-local zone", "0 * * * *", zone("Local"), "timezone-unavailable", "timezone-unavailable"},
		{"no date", "0 0 31 2 *", zone("UTC"), "no-next-occurrence", "no-next-occurrence"},
		{"invalid syntax", "not a schedule", zone("UTC"), "invalid-schedule", "invalid-schedule"},
		{"embedded timezone", "CRON_TZ=UTC 0 * * * *", zone("UTC"), "embedded-timezone", "embedded-timezone"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, findings := Evaluate(Input{Expression: tc.expression, Timezone: tc.timezone, Now: instant("2026-11-05T08:00:00Z")})
			if got.Next.Time != nil || got.Next.Reason != tc.reason {
				t.Fatalf("unexpected next: %+v", got.Next)
			}
			if len(findings) != 1 || findings[0].Rule != tc.rule {
				t.Fatalf("unexpected findings: %+v", findings)
			}
		})
	}
	if _, err := Parse("TZ=0"); err == nil {
		t.Fatal("malformed timezone prefix should return an error rather than panic")
	}
}

func TestRelativeSchedulePreservesReference(t *testing.T) {
	anchor := instant("2026-11-05T08:00:00Z")
	got, findings := Evaluate(Input{Expression: "@every 7m", Timezone: zone("UTC"), Now: instant("2026-11-05T08:11:00Z"), Anchor: &anchor, AnchorSource: "creation", Count: 3})
	if len(findings) > 0 || got.Next.Time == nil || !got.Next.Time.Equal(instant("2026-11-05T08:14:00Z")) {
		t.Fatalf("phase changed: %+v / %+v", got, findings)
	}
	if got.AnchorSource != "creation" || len(got.Occurrences) != 3 || got.Occurrences[2].Minute() != 28 {
		t.Fatal(got)
	}
	ancient := instant("1500-01-01T00:00:00Z")
	got, _ = Evaluate(Input{Expression: "@every 1s", Timezone: zone("UTC"), Now: instant("2026-11-05T08:11:00Z"), Anchor: &ancient})
	if got.Next.Time == nil || !got.Next.Time.Equal(instant("2026-11-05T08:11:01Z")) {
		t.Fatalf("old anchor overflow: %+v", got.Next)
	}
	got, findings = Evaluate(Input{Expression: "@every 1m", Timezone: zone("UTC"), Now: anchor})
	if got.Next.Reason != "relative-anchor-unavailable" || len(findings) != 1 {
		t.Fatalf("invented reference: %+v", got)
	}
}

func TestDescriptionExplainsDayORAndUnevenSteps(t *testing.T) {
	s, err := Parse("0 9 1 * MON")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(Describe("0 9 1 * MON", s), " OR ") {
		t.Fatal("day constraints must retain OR semantics")
	}
	s, _ = Parse("*/7 * * * *")
	if Describe("*/7 * * * *", s) == "every 7 minutes" {
		t.Fatal("seven-minute field steps reset at the hour")
	}
}
