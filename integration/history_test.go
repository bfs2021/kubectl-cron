// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mertdotcc/kubectl-cron/internal/cli"
	"github.com/mertdotcc/kubectl-cron/internal/model"
)

func TestHistoryCorrelatesRunsWithoutInventingMissingResults(t *testing.T) {
	f := fixtureData()
	config := serveEvidence(t, f, "")
	out, _, err := command("", "runs", "ledger", "--kubeconfig", config, "--since=15m", "--limit=100", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var r model.Report
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	h := r.History
	if r.Kind != "CronJobRunHistory" || h == nil || h.RetainedRuns == nil || *h.RetainedRuns != 3 || len(h.Entries) != 3 {
		t.Fatal(out)
	}
	for _, v := range h.Entries {
		if v.Run == nil || v.State == "unknown" {
			t.Fatal("recorded slots were not correlated", out)
		}
	}
	for _, path := range f.Paths {
		if strings.HasSuffix(path, "/events") {
			t.Fatal("history fetched Events without need")
		}
	}
	out, _, err = command("", "runs", "ledger", "--kubeconfig", config, "--since=1h", "--limit=100", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	unknown := 0
	for _, v := range r.History.Entries {
		if v.Run == nil {
			unknown++
			if v.State != "unknown" || v.Basis != "current-spec-projection" {
				t.Fatal(out)
			}
		}
	}
	if unknown != 3 {
		t.Fatal("missing slots not preserved as unknown", out)
	}
}

func TestHistoryUnknownZoneAndDeniedJobsPreserveUncertainty(t *testing.T) {
	f := fixtureData()
	f.CronJob.Spec.TimeZone = nil
	config := serveEvidence(t, f, "")
	out, _, err := command("", "runs", "ledger", "--kubeconfig", config, "-o", "json")
	if err != nil || !strings.Contains(out, `"projectionUnavailable": "controller-timezone-unknown"`) || !strings.Contains(out, `"name": "ad-hoc"`) {
		t.Fatalf("%v\n%s", err, out)
	}
	config = serveEvidence(t, fixtureData(), "jobs")
	out, _, err = command("", "runs", "ledger", "--kubeconfig", config, "-o", "yaml")
	if err != nil || !strings.Contains(out, "retainedRuns: null") || !strings.Contains(out, "state: forbidden") {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, flags := range [][]string{{"--since=0"}, {"--since=-1h"}, {"--since=8761h"}, {"--limit=0"}, {"--limit=1001"}, {"-f", "../testdata/cronjobs.yaml"}} {
		args := append([]string{"runs", "ledger"}, flags...)
		out, _, err := command("", args...)
		if cli.ExitCode(err) != 1 || out != "" {
			t.Fatalf("invalid flags accepted %v: %v %s", flags, err, out)
		}
	}
}
