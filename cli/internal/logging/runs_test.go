package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendAndReadRunLog(t *testing.T) {
	dir := t.TempDir()

	entry := RunEntry{
		Timestamp:              "2026-05-02T10:00:00Z",
		Type:                   "capture",
		Trigger:                "manual",
		DirectoriesScanned:     5,
		ConversationsProcessed: 3,
		NotesExtracted:         10,
		Errors:                 0,
		DurationMS:             1234,
	}

	if err := AppendRunLog(dir, entry); err != nil {
		t.Fatalf("AppendRunLog failed: %v", err)
	}

	entries, err := ReadRunLog(dir, 10)
	if err != nil {
		t.Fatalf("ReadRunLog failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("ReadRunLog returned %d entries, want 1", len(entries))
	}

	got := entries[0]
	if got.Timestamp != entry.Timestamp {
		t.Errorf("Timestamp = %q, want %q", got.Timestamp, entry.Timestamp)
	}
	if got.Type != entry.Type {
		t.Errorf("Type = %q, want %q", got.Type, entry.Type)
	}
	if got.Trigger != entry.Trigger {
		t.Errorf("Trigger = %q, want %q", got.Trigger, entry.Trigger)
	}
	if got.DirectoriesScanned != entry.DirectoriesScanned {
		t.Errorf("DirectoriesScanned = %d, want %d", got.DirectoriesScanned, entry.DirectoriesScanned)
	}
	if got.ConversationsProcessed != entry.ConversationsProcessed {
		t.Errorf("ConversationsProcessed = %d, want %d", got.ConversationsProcessed, entry.ConversationsProcessed)
	}
	if got.NotesExtracted != entry.NotesExtracted {
		t.Errorf("NotesExtracted = %d, want %d", got.NotesExtracted, entry.NotesExtracted)
	}
	if got.Errors != entry.Errors {
		t.Errorf("Errors = %d, want %d", got.Errors, entry.Errors)
	}
	if got.DurationMS != entry.DurationMS {
		t.Errorf("DurationMS = %d, want %d", got.DurationMS, entry.DurationMS)
	}
}

func TestAppendMultipleAndReadWithLimit(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 3; i++ {
		entry := RunEntry{
			Timestamp:  "2026-05-02T10:00:00Z",
			Type:       "capture",
			Trigger:    "cron",
			DurationMS: int64((i + 1) * 100),
			Errors:     0,
		}
		if err := AppendRunLog(dir, entry); err != nil {
			t.Fatalf("AppendRunLog [%d] failed: %v", i, err)
		}
	}

	// Read with maxEntries=2 → should return last 2
	entries, err := ReadRunLog(dir, 2)
	if err != nil {
		t.Fatalf("ReadRunLog failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("ReadRunLog returned %d entries, want 2", len(entries))
	}

	// Last two entries have DurationMS 200 and 300
	if entries[0].DurationMS != 200 {
		t.Errorf("entries[0].DurationMS = %d, want 200", entries[0].DurationMS)
	}
	if entries[1].DurationMS != 300 {
		t.Errorf("entries[1].DurationMS = %d, want 300", entries[1].DurationMS)
	}
}

func TestAppendExtractionError(t *testing.T) {
	dir := t.TempDir()

	entry := ExtractionErrorEntry{
		Timestamp:      "2026-05-02T10:00:00Z",
		ConversationID: "conv-123",
		SourcePath:     "/path/to/source",
		Error:          "parse failure",
		Retries:        2,
	}

	if err := AppendExtractionError(dir, entry); err != nil {
		t.Fatalf("AppendExtractionError failed: %v", err)
	}

	path := filepath.Join(dir, "extraction-errors.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read extraction-errors.jsonl: %v", err)
	}

	var got ExtractionErrorEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &got); err != nil {
		t.Fatalf("unmarshal extraction error: %v", err)
	}

	if got.ConversationID != entry.ConversationID {
		t.Errorf("ConversationID = %q, want %q", got.ConversationID, entry.ConversationID)
	}
	if got.SourcePath != entry.SourcePath {
		t.Errorf("SourcePath = %q, want %q", got.SourcePath, entry.SourcePath)
	}
	if got.Error != entry.Error {
		t.Errorf("Error = %q, want %q", got.Error, entry.Error)
	}
	if got.Retries != entry.Retries {
		t.Errorf("Retries = %d, want %d", got.Retries, entry.Retries)
	}
}

func TestAppendHookError(t *testing.T) {
	dir := t.TempDir()

	entry := HookErrorEntry{
		Timestamp:      "2026-05-02T10:00:00Z",
		Harness:        "claude",
		Directory:      "/home/user/.claude",
		Error:          "hook timeout",
		PartialResults: true,
	}

	if err := AppendHookError(dir, entry); err != nil {
		t.Fatalf("AppendHookError failed: %v", err)
	}

	path := filepath.Join(dir, "hook-errors.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hook-errors.jsonl: %v", err)
	}

	var got HookErrorEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &got); err != nil {
		t.Fatalf("unmarshal hook error: %v", err)
	}

	if got.Harness != entry.Harness {
		t.Errorf("Harness = %q, want %q", got.Harness, entry.Harness)
	}
	if got.Directory != entry.Directory {
		t.Errorf("Directory = %q, want %q", got.Directory, entry.Directory)
	}
	if got.Error != entry.Error {
		t.Errorf("Error = %q, want %q", got.Error, entry.Error)
	}
	if got.PartialResults != entry.PartialResults {
		t.Errorf("PartialResults = %v, want %v", got.PartialResults, entry.PartialResults)
	}
}

func TestCreatesDirectory(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "logs")

	entry := RunEntry{
		Timestamp:  "2026-05-02T10:00:00Z",
		Type:       "dream_cycle",
		Trigger:    "cron",
		DurationMS: 500,
		Errors:     0,
	}

	if err := AppendRunLog(dir, entry); err != nil {
		t.Fatalf("AppendRunLog to non-existent dir failed: %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected a directory")
	}

	// Verify we can read back
	entries, err := ReadRunLog(dir, 10)
	if err != nil {
		t.Fatalf("ReadRunLog failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestEmptyLog(t *testing.T) {
	dir := t.TempDir()

	// ReadRunLog on non-existent file → returns empty slice, no error
	entries, err := ReadRunLog(dir, 10)
	if err != nil {
		t.Fatalf("ReadRunLog on empty dir should not error: %v", err)
	}
	if entries != nil && len(entries) != 0 {
		t.Errorf("ReadRunLog on empty dir returned %d entries, want 0", len(entries))
	}
}
