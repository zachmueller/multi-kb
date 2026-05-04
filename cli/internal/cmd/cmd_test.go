package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/extract"
	"github.com/zmueller/multi-kb/internal/git"
	"github.com/zmueller/multi-kb/internal/lock"
	"github.com/zmueller/multi-kb/internal/route"
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

// --- execApprove tests ---

func TestExecApprove_NoPendingNotes(t *testing.T) {
	pendingDir := t.TempDir() // empty dir — no pending notes
	cfgPath := writeTempCmdConfig(t)

	var out strings.Builder
	err := execApprove(cfgPath, pendingDir,
		func(d string, c *config.Config) error {
			t.Error("startServer should not be called when no pending notes")
			return nil
		},
		&out, &out)

	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if !contains(out.String(), "No notes awaiting approval") {
		t.Errorf("expected no-pending message, got: %q", out.String())
	}
}

func TestExecApprove_WithPendingNotes(t *testing.T) {
	pendingDir := t.TempDir()
	cfgPath := writeTempCmdConfig(t)

	// Write a pending note JSON directly.
	noteData := `{"title":"T","content":"C","author":"a","target_kbs":["local/dev"],"source_conversation":"","extracted_at":""}`
	if err := os.WriteFile(pendingDir+"/20260101T000000-aabbccdd.json", []byte(noteData), 0o600); err != nil {
		t.Fatalf("write pending note: %v", err)
	}

	var out strings.Builder
	serverCalled := false
	err := execApprove(cfgPath, pendingDir,
		func(d string, c *config.Config) error {
			serverCalled = true
			return nil
		},
		&out, &out)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !serverCalled {
		t.Error("expected startServer to be called when pending notes exist")
	}
	if !contains(out.String(), "pending note") {
		t.Errorf("expected pending-note message, got: %q", out.String())
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

// --- submitNote local KB auto-create tests ---

func TestSubmitNote_LocalKB_CreatesDirectory(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	target := route.ResolvedTarget{
		KB:           "local/fresh-kb",
		ApprovalMode: "auto-approve",
	}
	note := extract.Note{
		Title:   "Test Note",
		Content: "Knowledge worth keeping.",
	}
	cfg := &config.Config{Author: "tester"}

	err := submitNote(context.Background(), cfg, target, note, nil, nil)
	if err != nil {
		t.Fatalf("submitNote failed: %v", err)
	}

	kbDir := filepath.Join(fakeHome, ".multi-kb", "local", "fresh-kb")
	if !git.IsRepo(kbDir) {
		t.Fatalf("expected %s to be a git repo", kbDir)
	}

	entries, err := os.ReadDir(kbDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var mdFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") && e.Name() != ".gitkeep" {
			mdFiles = append(mdFiles, e.Name())
		}
	}
	if len(mdFiles) != 1 {
		t.Fatalf("expected 1 .md note file, got %d: %v", len(mdFiles), mdFiles)
	}
}

func TestSubmitNote_LocalKB_Idempotent(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	kbDir := filepath.Join(fakeHome, ".multi-kb", "local", "existing-kb")
	if err := git.InitRepo(kbDir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	target := route.ResolvedTarget{
		KB:           "local/existing-kb",
		ApprovalMode: "auto-approve",
	}
	note := extract.Note{
		Title:   "Second Note",
		Content: "More knowledge.",
	}
	cfg := &config.Config{Author: "tester"}

	err := submitNote(context.Background(), cfg, target, note, nil, nil)
	if err != nil {
		t.Fatalf("submitNote on existing repo failed: %v", err)
	}
}

func TestSubmitNote_LocalKB_NoteContent(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	target := route.ResolvedTarget{
		KB:           "local/verify-kb",
		ApprovalMode: "auto-approve",
	}
	note := extract.Note{
		Title:   "Verifiable Note",
		Content: "Content to verify in frontmatter.",
	}
	cfg := &config.Config{Author: "alice"}

	err := submitNote(context.Background(), cfg, target, note, nil, nil)
	if err != nil {
		t.Fatalf("submitNote failed: %v", err)
	}

	kbDir := filepath.Join(fakeHome, ".multi-kb", "local", "verify-kb")
	entries, _ := os.ReadDir(kbDir)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".md") || e.Name() == ".gitkeep" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(kbDir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		content := string(data)
		for _, want := range []string{`title: "Verifiable Note"`, "author: alice", "Content to verify in frontmatter."} {
			if !strings.Contains(content, want) {
				t.Errorf("note missing %q\nfull:\n%s", want, content)
			}
		}
		return
	}
	t.Fatal("no .md note file found")
}
