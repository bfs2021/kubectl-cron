// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package model

// WatchState describes the connection, not a guarantee of historical coverage.
type WatchState struct {
	Mode      string `json:"mode"`
	State     string `json:"state"`
	LastError string `json:"lastError,omitempty"`
}
