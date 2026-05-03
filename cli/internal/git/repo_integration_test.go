//go:build integration

package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitOperations_RealGit(t *testing.T) {
	dir := t.TempDir()

	// Init repo
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// Verify .git directory exists
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("expected .git directory: %v", err)
	}

	// Write a test file and commit
	testFile := filepath.Join(dir, "TEST001.md")
	content := "---\nuid: TEST001\ntitle: Test Note\nstatus: active\nauthor: tester\n---\n\nTest content about AWS configuration."
	if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// Commit
	if err := CommitFiles(dir, []string{"TEST001.md"}, "test: add test note"); err != nil {
		t.Fatalf("CommitFiles: %v", err)
	}

	// Grep for content
	results, err := GrepNotes(dir, []string{"AWS", "configuration"})
	if err != nil {
		t.Fatalf("GrepNotes: %v", err)
	}

	if len(results) == 0 {
		t.Error("expected at least 1 grep result")
	}

	found := false
	for _, r := range results {
		if r.UID == "TEST001" {
			found = true
			if r.MatchCount < 1 {
				t.Errorf("expected MatchCount >= 1, got %d", r.MatchCount)
			}
		}
	}
	if !found {
		t.Error("expected to find TEST001 in grep results")
	}
}
