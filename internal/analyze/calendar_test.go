// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package analyze

import (
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
)

func TestCalendarDoesNotMergeRepeatedLocalClockTimes(t *testing.T) {
	zone := "Europe/Berlin"
	now := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	cj := batchv1.CronJob{Spec: batchv1.CronJobSpec{Schedule: "30 2 * * *", TimeZone: &zone}}
	r := Calendar([]batchv1.CronJob{cj}, "file", now, 3*time.Hour, 100, Options{})
	c := r.Calendar
	if len(c.Slots) != 2 || c.Slots[0].Count != 1 || c.Slots[1].Count != 1 || c.Slots[1].At.Sub(c.Slots[0].At) != time.Hour {
		t.Fatalf("DST times collapsed: %+v", c)
	}
}

func TestCoincidentSchedulesMessageMatchesCount(t *testing.T) {
	zone := "UTC"
	now := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)
	cj := batchv1.CronJob{Spec: batchv1.CronJobSpec{Schedule: "0 * * * *", TimeZone: &zone}}
	other := *cj.DeepCopy()
	other.Name = "other"
	for window, want := range map[time.Duration]string{time.Hour: "1 displayed nominal time is shared", 2 * time.Hour: "2 displayed nominal times are shared"} {
		c := Calendar([]batchv1.CronJob{cj, other}, "file", now, window, 100, Options{}).Calendar
		if len(c.Findings) != 1 || !strings.HasPrefix(c.Findings[0].Message, want+" by selected CronJobs") {
			t.Fatalf("%s: %+v", window, c.Findings)
		}
	}
}
