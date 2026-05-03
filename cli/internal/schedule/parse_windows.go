//go:build windows

package schedule

import (
	"bytes"
	"fmt"
	"os/exec"
	"time"
)

// ParseNextRun queries schtasks for the multi-kb scheduled task and parses the
// next run time. Returns nil if the task is not installed.
func ParseNextRun() (*time.Time, error) {
	cmd := exec.Command("schtasks.exe", "/Query", "/TN", taskName, "/FO", "CSV", "/NH")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Task doesn't exist — return nil, not an error.
		return nil, nil
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, nil
	}

	// CSV format (no header): "TaskName","Next Run Time","Status"
	// Example: "multi-kb-run","5/3/2026 2:30:00 PM","Ready"
	fields := parseCSVLine(output)
	if len(fields) < 2 {
		return nil, fmt.Errorf("schedule: unexpected schtasks output: %s", output)
	}

	nextRunStr := fields[1]
	if nextRunStr == "N/A" || nextRunStr == "" {
		return nil, nil
	}

	// Try common Windows date/time formats.
	formats := []string{
		"1/2/2006 3:04:05 PM",
		"1/2/2006 15:04:05",
		"2006-01-02 15:04:05",
		"01/02/2006 3:04:05 PM",
		"01/02/2006 15:04:05",
	}

	for _, format := range formats {
		t, err := time.ParseInLocation(format, nextRunStr, time.Local)
		if err == nil {
			return &t, nil
		}
	}

	return nil, fmt.Errorf("schedule: cannot parse next run time %q", nextRunStr)
}

