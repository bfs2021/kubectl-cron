// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package schedule

import (
	"testing"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/model"
)

func TestWindowRetainsNewestWhenBounded(t *testing.T) {
	zone := "UTC"
	end := time.Date(2026, 11, 5, 8, 12, 0, 0, time.UTC)
	in := Input{Expression: "* * * * *", Timezone: model.Timezone{Name: &zone, Source: "spec"}}
	dates, reason, truncated := Window(in, end.Add(-30*24*time.Hour), end, 3, true)
	if reason != "" || !truncated || len(dates) != 3 || !dates[0].Equal(end.Add(-2*time.Minute)) || !dates[2].Equal(end) {
		t.Fatalf("%v %s %t", dates, reason, truncated)
	}
	dates, reason, truncated = Window(in, end.Add(-30*24*time.Hour), end, 3, false)
	if reason != "" || !truncated || len(dates) != 3 || !dates[0].Equal(end.Add(-30*24*time.Hour)) {
		t.Fatalf("%v %s %t", dates, reason, truncated)
	}
}

func TestWindowKeepsBothFallBackOccurrences(t *testing.T) {
	zone := "Europe/Berlin"
	start := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	dates, reason, truncated := Window(Input{Expression: "30 2 * * *", Timezone: model.Timezone{Name: &zone, Source: "spec"}}, start, start.Add(3*time.Hour), 5, true)
	if reason != "" || truncated || len(dates) != 2 || dates[1].Sub(dates[0]) != time.Hour || dates[0].Format("15:04 -07:00") == dates[1].Format("15:04 -07:00") {
		t.Fatalf("%v %s %t", dates, reason, truncated)
	}
}
