// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package analyze

import (
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
