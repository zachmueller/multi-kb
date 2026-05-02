package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// helper: write a markdown note file with YAML frontmatter into the repo
func writeNote(t *testing.T, dir, uid, title, status, content string) {
	t.Helper()
	body := fmt.Sprintf("---\nuid: %s\ntitle: %s\nstatus: %s\nauthor: tester\n---\n%s\n", uid, title, status, content)
	path := filepath.Join(dir, uid+".md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writeNote(%s): %v", uid, err)
	}
}

// helper: git add all and commit
func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-c", "user.email=test@test", "-c", "user.name=test", "commit", "-m", msg)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

func TestGrepNotes_basicMatch(t *testing.T) {
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	writeNote(t, dir, "TEST001", "Some Title", "active", "This note contains the keyword banana in it.")
	commitAll(t, dir, "add test note")

	results, err := GrepNotes(dir, []string{"banana"})
	if err != nil {
		t.Fatalf("GrepNotes: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].UID != "TEST001" {
		t.Errorf("UID = %q, want TEST001", results[0].UID)
	}
	if results[0].MatchCount < 1 {
		t.Errorf("MatchCount = %d, want >= 1", results[0].MatchCount)
	}
}

func TestGrepNotes_statusFilter(t *testing.T) {
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	writeNote(t, dir, "ACTIVE01", "Active Note", "active", "Contains keyword starfruit here.")
	writeNote(t, dir, "PEND01", "Pending Note", "pending", "Also contains keyword starfruit here.")
	commitAll(t, dir, "add notes")

	results, err := GrepNotes(dir, []string{"starfruit"})
	if err != nil {
		t.Fatalf("GrepNotes: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (only active)", len(results))
	}
	if results[0].UID != "ACTIVE01" {
		t.Errorf("UID = %q, want ACTIVE01", results[0].UID)
	}
}

func TestGrepNotes_titleWeighting(t *testing.T) {
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// Note with keyword in title (should get 3x weight for the title match)
	writeNote(t, dir, "TITLE01", "mango facts", "active", "This note is about fruit.")
	// Note with keyword only in content (no title bonus)
	writeNote(t, dir, "BODY01", "Fruit Guide", "active", "This note mentions mango once.")
	commitAll(t, dir, "add notes")

	results, err := GrepNotes(dir, []string{"mango"})
	if err != nil {
		t.Fatalf("GrepNotes: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	// The title-match note should have a higher count than the body-only note
	var titleCount, bodyCount int
	for _, r := range results {
		if r.UID == "TITLE01" {
			titleCount = r.MatchCount
		}
		if r.UID == "BODY01" {
			bodyCount = r.MatchCount
		}
	}
	if titleCount <= bodyCount {
		t.Errorf("title note MatchCount (%d) should be > body note MatchCount (%d)", titleCount, bodyCount)
	}
}

func TestGrepNotes_multipleKeywords(t *testing.T) {
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	writeNote(t, dir, "MULTI01", "Combined Note", "active", "This note has keyword papaya and also keyword guava in content.")
	commitAll(t, dir, "add note")

	results, err := GrepNotes(dir, []string{"papaya", "guava"})
	if err != nil {
		t.Fatalf("GrepNotes: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	// Match count should reflect hits from both keywords
	if results[0].MatchCount < 2 {
		t.Errorf("MatchCount = %d, want >= 2 (one per keyword)", results[0].MatchCount)
	}
}

func TestGrepNotes_noMatches(t *testing.T) {
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	writeNote(t, dir, "NOTE01", "A Note", "active", "Nothing special in this content.")
	commitAll(t, dir, "add note")

	results, err := GrepNotes(dir, []string{"zzzznonexistent"})
	if err != nil {
		t.Fatalf("GrepNotes: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestGrepNotes_emptyKeywords(t *testing.T) {
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	results, err := GrepNotes(dir, []string{})
	if err != nil {
		t.Fatalf("GrepNotes: %v", err)
	}
	if results != nil {
		t.Errorf("got %v, want nil for empty keywords", results)
	}

	results2, err2 := GrepNotes(dir, nil)
	if err2 != nil {
		t.Fatalf("GrepNotes(nil): %v", err2)
	}
	if results2 != nil {
		t.Errorf("got %v, want nil for nil keywords", results2)
	}
}
