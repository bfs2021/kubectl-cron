// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

type HistoryEntry struct {
	At    Timestamp `json:"at"`
	Basis string    `json:"basis"`
	State string    `json:"state"`
	Run   *Run      `json:"run,omitempty"`
}

type History struct {
	CronJob               CronJob        `json:"cronJob"`
	WindowStart           time.Time      `json:"windowStart"`
	WindowEnd             time.Time      `json:"windowEnd"`
	ProjectionBasis       string         `json:"projectionBasis"`
	ProjectionUnavailable string         `json:"projectionUnavailable,omitempty"`
	ProjectionTruncated   bool           `json:"projectionTruncated"`
	Entries               []HistoryEntry `json:"entries"`
	EntriesTruncated      bool           `json:"entriesTruncated"`
	RetainedRuns          *int           `json:"retainedRuns"`
	Evidence              []Availability `json:"evidence"`
}
