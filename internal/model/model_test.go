// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"sigs.k8s.io/yaml"
)

func TestRecordedTimestampsDoNotDependOnClientTimezone(t *testing.T) {
	input := time.Date(2026, 11, 5, 10, 0, 0, 0, time.FixedZone("client", 2*60*60))
	got := Recorded(&input)
	if got.Time == nil || got.Time.Format(time.RFC3339) != "2026-11-05T08:00:00Z" {
		t.Fatal(got)
	}
	if input.Hour() != 10 {
		t.Fatal("caller timestamp mutated")
	}
}

func TestJSONAndYAMLHaveSameStructureAndNulls(t *testing.T) {
	empty := []CronJob{}
	r := Report{APIVersion: APIVersion, Kind: "CronJobReport", GeneratedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Source: "file", CronJobs: &empty}
	var j, y bytes.Buffer
	if err := r.Encode(&j, "json"); err != nil {
		t.Fatal(err)
	}
	if err := r.Encode(&y, "yaml"); err != nil {
		t.Fatal(err)
	}
	yj, err := yaml.YAMLToJSON(y.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	if err := json.Unmarshal(j.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(yj, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("JSON and YAML diverged")
	}
	data := a.(map[string]any)
	if _, ok := data["cronJobs"].([]any); !ok {
		t.Fatal("empty selection was not an array")
	}
}
