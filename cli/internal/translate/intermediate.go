package translate

import (
	"bytes"
	"encoding/json"
	"time"
)

// ConversationHeader is the first line of an intermediate JSONL document.
type ConversationHeader struct {
	Type         string               `json:"type"`         // always "conversation"
	ID           string               `json:"id"`
	SourceHarness string              `json:"source_harness"`
	SourcePath   string               `json:"source_path"`
	StartedAt    string               `json:"started_at"`
	Metadata     ConversationMetadata `json:"metadata"`
}

// ConversationMetadata carries optional harness-specific fields.
type ConversationMetadata struct {
	Persona    *string `json:"persona"`
	Workflow   *string `json:"workflow"`
	ProjectDir string  `json:"project_dir"`
}

// Message is a single turn in the conversation.
type Message struct {
	Type               string    `json:"type"`                // always "message"
	Role               string    `json:"role"`                // "user" | "assistant" | "system"
	Content            string    `json:"content"`
	Timestamp          string    `json:"timestamp"`
	PreviouslyProcessed bool     `json:"previously_processed"`
	ToolUses           []ToolUse `json:"tool_uses"`
}

// ToolUse represents a summarized tool call/result pair on an assistant message.
type ToolUse struct {
	ToolName string `json:"tool_name"`
	Summary  string `json:"summary"`
}

// Conversation holds the parsed intermediate representation.
type Conversation struct {
	Header   ConversationHeader
	Messages []Message
}

// Serialize writes the conversation as JSONL: one header line followed by
// one line per message. Returns the raw bytes.
func (c *Conversation) Serialize() ([]byte, error) {
	var buf bytes.Buffer

	headerLine, err := json.Marshal(c.Header)
	if err != nil {
		return nil, err
	}
	buf.Write(headerLine)
	buf.WriteByte('\n')

	for _, msg := range c.Messages {
		msgLine, err := json.Marshal(msg)
		if err != nil {
			return nil, err
		}
		buf.Write(msgLine)
		buf.WriteByte('\n')
	}

	return buf.Bytes(), nil
}

// SerializeToString returns the JSONL as a string (convenience for LLM calls).
func (c *Conversation) SerializeToString() (string, error) {
	b, err := c.Serialize()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// NewHeader constructs a ConversationHeader for the given harness.
func NewHeader(id, harness, sourcePath, projectDir string, startedAt time.Time, persona, workflow *string) ConversationHeader {
	return ConversationHeader{
		Type:          "conversation",
		ID:            id,
		SourceHarness: harness,
		SourcePath:    sourcePath,
		StartedAt:     startedAt.UTC().Format(time.RFC3339),
		Metadata: ConversationMetadata{
			Persona:    persona,
			Workflow:   workflow,
			ProjectDir: projectDir,
		},
	}
}

// NewMessage constructs a Message.
func NewMessage(role, content string, ts time.Time, previouslyProcessed bool, toolUses []ToolUse) Message {
	if toolUses == nil {
		toolUses = []ToolUse{}
	}
	return Message{
		Type:                "message",
		Role:                role,
		Content:             content,
		Timestamp:           ts.UTC().Format(time.RFC3339Nano),
		PreviouslyProcessed: previouslyProcessed,
		ToolUses:            toolUses,
	}
}
