package recall

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBasicFormat(t *testing.T) {
	results := []MergedResult{
		{UID: "1", Title: "First Note", Content: "First content", SourceKB: "kb1"},
		{UID: "2", Title: "Second Note", Content: "Second content", SourceKB: "kb1"},
	}

	out := FormatInjection(results, "fallback", 0)
	if !strings.Contains(out, "## Relevant Knowledge") {
		t.Error("expected '## Relevant Knowledge' header")
	}
	if !strings.Contains(out, "### First Note") {
		t.Error("expected '### First Note' heading")
	}
	if !strings.Contains(out, "### Second Note") {
		t.Error("expected '### Second Note' heading")
	}
	if !strings.Contains(out, "First content") {
		t.Error("expected first note content")
	}
	if !strings.Contains(out, "Second content") {
		t.Error("expected second note content")
	}
}

func TestSourceKBPerNote(t *testing.T) {
	results := []MergedResult{
		{UID: "1", Title: "Note A", Content: "Content A", SourceKB: "alpha"},
		{UID: "2", Title: "Note B", Content: "Content B", SourceKB: "beta"},
	}

	out := FormatInjection(results, "fallback", 0)
	if !strings.Contains(out, "*Source: alpha*") {
		t.Error("expected '*Source: alpha*' for first note")
	}
	if !strings.Contains(out, "*Source: beta*") {
		t.Error("expected '*Source: beta*' for second note")
	}
}

func TestFallbackKBName(t *testing.T) {
	results := []MergedResult{
		{UID: "1", Title: "Note", Content: "Content", SourceKB: ""},
	}

	out := FormatInjection(results, "fallback", 0)
	if !strings.Contains(out, "*Source: fallback*") {
		t.Errorf("expected '*Source: fallback*' when SourceKB is empty, got:\n%s", out)
	}
}

func TestPendingNotice(t *testing.T) {
	results := []MergedResult{
		{UID: "1", Title: "Note", Content: "Content", SourceKB: "kb1"},
	}

	out := FormatInjection(results, "kb1", 5)
	if !strings.Contains(out, "5 note(s) awaiting approval") {
		t.Errorf("expected pending notice with count 5, got:\n%s", out)
	}
}

func TestEmptyResults(t *testing.T) {
	out := FormatInjection(nil, "kb", 0)
	if out != "" {
		t.Errorf("expected empty string for no results and no pending, got %q", out)
	}
}

func TestClaudeCodeOutput(t *testing.T) {
	markdown := "## Relevant Knowledge\n\n### Note\nContent"

	out := FormatHookOutput(markdown, "claude-code")
	var parsed map[string]string
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got error: %v\noutput: %s", err, out)
	}
	if parsed["systemMessage"] != markdown {
		t.Errorf("systemMessage mismatch.\nexpected: %q\ngot:      %q", markdown, parsed["systemMessage"])
	}
}

func TestNotorOutput(t *testing.T) {
	markdown := "## Relevant Knowledge\n\n### Note\nContent"

	out := FormatHookOutput(markdown, "notor")
	if out != markdown {
		t.Errorf("expected raw markdown for notor harness.\nexpected: %q\ngot:      %q", markdown, out)
	}
}

func TestEmptyMarkdown(t *testing.T) {
	out := FormatHookOutput("", "claude-code")
	if out != "" {
		t.Errorf("expected empty string for empty markdown, got %q", out)
	}

	out = FormatHookOutput("", "notor")
	if out != "" {
		t.Errorf("expected empty string for empty markdown (notor), got %q", out)
	}
}
