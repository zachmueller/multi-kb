package server

import (
	"testing"

	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/dreamcycle"
)

func TestParseOpenSearchNotes_ValidResults(t *testing.T) {
	result := map[string]interface{}{
		"hits": map[string]interface{}{
			"hits": []interface{}{
				map[string]interface{}{
					"_source": map[string]interface{}{
						"AMAZON_BEDROCK_METADATA": map[string]interface{}{
							"uid":    "ABC123",
							"title":  "Test Note",
							"status": "pending",
							"author": "tester",
						},
						"AMAZON_BEDROCK_TEXT_CHUNK": "note body content",
					},
				},
			},
		},
	}

	notes := parseOpenSearchNotes(result)
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
	if notes[0].Content != "note body content" {
		t.Errorf("expected content %q, got %q", "note body content", notes[0].Content)
	}
}

func TestParseOpenSearchNotes_EmptyResults(t *testing.T) {
	result := map[string]interface{}{
		"hits": map[string]interface{}{
			"hits": []interface{}{},
		},
	}
	notes := parseOpenSearchNotes(result)
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes, got %d", len(notes))
	}
}

func TestParseOpenSearchNotes_MissingUID(t *testing.T) {
	result := map[string]interface{}{
		"hits": map[string]interface{}{
			"hits": []interface{}{
				map[string]interface{}{
					"_source": map[string]interface{}{
						"AMAZON_BEDROCK_METADATA": map[string]interface{}{
							"title":  "No UID",
							"status": "pending",
						},
					},
				},
			},
		},
	}
	notes := parseOpenSearchNotes(result)
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes (uid required), got %d", len(notes))
	}
}

func TestParseOpenSearchNotes_MalformedStructure(t *testing.T) {
	// Missing hits entirely
	result := map[string]interface{}{}
	notes := parseOpenSearchNotes(result)
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes, got %d", len(notes))
	}

	// hits is wrong type
	result2 := map[string]interface{}{"hits": "wrong type"}
	notes2 := parseOpenSearchNotes(result2)
	if len(notes2) != 0 {
		t.Fatalf("expected 0 notes, got %d", len(notes2))
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
	// With S3 region
	cfg := &config.Config{S3: &config.S3Config{Region: "us-west-2"}}
	if got := regionOrDefault(cfg); got != "us-west-2" {
		t.Errorf("expected us-west-2, got %q", got)
	}

	// With CodeCommit region, no S3
	cfg2 := &config.Config{CodeCommit: &config.CodeCommitConfig{Region: "eu-west-1"}}
	if got := regionOrDefault(cfg2); got != "eu-west-1" {
		t.Errorf("expected eu-west-1, got %q", got)
	}

	// Default
	cfg3 := &config.Config{}
	if got := regionOrDefault(cfg3); got != "us-east-1" {
		t.Errorf("expected us-east-1, got %q", got)
	}
}
