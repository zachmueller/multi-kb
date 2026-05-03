//go:build unix

package schedule

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// newTestScheduler builds a scheduler whose crontab is backed by an in-memory
// slice so no real crontab process is invoked.
func newTestScheduler(initial []string) (*unixScheduler, *[]string, *bool) {
	lines := append([]string(nil), initial...)
	removed := false

	s := &unixScheduler{
		readFn: func() ([]string, error) {
			return append([]string(nil), lines...), nil
		},
		writeFn: func(l []string) error {
			lines = append([]string(nil), l...)
			return nil
		},
		removeFn: func() error {
			lines = nil
			removed = true
			return nil
		},
		logPathFn: func() (string, error) {
			return "/tmp/test-cron.log", nil
		},
	}
	return s, &lines, &removed
}

// fakeBinary creates a temp executable so resolveAbsPath doesn't fail.
func fakeBinary(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "multi-kb-*")
	if err != nil {
		t.Fatalf("create fake binary: %v", err)
	}
	f.Close()
	os.Chmod(f.Name(), 0o755) //nolint
	return f.Name()
}

func TestUnixScheduler_Install_Fresh(t *testing.T) {
	s, lines, _ := newTestScheduler(nil)
	bin := fakeBinary(t)

	if err := s.Install("*/30 * * * *", bin, "/home/user/.multi-kb/config.yaml"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(*lines) != 1 {
		t.Fatalf("expected 1 crontab line, got %d: %v", len(*lines), *lines)
	}
	line := (*lines)[0]
	if !strings.Contains(line, "*/30 * * * *") {
		t.Errorf("line missing cron expression: %q", line)
	}
	if !strings.Contains(line, marker) {
		t.Errorf("line missing marker: %q", line)
	}
	if !strings.Contains(line, bin) {
		t.Errorf("line missing binary path: %q", line)
	}
}

func TestUnixScheduler_Install_Idempotent(t *testing.T) {
	s, lines, _ := newTestScheduler(nil)
	bin := fakeBinary(t)

	if err := s.Install("*/30 * * * *", bin, "/home/user/.multi-kb/config.yaml"); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	if err := s.Install("0 * * * *", bin, "/home/user/.multi-kb/config.yaml"); err != nil {
		t.Fatalf("second Install: %v", err)
	}

	// Only one multi-kb line should exist after re-install.
	count := 0
	for _, l := range *lines {
		if strings.Contains(l, marker) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 multi-kb line after re-install, got %d", count)
	}
	// Should contain updated expression.
	if !strings.Contains((*lines)[len(*lines)-1], "0 * * * *") {
		t.Errorf("expected updated cron expression in line: %q", (*lines)[len(*lines)-1])
	}
}

func TestUnixScheduler_Install_PreservesOtherEntries(t *testing.T) {
	existing := []string{
		"0 2 * * * /usr/bin/backup",
		"30 6 * * 1 /usr/bin/weekly",
	}
	s, lines, _ := newTestScheduler(existing)
	bin := fakeBinary(t)

	if err := s.Install("*/15 * * * *", bin, "/home/user/.multi-kb/config.yaml"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(*lines) != 3 {
		t.Errorf("expected 3 lines (2 preserved + 1 new), got %d: %v", len(*lines), *lines)
	}
}

func TestUnixScheduler_IsInstalled_True(t *testing.T) {
	initial := []string{
		"*/15 * * * * /usr/local/bin/multi-kb run --config /cfg.yaml >> /tmp/cron.log 2>&1 " + marker,
	}
	s, _, _ := newTestScheduler(initial)

	ok, err := s.IsInstalled()
	if err != nil {
		t.Fatalf("IsInstalled: %v", err)
	}
	if !ok {
		t.Error("expected IsInstalled=true")
	}
}

func TestUnixScheduler_IsInstalled_False(t *testing.T) {
	initial := []string{"0 2 * * * /usr/bin/backup"}
	s, _, _ := newTestScheduler(initial)

	ok, err := s.IsInstalled()
	if err != nil {
		t.Fatalf("IsInstalled: %v", err)
	}
	if ok {
		t.Error("expected IsInstalled=false when no multi-kb entry")
	}
}

func TestUnixScheduler_Uninstall_RemovesEntry(t *testing.T) {
	initial := []string{
		"0 2 * * * /usr/bin/backup",
		"*/15 * * * * /usr/local/bin/multi-kb run --config /cfg.yaml >> /tmp/cron.log 2>&1 " + marker,
	}
	s, lines, _ := newTestScheduler(initial)

	if err := s.Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	for _, l := range *lines {
		if strings.Contains(l, marker) {
			t.Errorf("expected multi-kb line to be removed, still present: %q", l)
		}
	}
	// Other entry preserved.
	if len(*lines) != 1 || !strings.Contains((*lines)[0], "/usr/bin/backup") {
		t.Errorf("expected backup entry preserved, got: %v", *lines)
	}
}

func TestUnixScheduler_Uninstall_EmptyCrontab(t *testing.T) {
	// Only the multi-kb entry — after uninstall the crontab should be deleted.
	initial := []string{
		"*/15 * * * * /usr/local/bin/multi-kb run --config /cfg.yaml >> /tmp/cron.log 2>&1 " + marker,
	}
	s, _, removed := newTestScheduler(initial)

	if err := s.Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !*removed {
		t.Error("expected removeFn to be called when crontab becomes empty")
	}
}

func TestUnixScheduler_Uninstall_EmptyInput(t *testing.T) {
	s, _, removed := newTestScheduler(nil)

	if err := s.Uninstall(); err != nil {
		t.Fatalf("Uninstall on empty crontab: %v", err)
	}
	if !*removed {
		t.Error("expected removeFn called for empty crontab")
	}
}

func TestUnixScheduler_Install_ReadError(t *testing.T) {
	s := &unixScheduler{
		readFn: func() ([]string, error) {
			return nil, errors.New("read failure")
		},
		writeFn: func([]string) error { return nil },
		removeFn: func() error { return nil },
		logPathFn: func() (string, error) { return "/tmp/cron.log", nil },
	}

	err := s.Install("*/30 * * * *", "/usr/local/bin/multi-kb", "/cfg.yaml")
	if err == nil {
		t.Fatal("expected error on read failure")
	}
}

func TestUnixScheduler_Install_WriteError(t *testing.T) {
	s := &unixScheduler{
		readFn:    func() ([]string, error) { return nil, nil },
		writeFn:   func([]string) error { return fmt.Errorf("write failure") },
		removeFn:  func() error { return nil },
		logPathFn: func() (string, error) { return "/tmp/cron.log", nil },
	}

	err := s.Install("*/30 * * * *", "/usr/local/bin/multi-kb", "/cfg.yaml")
	if err == nil {
		t.Fatal("expected error on write failure")
	}
}
