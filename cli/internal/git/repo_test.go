package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitRepo_freshDir(t *testing.T) {
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo() error: %v", err)
	}
	gitDir := filepath.Join(dir, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		t.Fatalf(".git directory does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal(".git is not a directory")
	}
}

func TestInitRepo_idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("first InitRepo() error: %v", err)
	}
	if err := InitRepo(dir); err != nil {
		t.Fatalf("second InitRepo() error: %v", err)
	}
}

func TestIsRepo_validRepo(t *testing.T) {
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo() error: %v", err)
	}
	if !IsRepo(dir) {
		t.Error("IsRepo() = false, want true for initialized repo")
	}
}

func TestIsRepo_nonRepo(t *testing.T) {
	dir := t.TempDir()
	if IsRepo(dir) {
		t.Error("IsRepo() = true, want false for plain directory")
	}
}

func TestCommitFiles(t *testing.T) {
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo() error: %v", err)
	}

	// Write a file to the repo
	testFile := "hello.txt"
	if err := os.WriteFile(filepath.Join(dir, testFile), []byte("hello world"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if err := CommitFiles(dir, []string{testFile}, "test commit"); err != nil {
		t.Fatalf("CommitFiles() error: %v", err)
	}

	// Verify the commit shows up in git log
	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log error: %v", err)
	}
	if !strings.Contains(string(out), "test commit") {
		t.Errorf("git log output does not contain commit message:\n%s", out)
	}
}

func TestValidateFilename_shellMetacharacters(t *testing.T) {
	dangerous := []string{
		"file;rm -rf /",
		"file|cat",
		"file&background",
		"file$var",
		"file`cmd`",
		"file>out",
		"file<in",
		"file!bang",
		"file\\slash",
		"dir/file",
		"../escape",
	}
	for _, name := range dangerous {
		t.Run(name, func(t *testing.T) {
			err := validateFilename(name)
			if err == nil {
				t.Errorf("validateFilename(%q) = nil, want error", name)
			}
		})
	}
}

func TestValidateFilename_safeNames(t *testing.T) {
	safe := []string{
		"note.md",
		"TEST001.md",
		"my-note_v2.md",
		"ABC123.txt",
	}
	for _, name := range safe {
		t.Run(name, func(t *testing.T) {
			err := validateFilename(name)
			if err != nil {
				t.Errorf("validateFilename(%q) = %v, want nil", name, err)
			}
		})
	}
}
