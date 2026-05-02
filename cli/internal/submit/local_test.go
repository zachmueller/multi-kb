package submit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zmueller/multi-kb/internal/git"
)

func TestWriteNote_fileExists(t *testing.T) {
	dir := t.TempDir()
	if err := git.InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	uid, err := WriteNote(dir, NoteFields{
		Title:   "Test Note",
		Content: "Some interesting content here.",
		Author:  "tester",
	})
	if err != nil {
		t.Fatalf("WriteNote: %v", err)
	}
	if uid == "" {
		t.Fatal("WriteNote returned empty UID")
	}

	path := filepath.Join(dir, uid+".md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("note file does not exist at %s: %v", path, err)
	}
}

func TestWriteNote_frontmatter(t *testing.T) {
	dir := t.TempDir()
	if err := git.InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	uid, err := WriteNote(dir, NoteFields{
		Title:   "Frontmatter Check",
		Content: "Body text for testing.",
		Author:  "alice",
	})
	if err != nil {
		t.Fatalf("WriteNote: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, uid+".md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	// Verify frontmatter fields
	checks := []struct {
		label   string
		substr  string
	}{
		{"uid", "uid: " + uid},
		{"title", "title: Frontmatter Check"},
		{"status", "status: pending"},
		{"author", "author: alice"},
		{"last-linked-to null", "last-linked-to:"},
		{"last-recalled null", "last-recalled:"},
		{"consolidated-from-notes null", "consolidated-from-notes:"},
	}
	for _, c := range checks {
		if !strings.Contains(content, c.substr) {
			t.Errorf("frontmatter missing %s (expected %q)\nfull content:\n%s", c.label, c.substr, content)
		}
	}

	// Verify frontmatter delimiters
	if !strings.HasPrefix(content, "---\n") {
		t.Error("file does not start with ---")
	}
}

func TestWriteNote_gitCommit(t *testing.T) {
	dir := t.TempDir()
	if err := git.InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	_, err := WriteNote(dir, NoteFields{
		Title:   "Commit Test",
		Content: "Content for commit verification.",
		Author:  "bob",
	})
	if err != nil {
		t.Fatalf("WriteNote: %v", err)
	}

	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(string(out), "add note:") {
		t.Errorf("git log does not contain 'add note:' commit message:\n%s", out)
	}
}

func TestWriteNote_titleTooLong(t *testing.T) {
	dir := t.TempDir()
	if err := git.InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	longTitle := strings.Repeat("x", 256)
	_, err := WriteNote(dir, NoteFields{
		Title:   longTitle,
		Content: "Some content.",
		Author:  "tester",
	})
	if err == nil {
		t.Fatal("WriteNote with 256-char title should return error")
	}
}

func TestWriteNote_contentTooLong(t *testing.T) {
	dir := t.TempDir()
	if err := git.InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	longContent := strings.Repeat("x", 100_001)
	_, err := WriteNote(dir, NoteFields{
		Title:   "Valid Title",
		Content: longContent,
		Author:  "tester",
	})
	if err == nil {
		t.Fatal("WriteNote with 100001-char content should return error")
	}
}

func TestWriteNote_emptyTitle(t *testing.T) {
	dir := t.TempDir()
	if err := git.InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	_, err := WriteNote(dir, NoteFields{
		Title:   "",
		Content: "Some content.",
		Author:  "tester",
	})
	if err == nil {
		t.Fatal("WriteNote with empty title should return error")
	}
}
