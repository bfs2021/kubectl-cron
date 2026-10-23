// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mertdotcc/kubectl-cron/internal/model"
)

func TestTimelineEmptyWindowSkipsTheLegendAndCounts(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	h := model.History{WindowStart: now.Add(-time.Hour), WindowEnd: now}

	var out, notes bytes.Buffer
	if err := timeline(&out, &notes, h, Options{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "No entries in this window.") {
		t.Fatalf("empty window must say so: %q", got)
	}
	if strings.Contains(got, "+ complete") || strings.Contains(got, "Shown entries:") {
		t.Fatalf("empty window must not print the legend or a dangling label: %q", got)
	}

	h.Entries = []model.HistoryEntry{{At: model.Recorded(&now), Basis: "runs", State: "complete"}}
	var out2, notes2 bytes.Buffer
	if err := timeline(&out2, &notes2, h, Options{}); err != nil {
		t.Fatal(err)
	}
	got2 := out2.String()
	if !strings.Contains(got2, "+ complete") || !strings.Contains(got2, "Shown entries: complete=1\n") {
		t.Fatalf("entries must bring the legend and counts back: %q", got2)
	}
}
