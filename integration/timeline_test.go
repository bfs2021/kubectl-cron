// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mertdotcc/kubectl-cron/internal/model"
)

func TestTimelineUsesHistoryStatesAndOrder(t *testing.T) {
	config := serveEvidence(t, fixtureData(), "")
	out, notes, err := command("", "timeline", "ledger", "--kubeconfig", config, "--since=1h", "--limit=100", "--color=never")
	if err != nil || !strings.Contains(out, "|???+x+>|") || !strings.Contains(out, "complete=2 failed=1 running=1 unknown=3") || !strings.Contains(out, "not proportional elapsed time") || !strings.Contains(notes, "past configuration") {
		t.Fatalf("%v\n%s\n%s", err, out, notes)
	}
	var reports [2]model.Report
	for i, name := range []string{"runs", "timeline"} {
		out, _, err := command("", name, "ledger", "--kubeconfig", config, "--since=1h", "--limit=100", "-o", "json")
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(out), &reports[i]); err != nil {
			t.Fatal(err)
		}
	}
	if reports[1].Kind != "CronJobTimeline" || !reflect.DeepEqual(reports[0].History, reports[1].History) {
		t.Fatal("timeline changed historical evidence")
	}
	out, notes, err = command("", "timeline", "ledger", "--kubeconfig", config, "--since=1h", "--limit=2")
	if err != nil || !strings.Contains(out, "|+>|") || !strings.Contains(notes, "truncated") {
		t.Fatalf("truncation hidden: %v %s %s", err, out, notes)
	}
}
