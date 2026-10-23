// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"testing"

	"github.com/mertdotcc/kubectl-cron/internal/model"
)

func TestWatchNoticePrintsARepeatedErrorOnlyOnceUntilItStreamsAgain(t *testing.T) {
	const err = "too old resource version: 1600 (1605)"
	cases := []struct {
		name       string
		connection model.WatchState
		last       string
		say        bool
		next       string
	}{
		{
			name:       "first sighting of an error prints",
			connection: model.WatchState{State: "reconnecting", LastError: err},
			last:       "",
			say:        true,
			next:       err,
		},
		{
			name:       "the same error on the next tick is silent",
			connection: model.WatchState{State: "reconnecting", LastError: err},
			last:       err,
			say:        false,
			next:       err,
		},
		{
			name:       "a streamed tick clears the notice",
			connection: model.WatchState{State: "streaming"},
			last:       err,
			say:        false,
			next:       "",
		},
		{
			name:       "a different error prints again",
			connection: model.WatchState{State: "reconnecting", LastError: "too old resource version: 1700 (1705)"},
			last:       err,
			say:        true,
			next:       "too old resource version: 1700 (1705)",
		},
		{
			name:       "an empty error is silent and keeps the last notice",
			connection: model.WatchState{State: "resyncing"},
			last:       err,
			say:        false,
			next:       err,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			say, next := watchNotice(tc.connection, tc.last)
			if say != tc.say || next != tc.next {
				t.Fatalf("watchNotice(%+v, %q) = (%v, %q), want (%v, %q)", tc.connection, tc.last, say, next, tc.say, tc.next)
			}
		})
	}
}
