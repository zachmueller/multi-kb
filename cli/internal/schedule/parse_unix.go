//go:build unix

package schedule

import (
	"strings"
	"time"
)

// ParseNextRun reads the crontab, finds the multi-kb marker line, extracts the
// cron expression, and computes the next scheduled run time. Returns nil if no
// cron entry is found.
func ParseNextRun() (*time.Time, error) {
	lines, err := readCrontab()
	if err != nil {
		return nil, err
	}

	cronExpr := findCronExpr(lines)
	if cronExpr == "" {
		return nil, nil
	}

	next, err := NextRunAfter(cronExpr, time.Now())
	if err != nil {
		return nil, err
	}
	return &next, nil
}

// findCronExpr searches crontab lines for the multi-kb marker and extracts the
// 5-field cron expression from the matching line.
func findCronExpr(lines []string) string {
	for _, line := range lines {
		if !strings.Contains(line, marker) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		return strings.Join(fields[:5], " ")
	}
	return ""
}
