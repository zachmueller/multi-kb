package route

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAndList(t *testing.T) {
	dir := t.TempDir()

	entry := PendingEntry{
		Title:              "Test Note",
		Content:            "Some knowledge content",
		Author:             "tester",
		TargetKBs:          []string{"kb1", "kb2"},
		SourceConversation: "conv-001",
		ExtractedAt:        "2025-06-01T12:00:00Z",
	}

	filename, err := CreatePending(dir, entry)
	if err != nil {
		t.Fatalf("CreatePending failed: %v", err)
	}
	if filename == "" {
		t.Fatal("CreatePending returned empty filename")
	}

	names, err := ListPending(dir)
	if err != nil {
		t.Fatalf("ListPending failed: %v", err)
	}
	if len(names) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(names))
	}
	if names[0] != filename {
		t.Errorf("expected filename %q, got %q", filename, names[0])
	}

	// Verify the contents on disk
	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		t.Fatalf("cannot read pending file: %v", err)
	}
	var got PendingEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("cannot unmarshal pending file: %v", err)
	}
	if got.Title != entry.Title {
		t.Errorf("title: got %q, want %q", got.Title, entry.Title)
	}
	if got.Content != entry.Content {
		t.Errorf("content: got %q, want %q", got.Content, entry.Content)
	}
	if got.Author != entry.Author {
		t.Errorf("author: got %q, want %q", got.Author, entry.Author)
	}
	if len(got.TargetKBs) != 2 {
		t.Errorf("target_kbs length: got %d, want 2", len(got.TargetKBs))
	}
	if got.SourceConversation != entry.SourceConversation {
		t.Errorf("source_conversation: got %q, want %q", got.SourceConversation, entry.SourceConversation)
	}
}

func TestMultipleEntriesPendingCount(t *testing.T) {
	dir := t.TempDir()

	entries := []PendingEntry{
		{Title: "Note A", Content: "Content A", Author: "a", TargetKBs: []string{"kb1"}, ExtractedAt: "2025-06-01T12:00:00Z"},
		{Title: "Note B", Content: "Content B", Author: "b", TargetKBs: []string{"kb2"}, ExtractedAt: "2025-06-01T12:01:00Z"},
		{Title: "Note C", Content: "Content C", Author: "c", TargetKBs: []string{"kb3"}, ExtractedAt: "2025-06-01T12:02:00Z"},
	}

	for _, e := range entries {
		if _, err := CreatePending(dir, e); err != nil {
			t.Fatalf("CreatePending failed: %v", err)
		}
	}

	count, err := PendingCount(dir)
	if err != nil {
		t.Fatalf("PendingCount failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestUpdateRemovesTarget(t *testing.T) {
	dir := t.TempDir()

	entry := PendingEntry{
		Title:       "Multi Target",
		Content:     "Content",
		Author:      "tester",
		TargetKBs:   []string{"kb1", "kb2"},
		ExtractedAt: "2025-06-01T12:00:00Z",
	}

	filename, err := CreatePending(dir, entry)
	if err != nil {
		t.Fatalf("CreatePending failed: %v", err)
	}

	if err := UpdatePending(dir, filename, "kb1"); err != nil {
		t.Fatalf("UpdatePending failed: %v", err)
	}

	// File should still exist with 1 target remaining
	updated, err := ReadPending(dir, filename)
	if err != nil {
		t.Fatalf("ReadPending after update failed: %v", err)
	}
	if len(updated.TargetKBs) != 1 {
		t.Fatalf("expected 1 target remaining, got %d", len(updated.TargetKBs))
	}
	if updated.TargetKBs[0] != "kb2" {
		t.Errorf("expected remaining target %q, got %q", "kb2", updated.TargetKBs[0])
	}
}

func TestUpdateDeletesWhenEmpty(t *testing.T) {
	dir := t.TempDir()

	entry := PendingEntry{
		Title:       "Single Target",
		Content:     "Content",
		Author:      "tester",
		TargetKBs:   []string{"kb1"},
		ExtractedAt: "2025-06-01T12:00:00Z",
	}

	filename, err := CreatePending(dir, entry)
	if err != nil {
		t.Fatalf("CreatePending failed: %v", err)
	}

	if err := UpdatePending(dir, filename, "kb1"); err != nil {
		t.Fatalf("UpdatePending failed: %v", err)
	}

	// File should be deleted
	path := filepath.Join(dir, filename)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted, but it still exists")
	}
}

func TestDeletePending(t *testing.T) {
	dir := t.TempDir()

	entry := PendingEntry{
		Title:       "To Delete",
		Content:     "Content",
		Author:      "tester",
		TargetKBs:   []string{"kb1"},
		ExtractedAt: "2025-06-01T12:00:00Z",
	}

	filename, err := CreatePending(dir, entry)
	if err != nil {
		t.Fatalf("CreatePending failed: %v", err)
	}

	if err := DeletePending(dir, filename); err != nil {
		t.Fatalf("DeletePending failed: %v", err)
	}

	path := filepath.Join(dir, filename)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file to be gone after delete")
	}
}

func TestEmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	names, err := ListPending(dir)
	if err != nil {
		t.Fatalf("ListPending on empty dir failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected empty slice, got %d items", len(names))
	}
}

func TestCreatesDirectory(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nonexistent", "subdir")

	entry := PendingEntry{
		Title:       "Auto Dir",
		Content:     "Content",
		Author:      "tester",
		TargetKBs:   []string{"kb1"},
		ExtractedAt: "2025-06-01T12:00:00Z",
	}

	filename, err := CreatePending(dir, entry)
	if err != nil {
		t.Fatalf("CreatePending on non-existent dir failed: %v", err)
	}

	// Directory should now exist
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected a directory")
	}

	// File should exist
	if _, err := os.Stat(filepath.Join(dir, filename)); err != nil {
		t.Errorf("file not found in created directory: %v", err)
	}
}
