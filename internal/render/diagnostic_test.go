// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mertdotcc/kubectl-cron/internal/model"
)

func TestDiagnosticsOfflineNotesHaveNoTrailingWhitespace(t *testing.T) {
	rows := []model.Diagnostic{{
		Namespace: "cron-demo",
		Name:      "controller-local",
		Evidence: []model.Availability{
			{Resource: "jobs", State: "missing", Reason: "offline-source"},
			{Resource: "events", State: "missing", Reason: "offline-source", Message: "expired 3 days ago"},
		},
	}}
	var out, notes bytes.Buffer
	if err := diagnostics(&out, &notes, rows, Options{}); err != nil {
		t.Fatal(err)
	}
	got := notes.String()
	if !strings.Contains(got, "cron-demo/controller-local: jobs evidence missing (offline-source).\n") {
		t.Fatalf("empty message must end after the reason: %q", got)
	}
	if !strings.Contains(got, "(offline-source). expired 3 days ago\n") {
		t.Fatalf("a message must follow the reason: %q", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if line != strings.TrimRight(line, " \t") {
			t.Fatalf("note line ends in whitespace: %q", line)
		}
	}
}
