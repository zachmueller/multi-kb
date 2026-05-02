package lock

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAcquireFresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	l, err := Acquire(path, "test-activity")
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	t.Cleanup(func() { _ = l.Release() })

	// Verify lock file exists with correct JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("lock file not created: %v", err)
	}

	var lf lockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		t.Fatalf("lock file is not valid JSON: %v", err)
	}

	if lf.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", lf.PID, os.Getpid())
	}
	if lf.Activity != "test-activity" {
		t.Errorf("Activity = %q, want %q", lf.Activity, "test-activity")
	}
	if lf.StartedAt == "" {
		t.Error("StartedAt is empty")
	}
	if lf.Heartbeat == "" {
		t.Error("Heartbeat is empty")
	}

	// Verify timestamps are valid RFC3339
	if _, err := time.Parse(time.RFC3339, lf.StartedAt); err != nil {
		t.Errorf("StartedAt is not RFC3339: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, lf.Heartbeat); err != nil {
		t.Errorf("Heartbeat is not RFC3339: %v", err)
	}
}

func TestAcquireStale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	// Write a stale lock file with heartbeat >30 min old
	stale := lockFile{
		PID:       99999,
		StartedAt: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
		Heartbeat: time.Now().Add(-31 * time.Minute).UTC().Format(time.RFC3339),
		Activity:  "stale-activity",
	}
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatalf("marshal stale lock: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}

	// Acquire should succeed by overwriting the stale lock
	l, err := Acquire(path, "new-activity")
	if err != nil {
		t.Fatalf("Acquire on stale lock failed: %v", err)
	}
	t.Cleanup(func() { _ = l.Release() })

	// Verify the new lock replaced the stale one
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read lock file: %v", err)
	}
	var lf lockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		t.Fatalf("unmarshal lock file: %v", err)
	}
	if lf.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d (current process)", lf.PID, os.Getpid())
	}
	if lf.Activity != "new-activity" {
		t.Errorf("Activity = %q, want %q", lf.Activity, "new-activity")
	}
}

func TestFailOnActive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	l1, err := Acquire(path, "first-activity")
	if err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}
	t.Cleanup(func() { _ = l1.Release() })

	// Second acquire should fail
	l2, err := Acquire(path, "second-activity")
	if l2 != nil {
		_ = l2.Release()
		t.Fatal("second Acquire should have returned nil lock")
	}
	if err == nil {
		t.Fatal("second Acquire should have returned an error")
	}

	// Error should wrap ErrLockHeld
	if !errors.Is(err, ErrLockHeld) {
		t.Errorf("error should wrap ErrLockHeld, got: %v", err)
	}

	// Error message should mention pid and activity
	errMsg := err.Error()
	if !strings.Contains(errMsg, "pid=") {
		t.Errorf("error should mention pid, got: %v", errMsg)
	}
	if !strings.Contains(errMsg, "first-activity") {
		t.Errorf("error should mention activity, got: %v", errMsg)
	}
}

func TestRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	l, err := Acquire(path, "release-test")
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	// Verify lock file exists before release
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file should exist before release: %v", err)
	}

	if err := l.Release(); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Lock file should be deleted
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("lock file should be deleted after release")
	}

	// Double release should be safe (idempotent)
	if err := l.Release(); err != nil {
		t.Errorf("double Release should not error, got: %v", err)
	}
}

func TestHeartbeatAndLockFileValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	l, err := Acquire(path, "heartbeat-test")
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	t.Cleanup(func() { _ = l.Release() })

	// Verify the stopCh is open (heartbeat goroutine is running)
	select {
	case <-l.stopCh:
		t.Error("stopCh should be open while lock is held (heartbeat goroutine running)")
	default:
		// good — channel is open, goroutine is running
	}

	// Verify lock file exists and contains valid JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read lock file: %v", err)
	}
	var lf lockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		t.Fatalf("lock file is not valid JSON: %v", err)
	}
	if lf.Activity != "heartbeat-test" {
		t.Errorf("Activity = %q, want %q", lf.Activity, "heartbeat-test")
	}

	// Verify heartbeat timestamp is recent (within a few seconds)
	hb, err := time.Parse(time.RFC3339, lf.Heartbeat)
	if err != nil {
		t.Fatalf("parse heartbeat: %v", err)
	}
	if time.Since(hb) > 5*time.Second {
		t.Errorf("heartbeat is too old: %v", hb)
	}

	// After release, stopCh should be closed
	_ = l.Release()
	select {
	case <-l.stopCh:
		// good — channel is closed after release
	default:
		t.Error("stopCh should be closed after release")
	}
}
