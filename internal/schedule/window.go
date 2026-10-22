// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package schedule

import "time"

// Window returns at most limit occurrences in [start, end]. When truncated,
// keepLatest selects the newest occurrences without iterating an unbounded past.
func Window(in Input, start, end time.Time, limit int, keepLatest bool) ([]time.Time, string, bool) {
	if limit < 1 || end.Before(start) {
		return []time.Time{}, "", false
	}
	read := func(from time.Time) ([]time.Time, string, bool) {
		in.Now, in.Count = from.Add(-time.Nanosecond), limit+1
		s, _ := Evaluate(in)
		if s.Next.Time == nil {
			return []time.Time{}, s.Next.Reason, false
		}
		dates := []time.Time{}
		for _, at := range s.Occurrences {
			if at.After(end) {
				break
			}
			dates = append(dates, at)
		}
		return dates, "", len(dates) > limit
	}
	dates, reason, truncated := read(start)
	if !truncated {
		return dates, reason, false
	}
	if !keepLatest {
		return dates[:limit], reason, true
	}
	// The count in [from,end] decreases monotonically as from moves forward.
	// Binary search bounds work even for uneven schedules and DST transitions.
	low, high := start, end.Add(time.Nanosecond)
	best := []time.Time{}
	for high.Sub(low) > time.Nanosecond {
		mid := low.Add(high.Sub(low) / 2)
		candidate, _, over := read(mid)
		if over {
			low = mid
		} else {
			high, best = mid, candidate
		}
	}
	return best, "", true
}
