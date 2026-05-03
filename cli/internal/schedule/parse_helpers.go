package schedule

import (
	"fmt"
	"strconv"
	"strings"
)

// extractMinuteInterval parses a 5-field cron expression and returns the
// minute interval for use with Windows Task Scheduler's MINUTE schedule type.
// Supports `*/N` in the minute field (returns N), bare `*` (returns 1), and
// a single numeric value (returns 60, mapping to once-per-hour).
func extractMinuteInterval(cronExpr string) (int, error) {
	fields := strings.Fields(cronExpr)
	if len(fields) != 5 {
		return 0, fmt.Errorf("expected 5-field cron expression, got %d fields", len(fields))
	}

	minuteField := fields[0]

	if minuteField == "*" {
		return 1, nil
	}

	if strings.HasPrefix(minuteField, "*/") {
		n, err := strconv.Atoi(minuteField[2:])
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid step value in %q", minuteField)
		}
		return n, nil
	}

	// Single numeric value — run once per hour at that minute. Map to 60 min interval.
	_, err := strconv.Atoi(minuteField)
	if err == nil {
		return 60, nil
	}

	return 0, fmt.Errorf("unsupported minute field %q for Windows scheduler", minuteField)
}

// parseCSVLine splits a single CSV line, handling quoted fields.
func parseCSVLine(line string) []string {
	var fields []string
	var current strings.Builder
	inQuotes := false

	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch {
		case ch == '"':
			inQuotes = !inQuotes
		case ch == ',' && !inQuotes:
			fields = append(fields, current.String())
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}
	fields = append(fields, current.String())
	return fields
}
