package server

import (
	"encoding/json"
	"testing"
)

func TestGenerateSidecar_WellFormedNote(t *testing.T) {
	content := []byte("---\nuid: ABC123\ntitle: Test Note\nstatus: pending\nauthor: tester\n---\n\nSome content.")
	data, err := generateSidecar("notes/ABC123.md", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sc sidecarFile
	if err := json.Unmarshal(data, &sc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	assertAttr := func(key, want string) {
		t.Helper()
		attr, ok := sc.MetadataAttributes[key]
		if !ok {
			t.Errorf("missing attribute %q", key)
			return
		}
		if attr.Value.Type != "STRING" {
			t.Errorf("attribute %q: expected type STRING, got %q", key, attr.Value.Type)
		}
		if attr.Value.StringValue != want {
			t.Errorf("attribute %q: expected %q, got %q", key, want, attr.Value.StringValue)
		}
	}

	assertAttr("uid", "ABC123")
	assertAttr("title", "Test Note")
	assertAttr("status", "pending")
	assertAttr("author", "tester")
}

func TestGenerateSidecar_MissingFrontmatter(t *testing.T) {
	content := []byte("Just plain text with no frontmatter.")
	data, err := generateSidecar("notes/NOFM.md", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sc sidecarFile
	if err := json.Unmarshal(data, &sc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// UID comes from filename
	if sc.MetadataAttributes["uid"].Value.StringValue != "NOFM" {
		t.Errorf("expected uid NOFM, got %q", sc.MetadataAttributes["uid"].Value.StringValue)
	}
	// Missing fields should be empty strings
	if sc.MetadataAttributes["title"].Value.StringValue != "" {
		t.Errorf("expected empty title, got %q", sc.MetadataAttributes["title"].Value.StringValue)
	}
	if sc.MetadataAttributes["status"].Value.StringValue != "" {
		t.Errorf("expected empty status, got %q", sc.MetadataAttributes["status"].Value.StringValue)
	}
	if sc.MetadataAttributes["author"].Value.StringValue != "" {
		t.Errorf("expected empty author, got %q", sc.MetadataAttributes["author"].Value.StringValue)
	}
}

func TestGenerateSidecar_PartialFrontmatter(t *testing.T) {
	content := []byte("---\nuid: PART1\ntitle: Only Title\n---\n\nContent here.")
	data, err := generateSidecar("PART1.md", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sc sidecarFile
	if err := json.Unmarshal(data, &sc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if sc.MetadataAttributes["uid"].Value.StringValue != "PART1" {
		t.Errorf("expected uid PART1, got %q", sc.MetadataAttributes["uid"].Value.StringValue)
	}
	if sc.MetadataAttributes["title"].Value.StringValue != "Only Title" {
		t.Errorf("expected title %q, got %q", "Only Title", sc.MetadataAttributes["title"].Value.StringValue)
	}
	if sc.MetadataAttributes["status"].Value.StringValue != "" {
		t.Errorf("expected empty status, got %q", sc.MetadataAttributes["status"].Value.StringValue)
	}
	if sc.MetadataAttributes["author"].Value.StringValue != "" {
		t.Errorf("expected empty author, got %q", sc.MetadataAttributes["author"].Value.StringValue)
	}
}

func TestGenerateSidecar_KeyFormat(t *testing.T) {
	content := []byte("---\nuid: KEY1\ntitle: Test\nstatus: active\nauthor: me\n---\n\nBody.")
	data, err := generateSidecar("notes/KEY1.md", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	attrs, ok := raw["metadataAttributes"].(map[string]interface{})
	if !ok {
		t.Fatal("expected metadataAttributes top-level key")
	}

	for _, key := range []string{"status", "uid", "title", "author"} {
		attr, ok := attrs[key].(map[string]interface{})
		if !ok {
			t.Errorf("missing attribute %q", key)
			continue
		}
		val, ok := attr["value"].(map[string]interface{})
		if !ok {
			t.Errorf("attribute %q missing value object", key)
			continue
		}
		if val["type"] != "STRING" {
			t.Errorf("attribute %q: expected type STRING, got %v", key, val["type"])
		}
	}
}
