// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

// Package integration exercises the command against saved API data and local inputs.
package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/cli"
)

var update = flag.Bool("update", false, "Update reviewed pipeline golden files")
var reference = time.Date(2026, 11, 5, 8, 12, 0, 0, time.UTC)

func command(input string, args ...string) (string, string, error) {
	var out, notes bytes.Buffer
	cmd := cli.New(strings.NewReader(input), &out, &notes, func() time.Time { return reference }, "test")
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), notes.String(), err
}

func TestPipelineGolden(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"overview.txt", []string{"-f", "../testdata/cronjobs.yaml", "--color=never"}},
		{"wide.txt", []string{"-f", "../testdata/cronjobs.yaml", "-o", "wide", "--color=never"}},
		{"report.json", []string{"-f", "../testdata/cronjobs.yaml", "-o", "json"}},
		{"report.yaml", []string{"-f", "../testdata/cronjobs.yaml", "-o", "yaml"}},
		{"summary.json", []string{"-f", "../testdata/cronjobs.yaml", "--summary=only", "-o", "json"}},
		{"explain.txt", []string{"explain", "0 6 * * MON-FRI", "--tz", "Europe/Berlin", "--count=3", "--color=never"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := command("", tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", "golden", tc.name)
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(out), 0644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if out != string(want) {
				t.Fatalf("report differs from %s\nactual:\n%s", path, out)
			}
		})
	}
}

func TestOfflineIgnoresBrokenClusterConfiguration(t *testing.T) {
	broken := filepath.Join(t.TempDir(), "broken-kubeconfig")
	if err := os.WriteFile(broken, []byte("not: [valid"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", broken)
	for _, args := range [][]string{
		{"explain", "@hourly", "--kubeconfig", broken, "-o", "json"},
		{"-f", "../testdata/cronjobs.yaml", "--kubeconfig", broken, "-o", "json"},
		{"explain", "-f", "../testdata/cronjobs.yaml", "--kubeconfig", broken, "--assume-timezone=UTC", "-o", "yaml"},
	} {
		out, _, err := command("", args...)
		if err != nil || out == "" {
			t.Fatalf("offline command contacted configuration: %v\n%s", err, out)
		}
	}
}

func TestExitCodesAndAtomicErrors(t *testing.T) {
	cases := []struct {
		args []string
		code int
	}{
		{[]string{"explain", "bad schedule"}, 0},
		{[]string{"explain", "bad schedule", "--exit-on-findings"}, 2},
		{[]string{"explain", "@hourly", "--exit-on-findings"}, 0},
		{[]string{"explain", "@hourly", "--tz=No/Such_Zone"}, 1},
		{[]string{"explain", "@hourly", "--no-findings", "--exit-on-findings"}, 1},
		{[]string{"--no-findings", "--problems"}, 1},
		{[]string{"-f", "../testdata/cronjobs.yaml", "-f", "/no/such/file", "--exit-on-findings"}, 1},
		{[]string{"explain", "@hourly", "-f", "../testdata/cronjobs.yaml"}, 1},
		{[]string{"explain", "-f", "../testdata/cronjobs.yaml", "--tz=UTC"}, 1},
		{[]string{"-f", "../testdata/cronjobs.yaml", "--summary=only", "--exit-on-findings", "-o", "json"}, 2},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			out, _, err := command("", tc.args...)
			if got := cli.ExitCode(err); got != tc.code {
				t.Fatalf("code=%d want=%d err=%v", got, tc.code, err)
			}
			if tc.code == 1 && out != "" {
				t.Fatal("operational error printed a partial success report", out)
			}
		})
	}
}

func TestReportUncertaintyAndFiltering(t *testing.T) {
	out, _, err := command("", "-f", "../testdata/cronjobs.yaml", "--assume-timezone=Asia/Tokyo", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		CronJobs []struct {
			Name     string
			Schedule struct {
				Timezone struct {
					Name   string
					Source string
				}
				Next struct {
					Time   *time.Time
					Reason string
				}
			}
			Findings []any
		}
		Summary struct{ Total int }
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	for _, row := range r.CronJobs {
		if row.Name == "ledger" && (row.Schedule.Timezone.Name != "UTC" || row.Schedule.Timezone.Source != "spec") {
			t.Fatal("explicit zone overridden")
		}
		if row.Name == "pulse" && (row.Schedule.Timezone.Source != "assumed" || row.Schedule.Next.Time == nil) {
			t.Fatal("assumption lost")
		}
	}
	out, _, err = command("", "-f", "../testdata/cronjobs.yaml", "--no-findings", "-o", "yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "findings:") || !strings.Contains(out, "controller-timezone-unknown") {
		t.Fatal("suppression changed facts", out)
	}
	out, _, err = command("", "-f", "../testdata/cronjobs.yaml", "--problems", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	if r.Summary.Total != 2 || len(r.CronJobs) != 2 {
		t.Fatal("Summary scope differs from filtered Overview", out)
	}
}

func TestLiveReadsCronJobsOnlyAndPropagatesForbidden(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(fmt.Sprint(denied), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != "GET" || r.URL.Path != "/apis/batch/v1/namespaces/finance/cronjobs/explain" {
					t.Errorf("unexpected cluster access: %s %s", r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				if denied {
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","message":"CronJob access denied","code":403}`)
					return
				}
				fmt.Fprint(w, `{"apiVersion":"batch/v1","kind":"CronJob","metadata":{"name":"explain","namespace":"finance"},"spec":{"schedule":"@hourly","timeZone":"UTC"}}`)
			}))
			defer server.Close()
			config := filepath.Join(t.TempDir(), "config")
			data := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: test\n  cluster:\n    server: %s\ncontexts:\n- name: test\n  context:\n    cluster: test\n    user: test\n    namespace: finance\ncurrent-context: test\nusers:\n- name: test\n  user: {}\n", server.URL)
			if err := os.WriteFile(config, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			out, _, err := command("", "cronjob/explain", "--kubeconfig", config, "-o", "json")
			if requests != 1 {
				t.Fatalf("requests=%d", requests)
			}
			if denied {
				if cli.ExitCode(err) != 1 || out != "" {
					t.Fatalf("denial hidden: %v %s", err, out)
				}
			} else if err != nil || !strings.Contains(out, `"name": "explain"`) {
				t.Fatalf("named lookup failed: %v %s", err, out)
			}
		})
	}
}

func TestHelpAndColor(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"explain", "--help"}} {
		out, _, err := command("", args...)
		if err != nil {
			t.Fatal(err)
		}
		want := "kubectl cron"
		if args[0] == "explain" {
			want += " explain"
		}
		if !strings.Contains(out, want) {
			t.Fatalf("missing plugin prefix %q:\n%s", want, out)
		}
	}
	out, _, err := command("", "-f", "../testdata/cronjobs.yaml", "--color=always")
	if err != nil || !strings.Contains(out, "\x1b[") {
		t.Fatalf("forced color missing: %v", err)
	}
	out, _, err = command("", "-f", "../testdata/cronjobs.yaml", "--color=always", "-o", "json")
	if err != nil || strings.Contains(out, "\x1b[") {
		t.Fatal("structured output was colored")
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestWriteErrorBeatsFindingExit(t *testing.T) {
	cmd := cli.New(strings.NewReader(""), brokenWriter{}, io.Discard, func() time.Time { return reference }, "")
	cmd.SetArgs([]string{"explain", "bad schedule", "--exit-on-findings", "-o", "json"})
	err := cmd.Execute()
	if !errors.Is(err, io.ErrClosedPipe) || cli.ExitCode(err) != 1 {
		t.Fatal(err)
	}
}
