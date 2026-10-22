// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package model

type Diagnostic struct {
	Namespace string         `json:"namespace"`
	Name      string         `json:"name"`
	Findings  []Finding      `json:"findings,omitempty"`
	Evidence  []Availability `json:"evidence"`
}
