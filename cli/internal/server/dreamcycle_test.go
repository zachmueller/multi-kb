package server

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/document"
	bratypes "github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/types"
	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/dreamcycle"
)

func TestParseRetrieveResults_ValidResults(t *testing.T) {
	results := []bratypes.KnowledgeBaseRetrievalResult{
		{
			Content: &bratypes.RetrievalResultContent{
				Text: strPtr("note body content"),
			},
			Metadata: map[string]document.Interface{
				"uid":    document.NewLazyDocument("ABC123"),
				"title":  document.NewLazyDocument("Test Note"),
				"status": document.NewLazyDocument("pending"),
				"author": document.NewLazyDocument("tester"),
			},
			Score: float64Ptr(0.85),
		},
	}

	notes := parseRetrieveResults(results)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].UID != "ABC123" {
		t.Errorf("expected UID ABC123, got %q", notes[0].UID)
	}
	if notes[0].Title != "Test Note" {
		t.Errorf("expected title %q, got %q", "Test Note", notes[0].Title)
	}
	if notes[0].Status != "pending" {
		t.Errorf("expected status %q, got %q", "pending", notes[0].Status)
	}
	if notes[0].Author != "tester" {
		t.Errorf("expected author %q, got %q", "tester", notes[0].Author)
	}
	if notes[0].Content != "note body content" {
		t.Errorf("expected content %q, got %q", "note body content", notes[0].Content)
	}
}

func TestParseRetrieveResults_EmptyResults(t *testing.T) {
	notes := parseRetrieveResults(nil)
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes, got %d", len(notes))
	}
}

func TestParseRetrieveResults_MissingUID(t *testing.T) {
	results := []bratypes.KnowledgeBaseRetrievalResult{
		{
			Content: &bratypes.RetrievalResultContent{
				Text: strPtr("some content"),
			},
			Metadata: map[string]document.Interface{
				"title":  document.NewLazyDocument("No UID"),
				"status": document.NewLazyDocument("pending"),
			},
		},
	}
	notes := parseRetrieveResults(results)
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes (uid required), got %d", len(notes))
	}
}

func TestParseRetrieveResults_NilMetadata(t *testing.T) {
	results := []bratypes.KnowledgeBaseRetrievalResult{
		{
			Content: &bratypes.RetrievalResultContent{
				Text: strPtr("content"),
			},
			Metadata: nil,
		},
	}
	notes := parseRetrieveResults(results)
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes (nil metadata → no uid), got %d", len(notes))
	}
}

func TestParseRetrieveResults_NilContent(t *testing.T) {
	results := []bratypes.KnowledgeBaseRetrievalResult{
		{
			Content: nil,
			Metadata: map[string]document.Interface{
				"uid":    document.NewLazyDocument("UID1"),
				"title":  document.NewLazyDocument("Title"),
				"status": document.NewLazyDocument("active"),
			},
		},
	}
	notes := parseRetrieveResults(results)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Content != "" {
		t.Errorf("expected empty content, got %q", notes[0].Content)
	}
}

func TestGroupIntoBatches(t *testing.T) {
	notes := []dreamcycle.Note{
		{UID: "A", Title: "Note A"},
		{UID: "B", Title: "Note B"},
		{UID: "C", Title: "Note C"},
	}

	batches := groupIntoBatches(nil, nil, notes)
	if len(batches) != 3 {
		t.Fatalf("expected 3 singleton batches, got %d", len(batches))
	}
	for i, b := range batches {
		if b.PendingNote.UID != notes[i].UID {
			t.Errorf("batch %d: expected UID %q, got %q", i, notes[i].UID, b.PendingNote.UID)
		}
	}
}

func TestGroupIntoBatches_Empty(t *testing.T) {
	batches := groupIntoBatches(nil, nil, nil)
	if len(batches) != 0 {
		t.Fatalf("expected 0 batches, got %d", len(batches))
	}
}

func TestServerNoteStore_ReadWriteDelete(t *testing.T) {
	dir := t.TempDir()
	store := &serverNoteStore{repoDir: dir}

	note := dreamcycle.Note{
		UID:     "TEST001",
		Title:   "Test",
		Content: "body",
		Status:  "pending",
		Author:  "tester",
	}

	if err := store.WriteNote(note); err != nil {
		t.Fatalf("write error: %v", err)
	}

	read, err := store.ReadNote("TEST001")
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if read.UID != "TEST001" {
		t.Errorf("expected UID %q, got %q", "TEST001", read.UID)
	}
	if read.Title != "Test" {
		t.Errorf("expected title %q, got %q", "Test", read.Title)
	}

	if err := store.DeleteNote("TEST001"); err != nil {
		t.Fatalf("delete error: %v", err)
	}

	_, err = store.ReadNote("TEST001")
	if err == nil {
		t.Error("expected error reading deleted note")
	}
}

func TestNoteFileFilename(t *testing.T) {
	n := NoteFile{UID: "ABCD1234"}
	if fn := n.Filename(); fn != "ABCD1234.md" {
		t.Errorf("expected %q, got %q", "ABCD1234.md", fn)
	}
}

func TestRegionOrDefault(t *testing.T) {
	cfg := &config.Config{S3: &config.S3Config{Region: "us-west-2"}}
	if got := regionOrDefault(cfg); got != "us-west-2" {
		t.Errorf("expected us-west-2, got %q", got)
	}

	cfg2 := &config.Config{CodeCommit: &config.CodeCommitConfig{Region: "eu-west-1"}}
	if got := regionOrDefault(cfg2); got != "eu-west-1" {
		t.Errorf("expected eu-west-1, got %q", got)
	}

	cfg3 := &config.Config{}
	if got := regionOrDefault(cfg3); got != "us-east-1" {
		t.Errorf("expected us-east-1, got %q", got)
	}
}

func strPtr(s string) *string  { return &s }
func float64Ptr(f float64) *float64 { return &f }
