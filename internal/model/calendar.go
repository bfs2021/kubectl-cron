// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

type CalendarMember struct {
	Namespace  string    `json:"namespace"`
	Name       string    `json:"name"`
	Expression string    `json:"expression"`
	Timezone   Timezone  `json:"timezone"`
	LocalTime  time.Time `json:"localTime"`
}
type CalendarSlot struct {
	At       time.Time        `json:"at"`
	Count    int              `json:"count"`
	CronJobs []CalendarMember `json:"cronJobs"`
}
type CalendarExclusion struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Reason    string `json:"reason"`
}
type Calendar struct {
	WindowStart time.Time           `json:"windowStart"`
	WindowEnd   time.Time           `json:"windowEnd"`
	CronJobs    []CronJob           `json:"cronJobs"`
	Slots       []CalendarSlot      `json:"slots"`
	Excluded    []CalendarExclusion `json:"excluded"`
	Truncated   bool                `json:"truncated"`
	Findings    []Finding           `json:"findings,omitempty"`
}
