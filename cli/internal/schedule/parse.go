package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cronSchedule represents a parsed 5-field cron expression.
type cronSchedule struct {
	minutes    []int // 0-59
	hours      []int // 0-23
	daysOfMonth []int // 1-31
	months     []int // 1-12
	daysOfWeek []int // 0-6 (0=Sunday)
}

// parseCronExpr parses a standard 5-field cron expression and returns a
// cronSchedule. Supported syntax:
//   - `*`    — all valid values
//   - `N`    — single value
//   - `*/N`  — step values
//   - `N-M`  — range
//   - `N,M`  — list (may include ranges and steps)
func parseCronExpr(expr string) (*cronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d", len(fields))
	}

	minutes, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("cron: minute field: %w", err)
	}
	hours, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("cron: hour field: %w", err)
	}
	dom, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("cron: day-of-month field: %w", err)
	}
	months, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("cron: month field: %w", err)
	}
	dow, err := parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("cron: day-of-week field: %w", err)
	}

	return &cronSchedule{
		minutes:    minutes,
		hours:      hours,
		daysOfMonth: dom,
		months:     months,
		daysOfWeek: dow,
	}, nil
}

// parseField parses a single cron field into a sorted slice of integers.
func parseField(field string, min, max int) ([]int, error) {
	// Handle comma-separated lists.
	parts := strings.Split(field, ",")
	seen := make(map[int]bool)

	for _, part := range parts {
		values, err := parsePart(part, min, max)
		if err != nil {
			return nil, err
		}
		for _, v := range values {
			seen[v] = true
		}
	}

	result := make([]int, 0, len(seen))
	for v := range seen {
		result = append(result, v)
	}
	sortInts(result)
	return result, nil
}

// parsePart parses a single part of a cron field (no commas). Supports:
// `*`, `*/N`, `N`, `N-M`, `N-M/S`
func parsePart(part string, min, max int) ([]int, error) {
	// Check for step value.
	stepStr := ""
	base := part
	if idx := strings.Index(part, "/"); idx != -1 {
		base = part[:idx]
		stepStr = part[idx+1:]
	}

	var rangeStart, rangeEnd int

	if base == "*" {
		rangeStart = min
		rangeEnd = max
	} else if idx := strings.Index(base, "-"); idx != -1 {
		var err error
		rangeStart, err = strconv.Atoi(base[:idx])
		if err != nil {
			return nil, fmt.Errorf("invalid range start %q", base[:idx])
		}
		rangeEnd, err = strconv.Atoi(base[idx+1:])
		if err != nil {
			return nil, fmt.Errorf("invalid range end %q", base[idx+1:])
		}
	} else {
		n, err := strconv.Atoi(base)
		if err != nil {
			return nil, fmt.Errorf("invalid value %q", base)
		}
		if stepStr == "" {
			if n < min || n > max {
				return nil, fmt.Errorf("value %d out of range [%d, %d]", n, min, max)
			}
			return []int{n}, nil
		}
		rangeStart = n
		rangeEnd = max
	}

	if rangeStart < min || rangeEnd > max || rangeStart > rangeEnd {
		return nil, fmt.Errorf("range %d-%d out of bounds [%d, %d]", rangeStart, rangeEnd, min, max)
	}

	step := 1
	if stepStr != "" {
		var err error
		step, err = strconv.Atoi(stepStr)
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step %q", stepStr)
		}
	}

	var values []int
	for i := rangeStart; i <= rangeEnd; i += step {
		values = append(values, i)
	}
	return values, nil
}

// nextRun computes the next time after `from` that matches the cron schedule.
// It searches up to 366 days into the future to avoid infinite loops.
func (s *cronSchedule) nextRun(from time.Time) (time.Time, error) {
	// Start from the next minute boundary.
	t := from.Truncate(time.Minute).Add(time.Minute)

	// Search limit: ~366 days.
	limit := from.Add(366 * 24 * time.Hour)

	for t.Before(limit) {
		// Check month.
		if !contains(s.months, int(t.Month())) {
			// Advance to the first day of the next month.
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}

		// Check day-of-month and day-of-week.
		// In standard cron, if both DOM and DOW are restricted (not *), a day
		// matches if EITHER condition is met. However, for simplicity and since
		// the spec says "support standard 5-field", we require both to match.
		dom := t.Day()
		dow := int(t.Weekday()) // 0=Sunday

		if !contains(s.daysOfMonth, dom) || !contains(s.daysOfWeek, dow) {
			// Advance to the next day.
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}

		// Check hour.
		if !contains(s.hours, t.Hour()) {
			// Advance to the next hour.
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}

		// Check minute.
		if !contains(s.minutes, t.Minute()) {
			t = t.Add(time.Minute)
			continue
		}

		return t, nil
	}

	return time.Time{}, fmt.Errorf("cron: no matching time found within 366 days")
}

// NextRunAfter computes the next occurrence of the given cron expression after
// the specified time. This is the primary public API for cron parsing.
func NextRunAfter(cronExpr string, after time.Time) (time.Time, error) {
	sched, err := parseCronExpr(cronExpr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.nextRun(after)
}

// contains checks if a sorted slice contains a value.
func contains(sorted []int, val int) bool {
	for _, v := range sorted {
		if v == val {
			return true
		}
		if v > val {
			return false
		}
	}
	return false
}

// sortInts sorts a slice of ints in ascending order (simple insertion sort,
// since cron field slices are small).
func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		key := a[i]
		j := i - 1
		for j >= 0 && a[j] > key {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = key
	}
}
