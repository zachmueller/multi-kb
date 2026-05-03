//go:build windows

package schedule

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const marker = "# multi-kb scheduled run"
const taskName = "multi-kb-run"

// Scheduler manages scheduled task registration for multi-kb.
type Scheduler interface {
	Install(cronExpr, binaryPath, configPath string) error
	Uninstall() error
	IsInstalled() (bool, error)
}

type windowsScheduler struct{}

// NewScheduler returns a Scheduler implementation for Windows.
func NewScheduler() Scheduler {
	return &windowsScheduler{}
}

// Install creates a Windows scheduled task for multi-kb. The cronExpr is
// expected to be a standard 5-field cron expression; we extract the interval
// in minutes from a */N minute field for the MINUTE schedule type.
func (s *windowsScheduler) Install(cronExpr, binaryPath, configPath string) error {
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

	interval, err := extractMinuteInterval(cronExpr)
	if err != nil {
		return fmt.Errorf("schedule: parse cron expression for Windows: %w", err)
	}

	tr := fmt.Sprintf(`cmd /c "%s run --config %s >> %s 2>&1"`, absPath, absConfigPath, logPath)

	cmd := exec.Command("schtasks.exe",
		"/Create",
		"/SC", "MINUTE",
		"/MO", strconv.Itoa(interval),
		"/TN", taskName,
		"/TR", tr,
		"/F",
		"/NP",
		"/RL", "LIMITED",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("schtasks /Create: %w: %s", err, stderr.String())
	}
	return nil
}

// Uninstall removes the multi-kb Windows scheduled task.
func (s *windowsScheduler) Uninstall() error {
	cmd := exec.Command("schtasks.exe", "/Delete", "/TN", taskName, "/F")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("schtasks /Delete: %w: %s", err, stderr.String())
	}
	return nil
}

// IsInstalled checks whether the multi-kb scheduled task exists.
func (s *windowsScheduler) IsInstalled() (bool, error) {
	cmd := exec.Command("schtasks.exe", "/Query", "/TN", taskName)
	err := cmd.Run()
	if err != nil {
		// Non-zero exit code means the task does not exist.
		return false, nil
	}
	return true, nil
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
