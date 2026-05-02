//go:build unix

package schedule

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const marker = "# multi-kb scheduled run"

// Scheduler manages cron-based scheduling for multi-kb.
type Scheduler interface {
	Install(cronExpr, binaryPath, configPath string) error
	Uninstall() error
	IsInstalled() (bool, error)
}

type unixScheduler struct{}

// NewScheduler returns a Scheduler implementation for the current platform.
func NewScheduler() Scheduler {
	return &unixScheduler{}
}

// Install adds a crontab entry for multi-kb. It is idempotent: any existing
// multi-kb entry is replaced.
func (s *unixScheduler) Install(cronExpr, binaryPath, configPath string) error {
	absPath, err := resolveAbsPath(binaryPath)
	if err != nil {
		return fmt.Errorf("schedule: resolve binary path: %w", err)
	}

	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("schedule: resolve config path: %w", err)
	}

	logPath, err := absLogPath()
	if err != nil {
		return fmt.Errorf("schedule: resolve log path: %w", err)
	}

	// Ensure log directory exists.
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return fmt.Errorf("schedule: create log directory: %w", err)
	}

	existing, err := readCrontab()
	if err != nil {
		return fmt.Errorf("schedule: read crontab: %w", err)
	}

	// Filter out any existing multi-kb line.
	filtered := filterMarkerLines(existing)

	// Build the new cron entry.
	entry := fmt.Sprintf("%s %s run --config %s >> %s 2>&1 %s",
		cronExpr, absPath, absConfigPath, logPath, marker)

	filtered = append(filtered, entry)

	return writeCrontab(filtered)
}

// Uninstall removes the multi-kb crontab entry. If the crontab becomes empty
// after removal, it is deleted entirely via `crontab -r`.
func (s *unixScheduler) Uninstall() error {
	existing, err := readCrontab()
	if err != nil {
		return fmt.Errorf("schedule: read crontab: %w", err)
	}

	filtered := filterMarkerLines(existing)

	// If nothing remains (or only blank lines), remove the crontab entirely.
	hasContent := false
	for _, line := range filtered {
		if strings.TrimSpace(line) != "" {
			hasContent = true
			break
		}
	}

	if !hasContent {
		cmd := exec.Command("crontab", "-r")
		// crontab -r may fail if there is no crontab; that is fine.
		_ = cmd.Run()
		return nil
	}

	return writeCrontab(filtered)
}

// IsInstalled checks whether a multi-kb cron entry exists.
func (s *unixScheduler) IsInstalled() (bool, error) {
	existing, err := readCrontab()
	if err != nil {
		return false, fmt.Errorf("schedule: read crontab: %w", err)
	}

	for _, line := range existing {
		if strings.Contains(line, marker) {
			return true, nil
		}
	}
	return false, nil
}

// readCrontab reads the current user crontab. An empty crontab (exit code 1)
// is treated as empty, not as an error.
func readCrontab() ([]string, error) {
	cmd := exec.Command("crontab", "-l")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			// No crontab for this user — treat as empty.
			return nil, nil
		}
		return nil, fmt.Errorf("crontab -l: %w: %s", err, stderr.String())
	}

	output := stdout.String()
	if output == "" {
		return nil, nil
	}
	return strings.Split(strings.TrimRight(output, "\n"), "\n"), nil
}

// writeCrontab writes lines back to the user's crontab via `crontab -` (stdin).
func writeCrontab(lines []string) error {
	content := strings.Join(lines, "\n") + "\n"

	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("crontab -: %w: %s", err, stderr.String())
	}
	return nil
}

// filterMarkerLines returns all lines that do NOT contain the multi-kb marker.
func filterMarkerLines(lines []string) []string {
	var result []string
	for _, line := range lines {
		if !strings.Contains(line, marker) {
			result = append(result, line)
		}
	}
	return result
}

// resolveAbsPath resolves a binary path to an absolute path, following symlinks.
func resolveAbsPath(binaryPath string) (string, error) {
	if binaryPath != "" {
		return filepath.EvalSymlinks(binaryPath)
	}
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exePath)
}

// absLogPath returns the absolute path to the cron log file.
func absLogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".multi-kb", "logs", "cron.log"), nil
}
