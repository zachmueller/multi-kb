package translate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ProjectDirName converts a filesystem path to a Claude Code project directory name.
// Replaces all '/' with '-'. Example: /Volumes/foo → -Volumes-foo
func ProjectDirName(fsPath string) string {
	return strings.ReplaceAll(fsPath, "/", "-")
}

// ClaudeCodeTranslator translates Claude Code session files to intermediate format.
type ClaudeCodeTranslator struct {
	SessionsDir   string // ~/.claude/projects/<encoded-path>/
	LastProcessed time.Time
}

// NewClaudeCodeTranslator creates a translator for the given directory path.
// projectDir is the user's configured source directory (e.g. /Volumes/workplace/foo).
// lastProcessed is the per-directory last_processed timestamp from state.
func NewClaudeCodeTranslator(projectDir string, lastProcessed time.Time) (*ClaudeCodeTranslator, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("claude-code: cannot determine home directory: %w", err)
	}
	encoded := ProjectDirName(projectDir)
	sessionsDir := filepath.Join(home, ".claude", "projects", encoded)
	return &ClaudeCodeTranslator{
		SessionsDir:   sessionsDir,
		LastProcessed: lastProcessed,
	}, nil
}

// Discover returns all session file paths for this project.
func (t *ClaudeCodeTranslator) Discover() ([]string, error) {
	entries, err := os.ReadDir(t.SessionsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude-code: cannot read sessions dir %q: %w", t.SessionsDir, err)
	}

	var paths []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			paths = append(paths, filepath.Join(t.SessionsDir, e.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// TranslateSession translates a single session file.
func (t *ClaudeCodeTranslator) TranslateSession(sessionPath string) (*Conversation, error) {
	f, err := os.Open(sessionPath)
	if err != nil {
		return nil, fmt.Errorf("claude-code: cannot open %q: %w", sessionPath, err)
	}
	defer f.Close()

	// Raw JSONL lines — each is a JSON object
	var rawLines []map[string]json.RawMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(line, &obj); err != nil {
			continue // skip malformed lines
		}
		rawLines = append(rawLines, obj)
	}

	return t.buildConversation(sessionPath, rawLines)
}

// ccLine represents a decoded JSONL line from a Claude Code session.
type ccLine struct {
	Type      string          `json:"type"`
	UUID      string          `json:"uuid"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"sessionId"`
	CWD       string          `json:"cwd"`
	Message   json.RawMessage `json:"message"`

	// tool_result metadata
	ToolUseResult       json.RawMessage `json:"toolUseResult"`
	SourceToolAssistant string          `json:"sourceToolAssistantUUID"`
}

type ccMessage struct {
	ID        string          `json:"id"`
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	StopReason string         `json:"stop_reason"`
}

type ccContentBlock struct {
	Type       string          `json:"type"`
	Text       string          `json:"text"`
	Thinking   string          `json:"thinking"`
	ID         string          `json:"id"`   // tool_use id
	Name       string          `json:"name"` // tool_use name
	Input      json.RawMessage `json:"input"`
	ToolUseID  string          `json:"tool_use_id"` // tool_result
	Content    json.RawMessage `json:"content"`
	IsError    bool            `json:"is_error"`
}

func (t *ClaudeCodeTranslator) buildConversation(sessionPath string, rawLines []map[string]json.RawMessage) (*Conversation, error) {
	// Decode all lines
	lines := make([]ccLine, 0, len(rawLines))
	for _, raw := range rawLines {
		var line ccLine
		// Re-encode back to JSON to unmarshal into struct
		b, _ := json.Marshal(raw)
		if err := json.Unmarshal(b, &line); err != nil {
			continue
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("claude-code: empty session file %q", sessionPath)
	}

	// Extract session metadata from first line
	sessionID := lines[0].SessionID
	cwd := lines[0].CWD
	var startedAt time.Time
	if ts := lines[0].Timestamp; ts != "" {
		if t2, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			startedAt = t2
		}
	}
	if startedAt.IsZero() {
		startedAt = time.Now()
	}

	header := NewHeader(sessionID, "claude-code", sessionPath, cwd, startedAt, nil, nil)

	// Build tool_result lookup: tool_use_id → ccContentBlock
	toolResults := make(map[string]ccContentBlock)
	for _, line := range lines {
		if line.Type != "user" {
			continue
		}
		var msg ccMessage
		if err := json.Unmarshal(line.Message, &msg); err != nil {
			continue
		}
		var blocks []ccContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			continue
		}
		for _, block := range blocks {
			if block.Type == "tool_result" && block.ToolUseID != "" {
				toolResults[block.ToolUseID] = block
			}
		}
	}

	// Group consecutive assistant lines by message.id
	type pendingAssistant struct {
		msgID     string
		timestamp string
		blocks    []ccContentBlock
	}
	var pending *pendingAssistant

	var messages []Message
	addAssistant := func(pa *pendingAssistant) {
		if pa == nil {
			return
		}
		ts, _ := time.Parse(time.RFC3339Nano, pa.timestamp)
		if ts.IsZero() {
			ts = time.Now()
		}
		prevProcessed := !ts.After(t.LastProcessed)

		var textParts []string
		var toolUses []ToolUse

		for _, block := range pa.blocks {
			switch block.Type {
			case "text":
				if block.Text != "" {
					textParts = append(textParts, block.Text)
				}
			case "tool_use":
				result := toolResults[block.ID]
				summary := summarizeToolUse(block.Name, block.Input, result)
				toolUses = append(toolUses, ToolUse{ToolName: block.Name, Summary: summary})
			// skip "thinking" blocks
			}
		}

		content := strings.Join(textParts, "\n")
		messages = append(messages, NewMessage("assistant", content, ts, prevProcessed, toolUses))
	}

	for _, line := range lines {
		switch line.Type {
		case "assistant":
			var msg ccMessage
			if err := json.Unmarshal(line.Message, &msg); err != nil {
				continue
			}
			var blocks []ccContentBlock
			if err := json.Unmarshal(msg.Content, &blocks); err != nil {
				continue
			}

			if pending != nil && pending.msgID != msg.ID {
				// Flush previous assistant group
				addAssistant(pending)
				pending = nil
			}
			if pending == nil {
				pending = &pendingAssistant{
					msgID:     msg.ID,
					timestamp: line.Timestamp,
				}
			}
			pending.blocks = append(pending.blocks, blocks...)

		case "user":
			// Flush any pending assistant group first
			addAssistant(pending)
			pending = nil

			var msg ccMessage
			if err := json.Unmarshal(line.Message, &msg); err != nil {
				continue
			}
			var blocks []ccContentBlock
			if err := json.Unmarshal(msg.Content, &blocks); err != nil {
				continue
			}

			// Skip if all blocks are tool_result (those are captured via toolResults map)
			allToolResults := true
			for _, block := range blocks {
				if block.Type != "tool_result" {
					allToolResults = false
					break
				}
			}
			if allToolResults {
				continue
			}

			// Flatten text blocks
			var textParts []string
			for _, block := range blocks {
				if block.Type == "text" && block.Text != "" {
					textParts = append(textParts, block.Text)
				}
			}
			content := strings.Join(textParts, "\n")
			if strings.TrimSpace(content) == "" {
				continue
			}

			ts, _ := time.Parse(time.RFC3339Nano, line.Timestamp)
			if ts.IsZero() {
				ts = time.Now()
			}
			prevProcessed := !ts.After(t.LastProcessed)

			messages = append(messages, NewMessage("user", content, ts, prevProcessed, nil))

		// Skip: queue-operation, permission-mode, file-history-snapshot, last-prompt, ai-title, attachment
		}
	}
	// Flush any remaining pending assistant group
	addAssistant(pending)

	return &Conversation{Header: header, Messages: messages}, nil
}

// summarizeToolUse creates a brief summary for a tool call/result pair.
func summarizeToolUse(toolName string, input json.RawMessage, result ccContentBlock) string {
	var inputMap map[string]json.RawMessage
	_ = json.Unmarshal(input, &inputMap)

	switch toolName {
	case "Read":
		if path, ok := inputMap["file_path"]; ok {
			var p string
			_ = json.Unmarshal(path, &p)
			return fmt.Sprintf("Read file %s", p)
		}
	case "Write":
		if path, ok := inputMap["file_path"]; ok {
			var p string
			_ = json.Unmarshal(path, &p)
			return fmt.Sprintf("Wrote file %s", p)
		}
	case "Edit", "MultiEdit":
		if path, ok := inputMap["file_path"]; ok {
			var p string
			_ = json.Unmarshal(path, &p)
			return fmt.Sprintf("Edited file %s", p)
		}
	case "Bash":
		if cmd, ok := inputMap["command"]; ok {
			var c string
			_ = json.Unmarshal(cmd, &c)
			if len(c) > 60 {
				c = c[:60] + "..."
			}
			status := "ok"
			if result.IsError {
				status = "error"
			}
			return fmt.Sprintf("Ran '%s' — %s", c, status)
		}
	case "Agent":
		return "Spawned sub-agent"
	}

	status := "ok"
	if result.IsError {
		status = "error"
	}
	return fmt.Sprintf("%s — %s", toolName, status)
}
