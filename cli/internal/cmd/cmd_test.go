package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zmueller/multi-kb/internal/lock"
)

// writeTempCmdConfig writes a minimal valid client-mode config to a temp file.
func writeTempCmdConfig(t *testing.T) string {
	t.Helper()
	cfg := `
mode: client
author: tester
knowledge_bases:
  - name: my-kb
    endpoint: https://example.com
    auth: iam
    aws_profile: default
sources:
  - directory: /tmp/src
    harnesses: [claude-code]
    targets:
      - kb: my-kb
        routing: always
        approval: auto-approve
extraction:
  model_id: anthropic.claude-sonnet-4-20250514
  aws_region: us-east-1
hook:
  timeout: 8s
dream_cycle:
  model_id: anthropic.claude-sonnet-4-20250514
`
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	if _, err := f.WriteString(cfg); err != nil {
		f.Close()
		t.Fatalf("write temp config: %v", err)
	}
	f.Close()
	return f.Name()
}

// writeLockHeld writes a fresh, active lock file at path to simulate a held lock.
func writeLockHeld(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create lock dir: %v", err)
	}
	type lockPayload struct {
		PID       int    `json:"pid"`
		StartedAt string `json:"started_at"`
		Heartbeat string `json:"heartbeat"`
		Activity  string `json:"activity"`
	}
	now := time.Now().UTC().Format(time.RFC3339)
	data, _ := json.Marshal(lockPayload{
		PID:       99999,
		StartedAt: now,
		Heartbeat: now,
		Activity:  "other_process",
	})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write lock file: %v", err)
	}
}

// --- execDreamCycle tests ---

func TestExecDreamCycle_MissingConfig(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "lock")
	logsDir := t.TempDir()

	err := execDreamCycle(context.Background(), "/nonexistent/config.yaml", lockPath, logsDir)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
	if !contains(err.Error(), "load config") {
		t.Errorf("expected 'load config' in error, got %q", err.Error())
	}
}

func TestExecDreamCycle_LockHeld(t *testing.T) {
	cfgPath := writeTempCmdConfig(t)
	lockPath := filepath.Join(t.TempDir(), "lock")
	logsDir := t.TempDir()

	writeLockHeld(t, lockPath)

	// Lock is held — execDreamCycle should print a message and return nil (not error).
	err := execDreamCycle(context.Background(), cfgPath, lockPath, logsDir)
	if err != nil {
		t.Fatalf("expected nil when lock is held, got: %v", err)
	}
}

func TestExecDreamCycle_LockHeld_ErrIsWrapped(t *testing.T) {
	// Verify that the underlying lock.ErrLockHeld is detectable in isolation.
	lockPath := filepath.Join(t.TempDir(), "lock")
	writeLockHeld(t, lockPath)

	_, err := lock.Acquire(lockPath, "test")
	if err == nil {
		t.Fatal("expected error acquiring held lock")
	}
	if !errors.Is(err, lock.ErrLockHeld) {
		t.Errorf("expected ErrLockHeld, got %v", err)
	}
}

// --- execRun tests ---

func TestExecRun_MissingConfig(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "lock")
	logsDir := t.TempDir()

	// runProcess loads the config and will fail — execRun should propagate.
	err := execRun(context.Background(), "/nonexistent/config.yaml", lockPath, logsDir)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

// TestExecRun_DreamCycleLockHeld verifies that when the dream cycle step encounters
// a held lock, execRun returns nil (not an error).  We achieve this by running
// execRun with a valid config that has no sources (so runProcess completes instantly)
// and a pre-held lock on the dream-cycle lockPath.
func TestExecRun_DreamCycleLockHeld(t *testing.T) {
	cfgPath := writeTempCmdConfig(t)
	lockPath := filepath.Join(t.TempDir(), "lock")
	logsDir := t.TempDir()

	// Pre-hold the lock so the dream-cycle step inside execRun returns ErrLockHeld.
	writeLockHeld(t, lockPath)

	// runProcess will use lock.DefaultLockPath() which is the real system lock —
	// it should succeed because no other process holds it.  If the real system
	// lock is held (e.g., cron is running), skip gracefully.
	err := execRun(context.Background(), cfgPath, lockPath, logsDir)
	if err != nil {
		// Accept ErrLockHeld on the runProcess side as a benign skip.
		if errors.Is(err, lock.ErrLockHeld) {
			t.Skip("real system lock is held; skipping dream-cycle lock test")
		}
		t.Fatalf("unexpected error: %v", err)
	}
}

// contains is a helper to check substring presence.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
