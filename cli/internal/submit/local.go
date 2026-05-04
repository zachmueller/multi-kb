package submit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zmueller/multi-kb/internal/git"
)

// NoteFields contains the content for a local KB note.
type NoteFields struct {
	Title   string
	Content string
	Author  string
}

// WriteNote generates a UID, writes the note as a Markdown file with YAML
// frontmatter, and commits it to the local KB git repo.
func WriteNote(kbDir string, fields NoteFields) (string, error) {
	if err := validateNote(fields); err != nil {
		return "", err
	}

	uid, err := GenerateUID()
	if err != nil {
		return "", fmt.Errorf("submit: cannot generate UID: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	filename := uid + ".md"
	path := filepath.Join(kbDir, filename)

	body := renderNote(uid, fields, now)

	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return "", fmt.Errorf("submit: cannot write note file: %w", err)
	}

	commitMsg := fmt.Sprintf("add note: %s", sanitizeForCommitMsg(fields.Title))
	if err := git.CommitFiles(kbDir, []string{filename}, commitMsg); err != nil {
		return "", fmt.Errorf("submit: cannot commit note: %w", err)
	}

	return uid, nil
}

func renderNote(uid string, fields NoteFields, now string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("uid: %s\n", uid))
	sb.WriteString(fmt.Sprintf("title: %q\n", fields.Title))
	sb.WriteString("status: pending\n")
	sb.WriteString(fmt.Sprintf("author: %s\n", fields.Author))
	sb.WriteString(fmt.Sprintf("last-updated: %s\n", now))
	// YAML null convention: key present with no value
	sb.WriteString("last-linked-to:\n")
	sb.WriteString("last-recalled:\n")
	sb.WriteString("consolidated-from-notes:\n")
	sb.WriteString("---\n\n")
	sb.WriteString(fields.Content)
	return sb.String()
}

func validateNote(fields NoteFields) error {
	if strings.TrimSpace(fields.Title) == "" {
		return fmt.Errorf("submit: title must be non-empty")
	}
	if len(fields.Title) > 255 {
		return fmt.Errorf("submit: title must be ≤255 characters")
	}
	if strings.TrimSpace(fields.Content) == "" {
		return fmt.Errorf("submit: content must be non-empty")
	}
	if len(fields.Content) > 100_000 {
		return fmt.Errorf("submit: content must be ≤100,000 characters")
	}
	if strings.TrimSpace(fields.Author) == "" {
		return fmt.Errorf("submit: author must be non-empty")
	}
	if len(fields.Author) > 100 {
		return fmt.Errorf("submit: author must be ≤100 characters")
	}
	return nil
}

func sanitizeForCommitMsg(s string) string {
	// Remove newlines and truncate for safety
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) > 72 {
		s = s[:72]
	}
	return s
}
