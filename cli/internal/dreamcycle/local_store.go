package dreamcycle

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// localNoteStore implements NoteStore for local KB git repos.
type localNoteStore struct {
	kbDir   string
	staged  []string
	deleted []string
}

func (s *localNoteStore) ReadNote(uid string) (*Note, error) {
	path := filepath.Join(s.kbDir, uid+".md")
	return readNote(path)
}

func (s *localNoteStore) WriteNote(note Note) error {
	filename := note.UID + ".md"
	path := filepath.Join(s.kbDir, filename)

	body := renderNoteFile(note)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return fmt.Errorf("notestore: write %q: %w", filename, err)
	}

	s.staged = append(s.staged, filename)
	return nil
}

func (s *localNoteStore) DeleteNote(uid string) error {
	filename := uid + ".md"
	path := filepath.Join(s.kbDir, filename)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("notestore: delete %q: %w", filename, err)
	}
	s.deleted = append(s.deleted, filename)
	return nil
}

func (s *localNoteStore) CommitBatch(message string) error {
	// Stage new/modified files
	if len(s.staged) > 0 {
		args := append([]string{"add", "--"}, s.staged...)
		if err := gitRun(s.kbDir, "git", args...); err != nil {
			return err
		}
	}

	// Remove deleted files from index
	for _, f := range s.deleted {
		_ = gitRun(s.kbDir, "git", "rm", "-f", "--", f)
	}

	err := gitRun(s.kbDir, "git", "-c", "user.email=multi-kb@local", "-c", "user.name=multi-kb",
		"commit", "--allow-empty", "-m", message)

	s.staged = nil
	s.deleted = nil
	return err
}

func gitRun(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w\n%s", name, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func renderNoteFile(note Note) string {
	now := time.Now().UTC().Format(time.RFC3339)
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("uid: %s\n", note.UID))
	sb.WriteString(fmt.Sprintf("title: %q\n", note.Title))
	sb.WriteString(fmt.Sprintf("status: %s\n", note.Status))
	sb.WriteString(fmt.Sprintf("author: %s\n", note.Author))
	sb.WriteString(fmt.Sprintf("last-updated: %s\n", now))
	sb.WriteString("last-linked-to:\n")
	sb.WriteString("last-recalled:\n")
	sb.WriteString("consolidated-from-notes:\n")
	sb.WriteString("---\n\n")
	sb.WriteString(note.Content)
	return sb.String()
}

func renderNoteFileWithConsolidated(note Note, consolidatedFrom []string) string {
	now := time.Now().UTC().Format(time.RFC3339)
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("uid: %s\n", note.UID))
	sb.WriteString(fmt.Sprintf("title: %q\n", note.Title))
	sb.WriteString(fmt.Sprintf("status: %s\n", note.Status))
	sb.WriteString(fmt.Sprintf("author: %s\n", note.Author))
	sb.WriteString(fmt.Sprintf("last-updated: %s\n", now))
	sb.WriteString("last-linked-to:\n")
	sb.WriteString("last-recalled:\n")
	if len(consolidatedFrom) > 0 {
		sb.WriteString("consolidated-from-notes:\n")
		for _, uid := range consolidatedFrom {
			sb.WriteString(fmt.Sprintf("  - %s\n", uid))
		}
	} else {
		sb.WriteString("consolidated-from-notes:\n")
	}
	sb.WriteString("---\n\n")
	sb.WriteString(note.Content)
	return sb.String()
}
