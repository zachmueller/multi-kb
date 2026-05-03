package dreamcycle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockNoteStore is an in-memory NoteStore for testing ApplyActions.
type mockNoteStore struct {
	notes    map[string]Note
	deleted  []string
	commits  []string
	writeErr error
}

func newMockStore(notes ...Note) *mockNoteStore {
	m := &mockNoteStore{notes: make(map[string]Note)}
	for _, n := range notes {
		m.notes[n.UID] = n
	}
	return m
}

func (m *mockNoteStore) ReadNote(uid string) (*Note, error) {
	n, ok := m.notes[uid]
	if !ok {
		return nil, fmt.Errorf("note %q not found", uid)
	}
	return &n, nil
}

func (m *mockNoteStore) WriteNote(note Note) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.notes[note.UID] = note
	return nil
}

func (m *mockNoteStore) DeleteNote(uid string) error {
	delete(m.notes, uid)
	m.deleted = append(m.deleted, uid)
	return nil
}

func (m *mockNoteStore) CommitBatch(message string) error {
	m.commits = append(m.commits, message)
	return nil
}

func TestApplyActions_Keep(t *testing.T) {
	pending := Note{UID: "PEND0001", Title: "Test", Content: "body", Status: "pending"}
	store := newMockStore(pending)
	batch := Batch{PendingNote: pending}

	actions := []consolidationAction{
		{Type: "keep", SourceUID: "PEND0001", Reason: "novel"},
	}

	counts, err := ApplyActions(store, actions, batch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts["keep"] != 1 {
		t.Errorf("expected 1 keep, got %d", counts["keep"])
	}
	if store.notes["PEND0001"].Status != "active" {
		t.Error("expected status changed to active")
	}
}

func TestApplyActions_Merge(t *testing.T) {
	pending := Note{UID: "PEND0001", Title: "New Info", Content: "new body", Status: "pending"}
	active := Note{UID: "ACTV0001", Title: "Existing", Content: "old body", Status: "active"}
	store := newMockStore(pending, active)
	batch := Batch{PendingNote: pending, RelatedNotes: []Note{active}}

	actions := []consolidationAction{
		{
			Type:          "merge",
			SourceUID:     "PEND0001",
			TargetUID:     "ACTV0001",
			MergedTitle:   "Merged Title",
			MergedContent: "merged body containing old and new",
			Reason:        "duplicate",
		},
	}

	counts, err := ApplyActions(store, actions, batch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts["merge"] != 1 {
		t.Errorf("expected 1 merge, got %d", counts["merge"])
	}
	// Source should be deleted
	if _, exists := store.notes["PEND0001"]; exists {
		t.Error("expected source note to be deleted")
	}
	// Target should have merged content
	target := store.notes["ACTV0001"]
	if target.Title != "Merged Title" {
		t.Errorf("expected merged title, got %q", target.Title)
	}
}

func TestApplyActions_Split(t *testing.T) {
	pending := Note{UID: "PEND0001", Title: "Mixed Topics", Content: "body", Status: "pending", Author: "tester"}
	store := newMockStore(pending)
	batch := Batch{PendingNote: pending}

	actions := []consolidationAction{
		{
			Type:      "split",
			SourceUID: "PEND0001",
			NewNotes: []newNoteSpec{
				{Title: "Topic A", Content: "content a"},
				{Title: "Topic B", Content: "content b"},
			},
			Reason: "covers two distinct topics",
		},
	}

	counts, err := ApplyActions(store, actions, batch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts["split"] != 1 {
		t.Errorf("expected 1 split, got %d", counts["split"])
	}
	// Source should be deleted
	if _, exists := store.notes["PEND0001"]; exists {
		t.Error("expected source note to be deleted after split")
	}
	// Should have 2 new notes (with generated UIDs)
	activeCount := 0
	for _, n := range store.notes {
		if n.Status == "active" {
			activeCount++
		}
	}
	if activeCount != 2 {
		t.Errorf("expected 2 new active notes, got %d", activeCount)
	}
}

func TestApplyActions_SplitRequiresAtLeastTwo(t *testing.T) {
	pending := Note{UID: "PEND0001", Title: "Test", Content: "body", Status: "pending"}
	store := newMockStore(pending)
	batch := Batch{PendingNote: pending}

	actions := []consolidationAction{
		{
			Type:      "split",
			SourceUID: "PEND0001",
			NewNotes:  []newNoteSpec{{Title: "Only One", Content: "c"}},
			Reason:    "bad",
		},
	}

	_, err := ApplyActions(store, actions, batch)
	if err == nil {
		t.Fatal("expected error for split with <2 notes")
	}
}

func TestApplyActions_UnknownType(t *testing.T) {
	store := newMockStore()
	batch := Batch{PendingNote: Note{UID: "X"}}

	actions := []consolidationAction{
		{Type: "delete_everything", SourceUID: "X"},
	}

	_, err := ApplyActions(store, actions, batch)
	if err == nil {
		t.Fatal("expected error for unknown action type")
	}
}

func TestParseNote_WithFrontmatter(t *testing.T) {
	content := "---\nuid: ABC123\ntitle: Test Note\nstatus: active\nauthor: tester\n---\n\nSome body content."
	note, err := ParseNote("ABC123", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.UID != "ABC123" {
		t.Errorf("expected UID %q, got %q", "ABC123", note.UID)
	}
	if note.Title != "Test Note" {
		t.Errorf("expected title %q, got %q", "Test Note", note.Title)
	}
	if note.Status != "active" {
		t.Errorf("expected status %q, got %q", "active", note.Status)
	}
	if note.Content != "Some body content." {
		t.Errorf("unexpected content: %q", note.Content)
	}
}

func TestParseNote_NoFrontmatter(t *testing.T) {
	content := "Just plain text without frontmatter."
	note, err := ParseNote("FALLBACK", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.UID != "FALLBACK" {
		t.Errorf("expected fallback UID, got %q", note.UID)
	}
	if note.Content != "Just plain text without frontmatter." {
		t.Errorf("unexpected content: %q", note.Content)
	}
}

func TestCreateBatches(t *testing.T) {
	dir := t.TempDir()

	// Write a pending note
	pendingContent := "---\nuid: P001\ntitle: Pending\nstatus: pending\nauthor: test\n---\n\nPending body"
	if err := os.WriteFile(filepath.Join(dir, "P001.md"), []byte(pendingContent), 0o600); err != nil {
		t.Fatal(err)
	}

	// Write an active note (should not appear in batches)
	activeContent := "---\nuid: A001\ntitle: Active\nstatus: active\nauthor: test\n---\n\nActive body"
	if err := os.WriteFile(filepath.Join(dir, "A001.md"), []byte(activeContent), 0o600); err != nil {
		t.Fatal(err)
	}

	batches, err := CreateBatches(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch (pending only), got %d", len(batches))
	}
	if batches[0].PendingNote.UID != "P001" {
		t.Errorf("expected UID P001, got %q", batches[0].PendingNote.UID)
	}
}

func TestCreateBatches_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	batches, err := CreateBatches(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(batches) != 0 {
		t.Fatalf("expected 0 batches, got %d", len(batches))
	}
}

func TestDeriveKeywordsFromTitle(t *testing.T) {
	tests := []struct {
		title string
		min   int
		max   int
	}{
		{"Go Error Handling Patterns", 2, 5},
		{"the", 0, 0},
		{"AWS CDK VPC Setup Guide", 2, 5},
		{"a-b-c", 0, 0}, // all too short after splitting
	}

	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			kw := deriveKeywordsFromTitle(tc.title)
			if len(kw) < tc.min || len(kw) > tc.max {
				t.Errorf("deriveKeywordsFromTitle(%q) returned %d keywords %v, expected %d-%d",
					tc.title, len(kw), kw, tc.min, tc.max)
			}
		})
	}
}

func TestNoteToMarkdown(t *testing.T) {
	note := Note{
		UID:     "TEST0001",
		Title:   "Test Note",
		Status:  "active",
		Author:  "tester",
		Content: "Note body.",
	}

	md := note.ToMarkdown()
	if !strings.Contains(md, "uid: TEST0001") {
		t.Error("expected UID in markdown")
	}
	if !strings.Contains(md, "title: Test Note") {
		t.Error("expected title in markdown")
	}
	if !strings.Contains(md, "status: active") {
		t.Error("expected status in markdown")
	}
	if !strings.Contains(md, "Note body.") {
		t.Error("expected body in markdown")
	}
}

func TestParseConsolidationOutput_Valid(t *testing.T) {
	input := `{"actions":[{"type":"keep","source_uid":"P001","reason":"novel"}]}`
	output, err := parseConsolidationOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(output.Actions))
	}
	if output.Actions[0].Type != "keep" {
		t.Errorf("expected keep action, got %q", output.Actions[0].Type)
	}
}

func TestParseConsolidationOutput_MarkdownFenced(t *testing.T) {
	input := "```json\n{\"actions\":[{\"type\":\"keep\",\"source_uid\":\"P001\",\"reason\":\"novel\"}]}\n```"
	output, err := parseConsolidationOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(output.Actions))
	}
}

func TestParseConsolidationOutput_InvalidJSON(t *testing.T) {
	_, err := parseConsolidationOutput("not valid json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseConsolidationOutput_EmptyActions(t *testing.T) {
	_, err := parseConsolidationOutput(`{"actions":[]}`)
	if err == nil {
		t.Fatal("expected error for empty actions")
	}
}

func TestFormatCommitMessage(t *testing.T) {
	counts := map[string]int{"keep": 2, "merge": 1, "split": 0, "consolidate": 0}
	msg := formatCommitMessage(counts)
	if !strings.Contains(msg, "3 actions") {
		t.Errorf("expected total count in message, got: %s", msg)
	}
	if !strings.Contains(msg, "2K/1M/0S/0C") {
		t.Errorf("expected action breakdown, got: %s", msg)
	}
}

func TestBuildConsolidationMessage(t *testing.T) {
	batch := Batch{
		PendingNote: Note{UID: "P001", Title: "Pending", Content: "body"},
		RelatedNotes: []Note{
			{UID: "A001", Title: "Related", Content: "related body"},
		},
	}

	msg := buildConsolidationMessage(batch)
	if !strings.Contains(msg, "Pending Notes") {
		t.Error("expected pending section")
	}
	if !strings.Contains(msg, "Related Active Notes") {
		t.Error("expected related section")
	}
	if !strings.Contains(msg, "P001") {
		t.Error("expected pending UID")
	}
	if !strings.Contains(msg, "A001") {
		t.Error("expected related UID")
	}
}
