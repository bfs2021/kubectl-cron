// Copyright 2026 Mert Öztürk
// SPDX-License-Identifier: Apache-2.0

package schedule

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/robfig/cron/v3"
)

func Describe(expression string, parsed cron.Schedule) string {
	if s, ok := parsed.(cron.ConstantDelaySchedule); ok {
		return "every " + s.Delay.String() + " from the reference point"
	}
	s, ok := parsed.(*cron.SpecSchedule)
	if !ok {
		return expression
	}
	if s.Minute == all(0, 59) && s.Hour == all(0, 23) && s.Dom == all(1, 31) && s.Month == all(1, 12) && s.Dow == all(0, 6) {
		return "every minute"
	}
	fields := strings.Fields(expression)
	if len(fields) == 5 && fields[1] == "*" && fields[2] == "*" && fields[3] == "*" && fields[4] == "*" && strings.HasPrefix(fields[0], "*/") {
		if n, err := strconv.Atoi(strings.TrimPrefix(fields[0], "*/")); err == nil && n > 0 && 60%n == 0 {
			return fmt.Sprintf("every %d minutes", n)
		}
	}
	hours, minutes := values(s.Hour, 0, 23), values(s.Minute, 0, 59)
	if len(hours) == 24 && len(minutes) == 1 && len(values(s.Dom, 1, 31)) == 31 && len(values(s.Dow, 0, 6)) == 7 && len(values(s.Month, 1, 12)) == 12 {
		if minutes[0] == 0 {
			return "every hour"
		}
		return fmt.Sprintf("at minute %d past every hour", minutes[0])
	}
	clock := "at minute " + compact(minutes, nil) + " of hour " + compact(hours, nil)
	if len(hours) == 1 && len(minutes) == 1 {
		clock = fmt.Sprintf("at %02d:%02d", hours[0], minutes[0])
	}
	const star = uint64(1) << 63
	domStar, dowStar := s.Dom&star != 0, s.Dow&star != 0
	dom := "day " + compact(values(s.Dom, 1, 31), nil) + " of the month"
	dow := compact(values(s.Dow, 0, 6), []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"})
	day := "every day"
	switch {
	case !domStar && !dowStar:
		day = dom + " OR " + dow
	case !domStar:
		day = dom
	case !dowStar:
		day = dow
	}
	result := clock + ", " + day
	if s.Month&star == 0 {
		months := []string{"", "January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
		result += ", in " + compact(values(s.Month, 1, 12), months)
	}
	return result
}

func all(min, max int) uint64 {
	var bits uint64 = 1 << 63
	for i := min; i <= max; i++ {
		bits |= 1 << uint(i)
	}
	return bits
}
func values(bits uint64, min, max int) []int {
	var result []int
	for i := min; i <= max; i++ {
		if bits&(1<<uint(i)) != 0 {
			result = append(result, i)
		}
	}
	return result
}
func compact(v []int, names []string) string {
	label := func(n int) string {
		if names != nil {
			return names[n]
		}
		return strconv.Itoa(n)
	}
	var parts []string
	for i := 0; i < len(v); i++ {
		end := i
		for end+1 < len(v) && v[end+1] == v[end]+1 {
			end++
		}
		if end-i >= 2 {
			parts = append(parts, label(v[i])+" through "+label(v[end]))
			i = end
		} else {
			parts = append(parts, label(v[i]))
		}
	}
	return strings.Join(parts, ", ")
}
