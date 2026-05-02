package translate

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewHeader(t *testing.T) {
	ts := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	persona := "dev"
	workflow := "review"
	h := NewHeader("conv-123", "claude-code", "/path/to/source.jsonl", "/home/project", ts, &persona, &workflow)

	if h.Type != "conversation" {
		t.Errorf("expected type 'conversation', got %q", h.Type)
	}
	if h.ID != "conv-123" {
		t.Errorf("expected id 'conv-123', got %q", h.ID)
	}
	if h.SourceHarness != "claude-code" {
		t.Errorf("expected source_harness 'claude-code', got %q", h.SourceHarness)
	}
	if h.SourcePath != "/path/to/source.jsonl" {
		t.Errorf("expected source_path '/path/to/source.jsonl', got %q", h.SourcePath)
	}
	if h.StartedAt != "2025-01-15T10:30:00Z" {
		t.Errorf("expected started_at '2025-01-15T10:30:00Z', got %q", h.StartedAt)
	}
	if h.Metadata.Persona == nil || *h.Metadata.Persona != "dev" {
		t.Errorf("expected persona 'dev', got %v", h.Metadata.Persona)
	}
	if h.Metadata.Workflow == nil || *h.Metadata.Workflow != "review" {
		t.Errorf("expected workflow 'review', got %v", h.Metadata.Workflow)
	}
	if h.Metadata.ProjectDir != "/home/project" {
		t.Errorf("expected project_dir '/home/project', got %q", h.Metadata.ProjectDir)
	}
}

func TestNewMessage(t *testing.T) {
	ts := time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)
	msg := NewMessage("user", "Hello, world!", ts, false, nil)

	if msg.Type != "message" {
		t.Errorf("expected type 'message', got %q", msg.Type)
	}
	if msg.Role != "user" {
		t.Errorf("expected role 'user', got %q", msg.Role)
	}
	if msg.Content != "Hello, world!" {
		t.Errorf("expected content 'Hello, world!', got %q", msg.Content)
	}
	if msg.PreviouslyProcessed {
		t.Error("expected previously_processed to be false")
	}
	if len(msg.ToolUses) != 0 {
		t.Errorf("expected empty tool_uses, got %d", len(msg.ToolUses))
	}
}

func TestConversation_SerializeToString(t *testing.T) {
	ts := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	conv := &Conversation{
		Header: NewHeader("conv-1", "claude-code", "/src.jsonl", "/project", ts, nil, nil),
		Messages: []Message{
			NewMessage("user", "What is Go?", ts, false, nil),
			NewMessage("assistant", "Go is a programming language.", ts.Add(time.Second), false, nil),
		},
	}

	result, err := conv.SerializeToString()
	if err != nil {
		t.Fatalf("SerializeToString() error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 JSONL lines, got %d", len(lines))
	}

	// Verify each line is valid JSON.
	for i, line := range lines {
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestConversation_RoundTrip(t *testing.T) {
	ts := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	persona := "tester"
	conv := &Conversation{
		Header: NewHeader("conv-rt", "harness", "/path", "/dir", ts, &persona, nil),
		Messages: []Message{
			NewMessage("user", "ping", ts, false, nil),
			NewMessage("assistant", "pong", ts.Add(time.Second), false, nil),
		},
	}

	serialized, err := conv.SerializeToString()
	if err != nil {
		t.Fatalf("SerializeToString() error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(serialized), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	// Parse header back.
	var header ConversationHeader
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("failed to unmarshal header: %v", err)
	}
	if header.Type != "conversation" {
		t.Errorf("header type = %q, want 'conversation'", header.Type)
	}
	if header.ID != "conv-rt" {
		t.Errorf("header id = %q, want 'conv-rt'", header.ID)
	}
	if header.Metadata.Persona == nil || *header.Metadata.Persona != "tester" {
		t.Errorf("header persona = %v, want 'tester'", header.Metadata.Persona)
	}

	// Parse messages back.
	var msg1 Message
	if err := json.Unmarshal([]byte(lines[1]), &msg1); err != nil {
		t.Fatalf("failed to unmarshal message 1: %v", err)
	}
	if msg1.Role != "user" || msg1.Content != "ping" {
		t.Errorf("message 1: role=%q content=%q, want user/ping", msg1.Role, msg1.Content)
	}

	var msg2 Message
	if err := json.Unmarshal([]byte(lines[2]), &msg2); err != nil {
		t.Fatalf("failed to unmarshal message 2: %v", err)
	}
	if msg2.Role != "assistant" || msg2.Content != "pong" {
		t.Errorf("message 2: role=%q content=%q, want assistant/pong", msg2.Role, msg2.Content)
	}
}

func TestMessage_WithToolUses(t *testing.T) {
	ts := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	tools := []ToolUse{
		{ToolName: "Read", Summary: "Read file main.go"},
		{ToolName: "Bash", Summary: "Ran 'go test'"},
	}
	msg := NewMessage("assistant", "I read the file and ran tests.", ts, false, tools)

	conv := &Conversation{
		Header:   NewHeader("conv-tools", "harness", "/p", "/d", ts, nil, nil),
		Messages: []Message{msg},
	}

	serialized, err := conv.SerializeToString()
	if err != nil {
		t.Fatalf("SerializeToString() error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(serialized), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var parsed Message
	if err := json.Unmarshal([]byte(lines[1]), &parsed); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}
	if len(parsed.ToolUses) != 2 {
		t.Fatalf("expected 2 tool_uses, got %d", len(parsed.ToolUses))
	}
	if parsed.ToolUses[0].ToolName != "Read" {
		t.Errorf("tool_uses[0].tool_name = %q, want 'Read'", parsed.ToolUses[0].ToolName)
	}
	if parsed.ToolUses[1].Summary != "Ran 'go test'" {
		t.Errorf("tool_uses[1].summary = %q, want \"Ran 'go test'\"", parsed.ToolUses[1].Summary)
	}
}

func TestMessage_PreviouslyProcessed(t *testing.T) {
	ts := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	msg := NewMessage("user", "old message", ts, true, nil)

	if !msg.PreviouslyProcessed {
		t.Error("expected previously_processed to be true")
	}

	// Verify it serializes correctly.
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var parsed Message
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if !parsed.PreviouslyProcessed {
		t.Error("expected previously_processed to be true after round-trip")
	}
}
