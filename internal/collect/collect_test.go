// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package collect

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	typedbatch "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/rest"
)

func TestOfflineDocumentsAndLists(t *testing.T) {
	input := `apiVersion: batch/v1
kind: CronJob
metadata:
  name: first
spec:
  schedule: "@daily"
---
apiVersion: v1
kind: List
items:
- apiVersion: batch/v1
  kind: CronJob
  metadata:
    name: second
  spec:
    schedule: "@hourly"
`
	jobs, err := Read(strings.NewReader(input))
	if err != nil || len(jobs) != 2 {
		t.Fatalf("%v: %+v", err, jobs)
	}
	if jobs[0].Namespace != "" {
		t.Fatal("invented namespace")
	}
	if _, err := Read(strings.NewReader(input + "---\napiVersion: v1\nkind: Pod\n")); err == nil {
		t.Fatal("unsupported document accepted")
	}
	if _, err := Read(strings.NewReader("kind: [broken")); err == nil {
		t.Fatal("malformed YAML accepted")
	}
}

func TestLiveListUsesOnlyCronJobsAndPaginates(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "GET" || r.URL.Path != "/apis/batch/v1/namespaces/finance/cronjobs" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("labelSelector") != "team=ledger" || r.URL.Query().Get("limit") != "500" {
			t.Errorf("query=%s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("resourceVersion") != "" {
			t.Error("stale-read request")
		}
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			fmt.Fprint(w, `{"apiVersion":"batch/v1","kind":"CronJobList","metadata":{"continue":"page-two"},"items":[{"metadata":{"name":"first"},"spec":{"schedule":"@daily"}}]}`)
		} else {
			if r.URL.Query().Get("continue") != "page-two" {
				t.Error("pagination token missing")
			}
			fmt.Fprint(w, `{"apiVersion":"batch/v1","kind":"CronJobList","items":[{"metadata":{"name":"second"},"spec":{"schedule":"@hourly"}}]}`)
		}
	}))
	defer server.Close()
	client, err := typedbatch.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := Live(context.Background(), client, Options{Namespace: "finance", Selector: "team=ledger"})
	if err != nil || len(jobs) != 2 || requests != 2 {
		t.Fatalf("jobs=%d requests=%d err=%v", len(jobs), requests, err)
	}
}

func TestTypedAPIListSuppliesItemType(t *testing.T) {
	input := `{"apiVersion":"batch/v1","kind":"CronJobList","items":[{"metadata":{"name":"typed"},"spec":{"schedule":"@hourly"}}]}`
	jobs, err := Read(strings.NewReader(input))
	if err != nil || len(jobs) != 1 || jobs[0].Name != "typed" {
		t.Fatalf("typed API list: %v %+v", err, jobs)
	}
	bad := `{"apiVersion":"v1","kind":"List","items":[{"metadata":{"name":"unknown"}}]}`
	if _, err := Read(strings.NewReader(bad)); err == nil {
		t.Fatal("untyped generic List item was guessed")
	}
}
