package translate

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// mockSummarizer implements LLMSummarizer for testing.
type mockSummarizer struct {
	result string
	err    error
}

func (m *mockSummarizer) Summarize(ctx context.Context, sys, user string) (string, error) {
	return m.result, m.err
}

// longString returns a string of length n filled with 'a'.
func longString(n int) string {
	return strings.Repeat("a", n)
}

func TestSummarizeTool_ReadTemplate(t *testing.T) {
	got := SummarizeTool("Read", "/path/file.go", "contents...", nil)
	if !strings.HasPrefix(got, "Read file") {
		t.Errorf("expected result to start with 'Read file', got %q", got)
	}
}

func TestSummarizeTool_WriteTemplate(t *testing.T) {
	got := SummarizeTool("Write", "/path/file.go", "", nil)
	if !strings.HasPrefix(got, "Wrote file") {
		t.Errorf("expected result to start with 'Wrote file', got %q", got)
	}
}

func TestSummarizeTool_EditTemplate(t *testing.T) {
	got := SummarizeTool("Edit", "/path/file.go", "", nil)
	if !strings.HasPrefix(got, "Edited file") {
		t.Errorf("expected result to start with 'Edited file', got %q", got)
	}
}

func TestSummarizeTool_BashTemplate(t *testing.T) {
	got := SummarizeTool("Bash", "ls -la", "file1 file2", nil)
	if !strings.Contains(got, "Ran") {
		t.Errorf("expected result to contain 'Ran', got %q", got)
	}
	if !strings.Contains(got, "ls -la") {
		t.Errorf("expected result to contain command 'ls -la', got %q", got)
	}
}

func TestSummarizeTool_NilSummarizer_LargeInput(t *testing.T) {
	// Should fall back to mechanical summary without panicking.
	got := SummarizeTool("Unknown", longString(5000), longString(5000), nil)
	if got == "" {
		t.Error("expected non-empty result for large input with nil summarizer")
	}
}

func TestSummarizeTool_LLMSummarizerUsed(t *testing.T) {
	mock := &mockSummarizer{result: "LLM summary", err: nil}
	got := SummarizeTool("Unknown", longString(5000), longString(5000), mock)
	if got != "LLM summary" {
		t.Errorf("expected 'LLM summary', got %q", got)
	}
}

func TestSummarizeTool_LLMErrorFallback(t *testing.T) {
	mock := &mockSummarizer{result: "", err: errors.New("llm failure")}
	got := SummarizeTool("Unknown", longString(5000), longString(5000), mock)
	if got == "" {
		t.Error("expected non-empty mechanical fallback on LLM error")
	}
	if got == "LLM summary" {
		t.Error("expected mechanical fallback, not LLM result")
	}
}
