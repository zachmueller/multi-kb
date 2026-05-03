package extract

import (
	"strings"
	"testing"
)

func TestParseExtractionOutput_ValidArray(t *testing.T) {
	input := `[{"title":"Config patterns","content":"Useful knowledge about config.","suggested_target_kbs":["local/dev"]}]`
	notes, warnings := ParseExtractionOutput(input)
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Title != "Config patterns" {
		t.Errorf("expected title %q, got %q", "Config patterns", notes[0].Title)
	}
	if notes[0].Content != "Useful knowledge about config." {
		t.Errorf("unexpected content: %q", notes[0].Content)
	}
	if len(notes[0].SuggestedTargetKBs) != 1 || notes[0].SuggestedTargetKBs[0] != "local/dev" {
		t.Errorf("unexpected target KBs: %v", notes[0].SuggestedTargetKBs)
	}
}

func TestParseExtractionOutput_EmptyArray(t *testing.T) {
	notes, warnings := ParseExtractionOutput("[]")
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(notes))
	}
}

func TestParseExtractionOutput_MarkdownFenced(t *testing.T) {
	input := "```json\n[{\"title\":\"Note\",\"content\":\"Body\",\"suggested_target_kbs\":[]}]\n```"
	notes, warnings := ParseExtractionOutput(input)
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Title != "Note" {
		t.Errorf("expected title %q, got %q", "Note", notes[0].Title)
	}
}

func TestParseExtractionOutput_InvalidJSON(t *testing.T) {
	notes, warnings := ParseExtractionOutput("not json at all")
	if len(notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(notes))
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if !strings.Contains(warnings[0], "failed to parse JSON") {
		t.Errorf("expected JSON parse warning, got: %s", warnings[0])
	}
}

func TestParseExtractionOutput_MissingTitle(t *testing.T) {
	input := `[{"title":"","content":"Body","suggested_target_kbs":[]}]`
	notes, warnings := ParseExtractionOutput(input)
	if len(notes) != 0 {
		t.Errorf("expected note dropped, got %d notes", len(notes))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "missing or empty title") {
		t.Errorf("expected title warning, got: %v", warnings)
	}
}

func TestParseExtractionOutput_TitleTooLong(t *testing.T) {
	longTitle := strings.Repeat("A", 256)
	input := `[{"title":"` + longTitle + `","content":"Body","suggested_target_kbs":[]}]`
	notes, warnings := ParseExtractionOutput(input)
	if len(notes) != 0 {
		t.Errorf("expected note dropped, got %d notes", len(notes))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "title exceeds 255") {
		t.Errorf("expected title length warning, got: %v", warnings)
	}
}

func TestParseExtractionOutput_ContentTooLong(t *testing.T) {
	longContent := strings.Repeat("X", 100_001)
	input := `[{"title":"OK Title","content":"` + longContent + `","suggested_target_kbs":[]}]`
	notes, warnings := ParseExtractionOutput(input)
	if len(notes) != 0 {
		t.Errorf("expected note dropped, got %d notes", len(notes))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "content exceeds 100K") {
		t.Errorf("expected content length warning, got: %v", warnings)
	}
}

func TestParseExtractionOutput_MissingContent(t *testing.T) {
	input := `[{"title":"Has Title","content":"","suggested_target_kbs":[]}]`
	notes, warnings := ParseExtractionOutput(input)
	if len(notes) != 0 {
		t.Errorf("expected note dropped, got %d notes", len(notes))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "missing or empty content") {
		t.Errorf("expected content warning, got: %v", warnings)
	}
}

func TestParseExtractionOutput_NilTargetKBs(t *testing.T) {
	input := `[{"title":"Note","content":"Body"}]`
	notes, warnings := ParseExtractionOutput(input)
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].SuggestedTargetKBs != nil {
		t.Errorf("expected nil target KBs, got %v", notes[0].SuggestedTargetKBs)
	}
}

func TestParseExtractionOutput_InvalidTargetKBsType(t *testing.T) {
	input := `[{"title":"Note","content":"Body","suggested_target_kbs":"not-an-array"}]`
	notes, warnings := ParseExtractionOutput(input)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note (with warning), got %d", len(notes))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "not an array") {
		t.Errorf("expected array warning, got: %v", warnings)
	}
}

func TestParseExtractionOutput_MultipleNotes_PartialAcceptance(t *testing.T) {
	input := `[
		{"title":"Good","content":"Valid note","suggested_target_kbs":[]},
		{"title":"","content":"Missing title","suggested_target_kbs":[]},
		{"title":"Also Good","content":"Another valid note","suggested_target_kbs":["remote/team"]}
	]`
	notes, warnings := ParseExtractionOutput(input)
	if len(notes) != 2 {
		t.Fatalf("expected 2 valid notes (partial acceptance), got %d", len(notes))
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning for dropped note, got %d: %v", len(warnings), warnings)
	}
}

func TestParseExtractionOutput_WhitespaceTitle(t *testing.T) {
	input := `[{"title":"  ","content":"Body","suggested_target_kbs":[]}]`
	notes, warnings := ParseExtractionOutput(input)
	if len(notes) != 0 {
		t.Errorf("expected whitespace-only title to be dropped, got %d notes", len(notes))
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(warnings))
	}
}
