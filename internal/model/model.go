// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

// Package model defines the shared human and machine Report contract.
package model

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"sigs.k8s.io/yaml"
)

const APIVersion = "cron.kubectl.dev/v1alpha1"

type Finding struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Timestamp keeps absence distinct from an observed instant.
type Timestamp struct {
	Time   *time.Time `json:"time"`
	Reason string     `json:"reason,omitempty"`
}

func Recorded(t *time.Time) Timestamp {
	if t == nil || t.IsZero() {
		return Timestamp{Reason: "not-recorded"}
	}
	v := t.UTC()
	return Timestamp{Time: &v}
}

type Timezone struct {
	Name   *string `json:"name"`
	Source string  `json:"source"`
}

type Schedule struct {
	Expression   string      `json:"expression"`
	Description  string      `json:"description,omitempty"`
	Timezone     Timezone    `json:"timezone"`
	Next         Timestamp   `json:"next"`
	Occurrences  []time.Time `json:"occurrences,omitempty"`
	Anchor       *time.Time  `json:"anchor,omitempty"`
	AnchorSource string      `json:"anchorSource,omitempty"`
}

type CronJob struct {
	Namespace               string    `json:"namespace"`
	Name                    string    `json:"name"`
	Schedule                Schedule  `json:"schedule"`
	LastSchedule            Timestamp `json:"lastSchedule"`
	LastSuccess             Timestamp `json:"lastSuccess"`
	ActiveReferences        int       `json:"activeReferences"`
	Suspended               bool      `json:"suspended"`
	Concurrency             string    `json:"concurrency"`
	StartingDeadlineSeconds *int64    `json:"startingDeadlineSeconds"`
	SuccessfulHistoryLimit  int32     `json:"successfulHistoryLimit"`
	FailedHistoryLimit      int32     `json:"failedHistoryLimit"`
	Findings                []Finding `json:"findings,omitempty"`
}

type Summary struct {
	Total                int `json:"total"`
	Suspended            int `json:"suspended"`
	WithActiveReferences int `json:"withActiveReferences"`
	TimezoneConfigured   int `json:"timezoneConfigured"`
	TimezoneUnspecified  int `json:"timezoneUnspecified"`
	TimezoneAssumed      int `json:"timezoneAssumed"`
}

type Explanation struct {
	Schedule Schedule  `json:"schedule"`
	Findings []Finding `json:"findings,omitempty"`
}

type Report struct {
	APIVersion  string    `json:"apiVersion"`
	Kind        string    `json:"kind"`
	GeneratedAt time.Time `json:"generatedAt"`
	Source      string    `json:"source"`
	// A pointer distinguishes an empty selection from Summary-only projection.
	CronJobs    *[]CronJob    `json:"cronJobs,omitempty"`
	Summary     *Summary      `json:"summary,omitempty"`
	Explanation *Explanation  `json:"explanation,omitempty"`
	Inspection  *Inspection   `json:"inspection,omitempty"`
	History     *History      `json:"history,omitempty"`
	Diagnostics *[]Diagnostic `json:"diagnostics,omitempty"`
}

func (r Report) Rows() []CronJob {
	if r.CronJobs == nil {
		return nil
	}
	return *r.CronJobs
}

func (r Report) HasFindings() bool {
	if r.Diagnostics != nil {
		for _, row := range *r.Diagnostics {
			if len(row.Findings) > 0 {
				return true
			}
		}
	}
	if r.History != nil && len(r.History.CronJob.Findings) > 0 {
		return true
	}
	if r.Inspection != nil && len(r.Inspection.CronJob.Findings) > 0 {
		return true
	}
	if r.Explanation != nil && len(r.Explanation.Findings) > 0 {
		return true
	}
	for _, row := range r.Rows() {
		if len(row.Findings) > 0 {
			return true
		}
	}
	return false
}

func (r Report) Encode(w io.Writer, format string) error {
	switch format {
	case "json":
		e := json.NewEncoder(w)
		e.SetIndent("", "  ")
		e.SetEscapeHTML(false)
		return e.Encode(r)
	case "yaml":
		data, err := yaml.Marshal(r)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	default:
		return fmt.Errorf("unsupported report format %q", format)
	}
}
