// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package analyze

import (
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestCoincidentSchedulesMessageMatchesTheCount(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	zone := "UTC"
	same := "05 12 * * *"
	pair := []batchv1.CronJob{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "a"}, Spec: batchv1.CronJobSpec{Schedule: same, TimeZone: &zone}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "b"}, Spec: batchv1.CronJobSpec{Schedule: same, TimeZone: &zone}},
	}
	r := Calendar(pair, "file", now, time.Hour, 100, Options{})
	if len(r.Calendar.Findings) != 1 {
		t.Fatalf("expected one coincident-schedules finding: %+v", r.Calendar.Findings)
	}
	if got := r.Calendar.Findings[0].Message; !strings.Contains(got, "1 displayed nominal time is shared") || strings.Contains(got, "times are") {
		t.Fatalf("singular count reads wrong: %q", got)
	}

	plural := append(pair,
		batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "c"}, Spec: batchv1.CronJobSpec{Schedule: "35 12 * * *", TimeZone: &zone}},
		batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "d"}, Spec: batchv1.CronJobSpec{Schedule: "35 12 * * *", TimeZone: &zone}},
	)
	r2 := Calendar(plural, "file", now, time.Hour, 100, Options{})
	if got := r2.Calendar.Findings[0].Message; !strings.Contains(got, "2 displayed nominal times are shared") {
		t.Fatalf("plural count reads wrong: %q", got)
	}
}
