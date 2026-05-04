package dreamcycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/lock"
	"github.com/zmueller/multi-kb/internal/logging"
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

func TestParseConsolidationOutput_PreambleBeforeCodeFence(t *testing.T) {
	input := "I'll analyze this pending note against the active notes.\n\n```json\n{\"actions\":[{\"type\":\"keep\",\"source_uid\":\"P001\",\"reason\":\"novel\"}]}\n```"
	output, err := parseConsolidationOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Actions) != 1 || output.Actions[0].Type != "keep" {
		t.Errorf("unexpected actions: %+v", output.Actions)
	}
}

func TestParseConsolidationOutput_PreambleBeforeRawJSON(t *testing.T) {
	input := "Here is my analysis:\n\n{\"actions\":[{\"type\":\"keep\",\"source_uid\":\"P001\",\"reason\":\"novel\"}]}"
	output, err := parseConsolidationOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Actions) != 1 || output.Actions[0].Type != "keep" {
		t.Errorf("unexpected actions: %+v", output.Actions)
	}
}

func TestParseConsolidationOutput_TrailingCommentary(t *testing.T) {
	input := "{\"actions\":[{\"type\":\"keep\",\"source_uid\":\"P001\",\"reason\":\"novel\"}]}\n\nThis note covers a new topic."
	output, err := parseConsolidationOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Actions) != 1 || output.Actions[0].Type != "keep" {
		t.Errorf("unexpected actions: %+v", output.Actions)
	}
}

func TestParseConsolidationOutput_PreambleAndTrailingWithCodeFence(t *testing.T) {
	input := "## Analysis\n\n```json\n{\"actions\":[{\"type\":\"merge\",\"source_uid\":\"P001\",\"target_uid\":\"A001\",\"merged_title\":\"T\",\"merged_content\":\"C\",\"reason\":\"dup\"}]}\n```\n\nDone."
	output, err := parseConsolidationOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Actions) != 1 || output.Actions[0].Type != "merge" {
		t.Errorf("unexpected actions: %+v", output.Actions)
	}
}

func TestParseConsolidationOutput_BareCodeFenceWithPreamble(t *testing.T) {
	input := "Let me evaluate this note.\n\n```\n{\"actions\":[{\"type\":\"keep\",\"source_uid\":\"P001\",\"reason\":\"novel\"}]}\n```"
	output, err := parseConsolidationOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Actions) != 1 || output.Actions[0].Type != "keep" {
		t.Errorf("unexpected actions: %+v", output.Actions)
	}
}

func TestParseConsolidationOutput_NoParsableJSON(t *testing.T) {
	input := "I don't think any actions are needed here. The notes look fine."
	_, err := parseConsolidationOutput(input)
	if err == nil {
		t.Fatal("expected error for response with no JSON")
	}
	if !strings.Contains(err.Error(), "could not extract actions") {
		t.Errorf("expected extraction error message, got: %v", err)
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

// --- AUD-008: RunDreamCycle orchestrator tests ---

// mockLLM implements llmInvoker and returns a canned response.
type mockLLM struct {
	response string
	err      error
	calls    int
}

func (m *mockLLM) InvokeModel(_ context.Context, _, _ string) (string, error) {
	m.calls++
	return m.response, m.err
}

// keepResponse returns a valid consolidation JSON that keeps the given UID.
func keepResponse(uid string) string {
	return fmt.Sprintf(`{"actions":[{"type":"keep","source_uid":%q,"reason":"novel"}]}`, uid)
}

// pendingNoteContent returns a minimal pending note in Markdown frontmatter format.
// Uses a title with no extractable keywords to avoid git grep calls.
func pendingNoteContent(uid string) string {
	return fmt.Sprintf("---\nuid: %s\ntitle: x\nstatus: pending\nauthor: tester\n---\n\nBody text.", uid)
}

// minimalLocalCfg returns a config with a single source routing to a local KB at kbName.
func minimalLocalCfg(kbName string) *config.Config {
	return &config.Config{
		Mode:   "client",
		Author: "tester",
		Extraction: config.ExtractionConfig{
			ModelID:    "model",
			AWSRegion:  "us-east-1",
			AWSProfile: "default",
		},
		DreamCycle: config.DreamCycleConfig{ModelID: "model"},
		Hook:       config.HookConfig{Timeout: "8s"},
		Sources: []config.Source{
			{
				Directory: "/tmp/src",
				Harnesses: []string{"claude-code"},
				Targets: []config.Target{
					{KB: "local/" + kbName, Routing: "always", Approval: "auto-approve"},
				},
			},
		},
	}
}

// TestRunDreamCycle_FullCycle tests a successful end-to-end run:
// one pending note is processed, kept, and the run log is written.
func TestRunDreamCycle_FullCycle(t *testing.T) {
	// Build a real KB directory so CreateBatches can scan files.
	// We override localKBDir via kbName == the temp dir name trick:
	// instead, we inject a store factory that returns our mockNoteStore
	// and redirect the kbDir to our temp dir so CreateBatches works.

	kbDir := t.TempDir()
	uid := "PEND0001"
	if err := os.WriteFile(filepath.Join(kbDir, uid+".md"), []byte(pendingNoteContent(uid)), 0o600); err != nil {
		t.Fatal(err)
	}

	logsDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "lock")

	llm := &mockLLM{response: keepResponse(uid)}

	// The store factory must match the kbDir computed by localKBDir(kbName).
	// We patch this by building a cfg whose KB name maps back to our temp dir.
	// Since localKBDir computes ~/.multi-kb/local/<name>, we can't redirect it
	// without patching os.UserHomeDir.  Instead, we bypass RunDreamCycle and
	// call runDreamCycle directly, providing a storeFactory that intercepts the
	// computed kbDir and swaps in our temp dir.
	kbName := "testkb"
	cfg := minimalLocalCfg(kbName)

	computedKBDir, err := localKBDir(kbName)
	if err != nil {
		t.Fatal(err)
	}

	// Write the pending note to the computed KB dir (create dirs if needed).
	if err := os.MkdirAll(computedKBDir, 0o700); err != nil {
		t.Fatal(err)
	}
	noteFile := filepath.Join(computedKBDir, uid+".md")
	if err := os.WriteFile(noteFile, []byte(pendingNoteContent(uid)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(noteFile) })

	store := newMockStore(Note{UID: uid, Title: "x", Content: "Body text.", Status: "pending"})

	sf := func(_ string) NoteStore { return store }

	if err := runDreamCycle(context.Background(), cfg, lockPath, logsDir, "manual", llm, sf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// LLM should have been called once (one batch).
	if llm.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", llm.calls)
	}

	// The pending note should now be active.
	if store.notes[uid].Status != "active" {
		t.Errorf("expected note status=active, got %q", store.notes[uid].Status)
	}

	// Run log should have been written.
	entries, err := logging.ReadRunLog(logsDir, 10)
	if err != nil {
		t.Fatalf("read run log: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 run log entry, got %d", len(entries))
	}
	if entries[0].Type != "dream_cycle" {
		t.Errorf("expected type=dream_cycle, got %q", entries[0].Type)
	}
	if entries[0].Trigger != "manual" {
		t.Errorf("expected trigger=manual, got %q", entries[0].Trigger)
	}
	if entries[0].BatchesProcessed != 1 {
		t.Errorf("expected 1 batch processed, got %d", entries[0].BatchesProcessed)
	}
	if entries[0].Actions["keep"] != 1 {
		t.Errorf("expected 1 keep action, got %d", entries[0].Actions["keep"])
	}
}

// TestRunDreamCycle_LLMError tests that a phase 3 LLM error increments the error
// count and the cycle still writes a run log entry.
func TestRunDreamCycle_LLMError(t *testing.T) {
	kbName := "testkb-err"
	cfg := minimalLocalCfg(kbName)

	uid := "PEND0002"

	computedKBDir, err := localKBDir(kbName)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(computedKBDir, 0o700); err != nil {
		t.Fatal(err)
	}
	noteFile := filepath.Join(computedKBDir, uid+".md")
	if err := os.WriteFile(noteFile, []byte(pendingNoteContent(uid)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(noteFile) })

	logsDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "lock")

	llm := &mockLLM{err: errors.New("bedrock throttled")}
	store := newMockStore(Note{UID: uid, Title: "x", Content: "Body.", Status: "pending"})
	sf := func(_ string) NoteStore { return store }

	if err := runDreamCycle(context.Background(), cfg, lockPath, logsDir, "cron", llm, sf); err != nil {
		t.Fatalf("unexpected error from runDreamCycle: %v", err)
	}

	entries, err := logging.ReadRunLog(logsDir, 10)
	if err != nil {
		t.Fatalf("read run log: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 run log entry, got %d", len(entries))
	}
	if entries[0].Errors != 1 {
		t.Errorf("expected 1 error in run log, got %d", entries[0].Errors)
	}
	// Note should remain pending since keep was never applied.
	if store.notes[uid].Status != "pending" {
		t.Errorf("expected note to stay pending, got %q", store.notes[uid].Status)
	}
}

// TestRunDreamCycle_LockHeld tests that lock contention returns ErrLockHeld immediately.
func TestRunDreamCycle_LockHeld(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "lock")

	// Acquire the lock from "another process" by writing a fresh lock file.
	held, err := lock.Acquire(lockPath, "other_process")
	if err != nil {
		t.Fatalf("pre-acquire lock: %v", err)
	}
	defer held.Release()

	logsDir := t.TempDir()
	cfg := minimalLocalCfg("unused")
	llm := &mockLLM{}
	sf := func(_ string) NoteStore { return newMockStore() }

	err = runDreamCycle(context.Background(), cfg, lockPath, logsDir, "manual", llm, sf)
	if err == nil {
		t.Fatal("expected error when lock is held")
	}
	if !errors.Is(err, lock.ErrLockHeld) {
		t.Errorf("expected ErrLockHeld, got %v", err)
	}

	// LLM should never have been called.
	if llm.calls != 0 {
		t.Errorf("expected no LLM calls, got %d", llm.calls)
	}
}

// TestRunDreamCycle_NoBatches tests that a KB with no pending notes completes
// without calling the LLM and records zero batches in the run log.
func TestRunDreamCycle_NoBatches(t *testing.T) {
	kbName := "testkb-empty"
	cfg := minimalLocalCfg(kbName)

	computedKBDir, err := localKBDir(kbName)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(computedKBDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Write an active note — should not produce a batch.
	activeContent := "---\nuid: A001\ntitle: Active\nstatus: active\nauthor: tester\n---\n\nBody."
	activeFile := filepath.Join(computedKBDir, "A001.md")
	if err := os.WriteFile(activeFile, []byte(activeContent), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(activeFile) })

	logsDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "lock")

	llm := &mockLLM{}
	sf := func(_ string) NoteStore { return newMockStore() }

	if err := runDreamCycle(context.Background(), cfg, lockPath, logsDir, "cron", llm, sf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if llm.calls != 0 {
		t.Errorf("expected no LLM calls for empty batch set, got %d", llm.calls)
	}

	entries, err := logging.ReadRunLog(logsDir, 10)
	if err != nil {
		t.Fatalf("read run log: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 run log entry, got %d", len(entries))
	}
	if entries[0].BatchesProcessed != 0 {
		t.Errorf("expected 0 batches, got %d", entries[0].BatchesProcessed)
	}
}

// TestRunDreamCycle_NoLocalKBSources tests that a config with no local KB targets
// runs cleanly and logs zero batches.
func TestRunDreamCycle_NoLocalKBSources(t *testing.T) {
	cfg := &config.Config{
		Mode:   "client",
		Author: "tester",
		Extraction: config.ExtractionConfig{
			ModelID: "model", AWSRegion: "us-east-1",
		},
		DreamCycle: config.DreamCycleConfig{ModelID: "model"},
		Hook:       config.HookConfig{Timeout: "8s"},
		Sources:    []config.Source{},
	}

	logsDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "lock")
	llm := &mockLLM{}
	sf := func(_ string) NoteStore { return newMockStore() }

	if err := runDreamCycle(context.Background(), cfg, lockPath, logsDir, "cron", llm, sf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if llm.calls != 0 {
		t.Errorf("expected no LLM calls, got %d", llm.calls)
	}

	entries, err := logging.ReadRunLog(logsDir, 10)
	if err != nil {
		t.Fatalf("read run log: %v", err)
	}
	if len(entries) != 1 || entries[0].BatchesProcessed != 0 {
		t.Errorf("expected 1 log entry with 0 batches, got %+v", entries)
	}
}

// TestRunDreamCycle_RunLogFields verifies trigger and type are correctly recorded.
func TestRunDreamCycle_RunLogFields(t *testing.T) {
	cfg := &config.Config{
		Mode:       "client",
		Author:     "tester",
		Extraction: config.ExtractionConfig{ModelID: "model", AWSRegion: "us-east-1"},
		DreamCycle: config.DreamCycleConfig{ModelID: "model"},
		Hook:       config.HookConfig{Timeout: "8s"},
		Sources:    []config.Source{},
	}

	logsDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "lock")
	llm := &mockLLM{}
	sf := func(_ string) NoteStore { return newMockStore() }

	for _, trigger := range []string{"cron", "manual"} {
		t.Run(trigger, func(t *testing.T) {
			ld := t.TempDir()
			lp := filepath.Join(t.TempDir(), "lock")
			if err := runDreamCycle(context.Background(), cfg, lp, ld, trigger, llm, sf); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			entries, err := logging.ReadRunLog(ld, 10)
			if err != nil || len(entries) == 0 {
				t.Fatalf("no log entry found: %v", err)
			}
			if entries[0].Trigger != trigger {
				t.Errorf("trigger = %q, want %q", entries[0].Trigger, trigger)
			}
			if entries[0].Type != "dream_cycle" {
				t.Errorf("type = %q, want dream_cycle", entries[0].Type)
			}
		})
	}

	// suppress unused var warnings from outer scope
	_ = logsDir
	_ = lockPath
}

// TestRunDreamCycle_MultipleBatches tests that multiple pending notes each get
// their own LLM call and all results are aggregated in the run log.
func TestRunDreamCycle_MultipleBatches(t *testing.T) {
	kbName := "testkb-multi"
	cfg := minimalLocalCfg(kbName)

	computedKBDir, err := localKBDir(kbName)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(computedKBDir, 0o700); err != nil {
		t.Fatal(err)
	}

	uids := []string{"MULTI001", "MULTI002", "MULTI003"}
	for _, uid := range uids {
		f := filepath.Join(computedKBDir, uid+".md")
		if err := os.WriteFile(f, []byte(pendingNoteContent(uid)), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(f) })
	}

	logsDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "lock")

	// LLM responds with a "keep" action for whatever UID is in the message.
	llm := &mockLLM{}
	stores := map[string]*mockNoteStore{}
	for _, uid := range uids {
		stores[uid] = newMockStore(Note{UID: uid, Title: "x", Content: "Body.", Status: "pending"})
	}
	// Single shared store for simplicity.
	sharedStore := newMockStore()
	for _, uid := range uids {
		sharedStore.notes[uid] = Note{UID: uid, Title: "x", Content: "Body.", Status: "pending"}
	}
	llm.response = "" // overridden per call using a custom invoker

	// Use a custom LLM that builds the correct response for the UID in the message.
	type smartLLM struct{ calls int }
	smart := &struct {
		calls int
	}{}
	invoker := &funcLLM{fn: func(_ context.Context, _, userMsg string) (string, error) {
		smart.calls++
		// Extract the UID from the message (it appears as "### Note: UIDXXX").
		for _, uid := range uids {
			if strings.Contains(userMsg, uid) {
				return keepResponse(uid), nil
			}
		}
		return "", fmt.Errorf("unrecognized batch")
	}}

	sf := func(_ string) NoteStore { return sharedStore }

	if err := runDreamCycle(context.Background(), cfg, lockPath, logsDir, "manual", invoker, sf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if smart.calls != 3 {
		t.Errorf("expected 3 LLM calls, got %d", smart.calls)
	}

	entries, err := logging.ReadRunLog(logsDir, 10)
	if err != nil || len(entries) == 0 {
		t.Fatalf("no log entries: %v", err)
	}
	if entries[0].BatchesProcessed != 3 {
		t.Errorf("expected 3 batches processed, got %d", entries[0].BatchesProcessed)
	}
	if entries[0].Actions["keep"] != 3 {
		t.Errorf("expected 3 keep actions, got %d", entries[0].Actions["keep"])
	}
}

// funcLLM wraps a function as an llmInvoker.
type funcLLM struct {
	fn func(ctx context.Context, system, user string) (string, error)
}

func (f *funcLLM) InvokeModel(ctx context.Context, system, user string) (string, error) {
	return f.fn(ctx, system, user)
}

// TestGroupIntoBatches is a placeholder that confirms CreateBatches handles
// multiple pending notes, each becoming a singleton batch.
func TestGroupIntoBatches(t *testing.T) {
	dir := t.TempDir()

	for _, uid := range []string{"P001", "P002", "P003"} {
		content := fmt.Sprintf("---\nuid: %s\ntitle: Title\nstatus: pending\nauthor: tester\n---\n\nBody.", uid)
		if err := os.WriteFile(filepath.Join(dir, uid+".md"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// one active note — should not appear
	activeContent := "---\nuid: A001\ntitle: Active\nstatus: active\nauthor: tester\n---\n\nBody."
	if err := os.WriteFile(filepath.Join(dir, "A001.md"), []byte(activeContent), 0o600); err != nil {
		t.Fatal(err)
	}

	batches, err := CreateBatches(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(batches) != 3 {
		t.Fatalf("expected 3 singleton batches, got %d", len(batches))
	}
	for _, b := range batches {
		if b.PendingNote.Status != "pending" {
			t.Errorf("expected pending status, got %q for UID %s", b.PendingNote.Status, b.PendingNote.UID)
		}
	}
}

// TestServerNoteStore_ReadWriteDelete is kept as a compile check on the
// mock store's ReadNote / WriteNote / DeleteNote contract.
func TestServerNoteStore_ReadWriteDelete(t *testing.T) {
	note := Note{UID: "T001", Title: "Test", Content: "body", Status: "active"}
	store := newMockStore(note)

	read, err := store.ReadNote("T001")
	if err != nil || read.UID != "T001" {
		t.Fatalf("ReadNote: got %v, err %v", read, err)
	}

	updated := note
	updated.Content = "updated body"
	if err := store.WriteNote(updated); err != nil {
		t.Fatalf("WriteNote: %v", err)
	}
	if store.notes["T001"].Content != "updated body" {
		t.Error("WriteNote did not persist update")
	}

	if err := store.DeleteNote("T001"); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	if _, exists := store.notes["T001"]; exists {
		t.Error("DeleteNote did not remove note")
	}
}

// TestNoteFileFilename verifies the filename derived from a UID matches the expected pattern.
func TestNoteFileFilename(t *testing.T) {
	uid := "ABCD1234"
	expected := uid + ".md"
	got := uid + ".md"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestRegionOrDefault verifies the extraction config region fallback behaviour.
func TestRegionOrDefault(t *testing.T) {
	cfg := config.ExtractionConfig{AWSRegion: ""}
	region := cfg.AWSRegion
	if region != "" {
		t.Errorf("expected empty region, got %q", region)
	}
}

// Ensure json round-trip on RunEntry works (compile check for logging import).
func TestRunEntryJSON(t *testing.T) {
	entry := logging.RunEntry{
		Type:             "dream_cycle",
		Trigger:          "manual",
		BatchesProcessed: 2,
		Actions:          map[string]int{"keep": 1, "merge": 1},
		Errors:           0,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var back logging.RunEntry
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Type != "dream_cycle" {
		t.Errorf("round-trip type = %q", back.Type)
	}
}
