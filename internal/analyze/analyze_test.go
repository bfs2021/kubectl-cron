// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package analyze

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRecordedFactsAndTimezoneAuthority(t *testing.T) {
	now := time.Date(2026, 11, 5, 8, 12, 0, 0, time.UTC)
	utc := "UTC"
	suspended := true
	jobs := []batchv1.CronJob{
		{ObjectMeta: metav1.ObjectMeta{Name: "explicit"}, Spec: batchv1.CronJobSpec{Schedule: "0 * * * *", TimeZone: &utc}},
		{ObjectMeta: metav1.ObjectMeta{Name: "unspecified"}, Spec: batchv1.CronJobSpec{Schedule: "0 * * * *"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "suspended"}, Spec: batchv1.CronJobSpec{Schedule: "broken", TimeZone: &utc, Suspend: &suspended}},
	}
	r := Overview(jobs, "file", now, Options{AssumeTimezone: "Asia/Tokyo"})
	rows := r.Rows()
	if rows[0].Schedule.Timezone.Source != "spec" || *rows[0].Schedule.Timezone.Name != "UTC" {
		t.Fatal("assumption overrode explicit timezone")
	}
	if rows[2].Schedule.Timezone.Source != "assumed" || len(rows[2].Findings) != 1 {
		t.Fatal("assumption was not marked", rows[2])
	}
	if rows[1].Schedule.Next.Reason != "suspended" || len(rows[1].Findings) != 1 || rows[1].Findings[0].Rule != "invalid-schedule" {
		t.Fatal("suspension hid a configuration problem", rows[1])
	}
	if rows[0].LastSchedule.Reason != "not-recorded" || rows[0].LastSuccess.Time != nil {
		t.Fatal("missing history became an observation")
	}
	if r.Summary.Total != 3 || r.Summary.Suspended != 1 || r.Summary.TimezoneConfigured != 2 || r.Summary.TimezoneAssumed != 1 {
		t.Fatal(r.Summary)
	}
	encoded, _ := json.Marshal(r)
	if strings.Contains(string(encoded), "0001-") || !strings.Contains(string(encoded), `"time":null`) {
		t.Fatal(string(encoded))
	}
}

func TestNoFindingsAndFilteredSummary(t *testing.T) {
	utc := "UTC"
	now := time.Now()
	jobs := []batchv1.CronJob{{ObjectMeta: metav1.ObjectMeta{Name: "fine"}, Spec: batchv1.CronJobSpec{Schedule: "@daily", TimeZone: &utc}}, {ObjectMeta: metav1.ObjectMeta{Name: "unknown"}, Spec: batchv1.CronJobSpec{Schedule: "@daily"}}}
	r := Overview(jobs, "file", now, Options{Problems: true})
	if len(r.Rows()) != 1 || r.Summary.Total != 1 || r.Rows()[0].Name != "unknown" {
		t.Fatal(r)
	}
	r = Overview(jobs, "file", now, Options{NoFindings: true})
	if r.HasFindings() || r.Rows()[1].Schedule.Next.Time != nil || r.Rows()[1].Schedule.Next.Reason == "" {
		t.Fatal("facts or uncertainty lost when suppressing findings")
	}
}
