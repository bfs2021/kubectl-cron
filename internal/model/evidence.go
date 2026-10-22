// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package model

// Availability describes a read, not the completeness of historical evidence.
type Availability struct {
	Resource string `json:"resource"`
	State    string `json:"state"`
	Reason   string `json:"reason,omitempty"`
	Message  string `json:"message,omitempty"`
}

type Measure struct {
	Seconds *float64 `json:"seconds"`
	Reason  string   `json:"reason,omitempty"`
	Basis   string   `json:"basis,omitempty"`
}

type Run struct {
	Name            string    `json:"name"`
	Namespace       string    `json:"namespace"`
	UID             string    `json:"uid"`
	Origin          string    `json:"origin"`
	State           string    `json:"state"`
	Reason          string    `json:"reason,omitempty"`
	Message         string    `json:"message,omitempty"`
	ManagedBy       string    `json:"managedBy"`
	Slot            Timestamp `json:"slot"`
	Created         Timestamp `json:"created"`
	ControllerStart Timestamp `json:"controllerStart"`
	Finished        Timestamp `json:"finished"`
	FinishSource    string    `json:"finishSource,omitempty"`
	SchedulingDelay Measure   `json:"schedulingDelay"`
	Duration        Measure   `json:"duration"`
	ActivePods      int32     `json:"activePods"`
	SucceededPods   int32     `json:"succeededPods"`
	FailedPods      int32     `json:"failedPods"`
}

type Event struct {
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	Reason       string    `json:"reason"`
	Message      string    `json:"message"`
	Observations int32     `json:"observations"`
	ObservedAt   Timestamp `json:"observedAt"`
}

type Inspection struct {
	CronJob       CronJob        `json:"cronJob"`
	Runs          []Run          `json:"runs"`
	RetainedRuns  *int           `json:"retainedRuns"`
	RunsTruncated bool           `json:"runsTruncated"`
	Events        []Event        `json:"events"`
	Evidence      []Availability `json:"evidence"`
}
