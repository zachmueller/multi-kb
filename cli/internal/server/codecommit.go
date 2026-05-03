package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zmueller/multi-kb/internal/config"
)

const repoDir = "/opt/multi-kb/repo"

// CommitBatch writes note files to the CodeCommit repo and commits them.
func CommitBatch(ctx context.Context, cfg *config.Config, notes []NoteFile) error {
	dir := repoDir

	for _, note := range notes {
		path := filepath.Join(dir, note.Filename())
		if err := os.WriteFile(path, []byte(note.ToMarkdown()), 0o600); err != nil {
			return fmt.Errorf("codecommit: write %s: %w", note.Filename(), err)
		}
	}

	// Stage all new/modified files
	filenames := make([]string, len(notes))
	for i, note := range notes {
		filenames[i] = note.Filename()
	}

	addArgs := append([]string{"add", "--"}, filenames...)
	if err := gitCmd(ctx, dir, addArgs...); err != nil {
		return fmt.Errorf("codecommit: git add: %w", err)
	}

	// Check if there are staged changes
	out, err := gitOutput(ctx, dir, "diff", "--cached", "--name-only")
	if err != nil {
		return fmt.Errorf("codecommit: git diff --cached: %w", err)
	}
	if strings.TrimSpace(out) == "" {
		slog.Debug("codecommit: no changes to commit")
		return nil
	}

	titles := make([]string, len(notes))
	for i, n := range notes {
		titles[i] = n.Title
	}
	msg := fmt.Sprintf("ingest: %d note(s)\n\n%s", len(notes), strings.Join(titles, "\n"))

	if err := gitCmd(ctx, dir, "commit", "-m", msg); err != nil {
		return fmt.Errorf("codecommit: git commit: %w", err)
	}

	if err := gitCmd(ctx, dir, "push"); err != nil {
		return fmt.Errorf("codecommit: git push: %w", err)
	}

	slog.Info("codecommit: committed and pushed", "notes", len(notes))
	return nil
}

// CloneRepo clones the CodeCommit repository if it doesn't exist locally.
// The CDK user data script handles initial clone; this is a fallback.
func CloneRepo(ctx context.Context, cfg *config.Config) error {
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err == nil {
		return nil
	}

	region := cfg.CodeCommit.Region
	repoName := cfg.CodeCommit.RepoName
	url := fmt.Sprintf("https://git-codecommit.%s.amazonaws.com/v1/repos/%s", region, repoName)

	if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
		return fmt.Errorf("codecommit: create parent dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "clone", url, repoDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// If clone fails (empty repo), init and set remote
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			return err
		}
		if err := gitCmd(ctx, repoDir, "init"); err != nil {
			return err
		}
		if err := gitCmd(ctx, repoDir, "remote", "add", "origin", url); err != nil {
			return err
		}
	}

	return nil
}

func gitCmd(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOME=/root")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args[:1], " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOME=/root")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", args[0], strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}
