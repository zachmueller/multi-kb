package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LocalKBDir returns the local KB directory for a named KB.
func LocalKBDir(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".multi-kb", "local", name), nil
}

// InitRepo creates a local KB git repository. Idempotent if already initialized.
func InitRepo(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("git: cannot create directory %q: %w", dir, err)
	}

	if IsRepo(dir) {
		return nil
	}

	if err := run(dir, "git", "init"); err != nil {
		return fmt.Errorf("git: init failed: %w", err)
	}

	// Initial empty commit
	keepFile := filepath.Join(dir, ".gitkeep")
	if err := os.WriteFile(keepFile, nil, 0o600); err != nil {
		return fmt.Errorf("git: cannot create .gitkeep: %w", err)
	}
	if err := run(dir, "git", "add", ".gitkeep"); err != nil {
		return err
	}
	if err := run(dir, "git", "-c", "user.email=multi-kb@local", "-c", "user.name=multi-kb",
		"commit", "-m", "init: create local KB"); err != nil {
		return err
	}

	return nil
}

// IsRepo reports whether dir is an initialized git repository.
func IsRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	return cmd.Run() == nil
}

// CommitFiles stages and commits the given files with the given message.
func CommitFiles(dir string, files []string, message string) error {
	// Sanitize: only allow safe filenames (no shell metacharacters)
	for _, f := range files {
		if err := validateFilename(f); err != nil {
			return err
		}
	}

	args := append([]string{"add", "--"}, files...)
	if err := run(dir, "git", args...); err != nil {
		return fmt.Errorf("git: add failed: %w", err)
	}

	return run(dir, "git", "-c", "user.email=multi-kb@local", "-c", "user.name=multi-kb",
		"commit", "--allow-empty", "-m", message)
}

func run(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w\n%s", name, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func validateFilename(name string) error {
	base := filepath.Base(name)
	if base != name {
		return fmt.Errorf("git: filename must not contain path separators: %q", name)
	}
	// Reject shell metacharacters
	forbidden := "|;&$`><!\\\n\r\t"
	if strings.ContainsAny(name, forbidden) {
		return fmt.Errorf("git: filename contains forbidden characters: %q", name)
	}
	return nil
}
