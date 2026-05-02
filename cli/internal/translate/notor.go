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

// NotorTranslator translates Notor conversation files to intermediate format.
type NotorTranslator struct {
	HistoryDir    string
	LastProcessed time.Time
}

// NewNotorTranslator creates a translator for Notor conversations.
// vaultDir is the Obsidian vault root.
// lastProcessed is the per-directory last_processed timestamp from state.
func NewNotorTranslator(vaultDir string, lastProcessed time.Time) (*NotorTranslator, error) {
	// Discover history path from data.json
	historyDir, err := discoverNotorHistoryDir(vaultDir)
	if err != nil {
		return nil, err
	}
	return &NotorTranslator{
		HistoryDir:    historyDir,
		LastProcessed: lastProcessed,
	}, nil
}

func discoverNotorHistoryDir(vaultDir string) (string, error) {
	dataPath := filepath.Join(vaultDir, ".obsidian", "plugins", "notor", "data.json")
	data, err := os.ReadFile(dataPath)
	if err != nil {
		// Fall back to default
		return filepath.Join(vaultDir, ".obsidian", "plugins", "notor", "history"), nil
	}

	var config struct {
		HistoryPath string `json:"history_path"`
	}
	if err := json.Unmarshal(data, &config); err == nil && config.HistoryPath != "" {
		if filepath.IsAbs(config.HistoryPath) {
			return config.HistoryPath, nil
		}
		return filepath.Join(vaultDir, config.HistoryPath), nil
	}

	return filepath.Join(vaultDir, ".obsidian", "plugins", "notor", "history"), nil
}

// Discover returns all conversation file paths (excluding sub-agent files).
func (t *NotorTranslator) Discover() ([]string, error) {
	entries, err := os.ReadDir(t.HistoryDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("notor: cannot read history dir %q: %w", t.HistoryDir, err)
	}

	var paths []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if strings.Contains(e.Name(), "_subagent_") {
			continue
		}
		paths = append(paths, filepath.Join(t.HistoryDir, e.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}

// TranslateSession translates a single Notor conversation file.
func (t *NotorTranslator) TranslateSession(sessionPath string) (*Conversation, error) {
	f, err := os.Open(sessionPath)
	if err != nil {
		return nil, fmt.Errorf("notor: cannot open %q: %w", sessionPath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	var headerRaw []byte
	var msgRaws [][]byte

	lineNum := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		if lineNum == 0 {
			headerRaw = append([]byte{}, line...)
		} else {
			msgRaws = append(msgRaws, append([]byte{}, line...))
		}
		lineNum++
	}

	if len(headerRaw) == 0 {
		return nil, fmt.Errorf("notor: empty file %q", sessionPath)
	}

	return t.buildConversation(sessionPath, headerRaw, msgRaws)
}

// notorHeader is the first JSONL line.
type notorHeader struct {
	Type         string  `json:"_type"`
	ID           string  `json:"id"`
	CreatedAt    string  `json:"created_at"`
	PersonaName  *string `json:"persona_name"`
	WorkflowName *string `json:"workflow_name"`
	WorkflowPath *string `json:"workflow_path"`
	IsBackground bool    `json:"is_background"`
}

// notorMessage is a subsequent JSONL line.
type notorMessage struct {
	Type              string          `json:"_type"`
	ID                string          `json:"id"`
	Role              string          `json:"role"`
	Content           json.RawMessage `json:"content"`
	Timestamp         string          `json:"timestamp"`
	IsWorkflowMessage bool            `json:"is_workflow_message"`
	ToolCall          *notorToolCall  `json:"tool_call"`
	ToolResult        *notorToolResult `json:"tool_result"`
}

type notorToolCall struct {
	ID         string          `json:"id"`
	ToolName   string          `json:"tool_name"`
	Parameters json.RawMessage `json:"parameters"`
	Status     string          `json:"status"`
}

type notorToolResult struct {
	ToolName   string          `json:"tool_name"`
	Success    bool            `json:"success"`
	Result     json.RawMessage `json:"result"`
	Error      *string         `json:"error"`
	ToolCallID string          `json:"tool_call_id"`
}

func (t *NotorTranslator) buildConversation(sessionPath string, headerRaw []byte, msgRaws [][]byte) (*Conversation, error) {
	var hdr notorHeader
	if err := json.Unmarshal(headerRaw, &hdr); err != nil {
		return nil, fmt.Errorf("notor: cannot parse header in %q: %w", sessionPath, err)
	}

	var startedAt time.Time
	if hdr.CreatedAt != "" {
		if t2, err := time.Parse(time.RFC3339Nano, hdr.CreatedAt); err == nil {
			startedAt = t2
		}
	}
	if startedAt.IsZero() {
		startedAt = time.Now()
	}

	// Vault root is HistoryDir minus the suffix path
	vaultRoot := strings.TrimSuffix(t.HistoryDir, "/.obsidian/plugins/notor/history")
	vaultRoot = strings.TrimSuffix(vaultRoot, "/.obsidian/plugins/notor/history/")

	header := NewHeader(hdr.ID, "notor", sessionPath, vaultRoot, startedAt, hdr.PersonaName, hdr.WorkflowName)

	// Parse all messages
	var rawMsgs []notorMessage
	for _, raw := range msgRaws {
		var msg notorMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		rawMsgs = append(rawMsgs, msg)
	}

	// Build tool_result lookup: tool_call_id → notorToolResult
	toolResults := make(map[string]*notorToolResult)
	for _, msg := range rawMsgs {
		if msg.Role == "tool_result" && msg.ToolResult != nil {
			toolResults[msg.ToolResult.ToolCallID] = msg.ToolResult
		}
	}

	// Build messages — skip extension_block; collapse tool_call/tool_result into preceding assistant
	var messages []Message
	var pendingToolUses []ToolUse
	var lastRole string

	flushPendingToolUses := func() {
		if len(pendingToolUses) > 0 && len(messages) > 0 && messages[len(messages)-1].Role == "assistant" {
			messages[len(messages)-1].ToolUses = append(messages[len(messages)-1].ToolUses, pendingToolUses...)
			pendingToolUses = nil
		}
	}

	for _, msg := range rawMsgs {
		switch msg.Role {
		case "extension_block":
			continue

		case "system":
			// Skip compaction records
			var parsed map[string]interface{}
			contentStr := extractNotorContent(msg.Content)
			if err := json.Unmarshal([]byte(contentStr), &parsed); err == nil {
				if parsed["type"] == "compaction" {
					continue
				}
			}
			continue

		case "tool_call":
			// Pair with result and attach to preceding assistant
			if msg.ToolCall == nil {
				continue
			}
			result := toolResults[msg.ToolCall.ID]
			summary := summarizeNotorToolUse(msg.ToolCall, result)
			pendingToolUses = append(pendingToolUses, ToolUse{ToolName: msg.ToolCall.ToolName, Summary: summary})
			lastRole = "tool_call"
			continue

		case "tool_result":
			// Skip standalone — already handled via lookup
			lastRole = "tool_result"
			continue

		case "user", "assistant":
			// Flush any pending tool uses before a new user/assistant turn
			if lastRole != "tool_call" && lastRole != "tool_result" {
				flushPendingToolUses()
			} else if msg.Role == "user" {
				flushPendingToolUses()
			}

			ts, _ := time.Parse(time.RFC3339Nano, msg.Timestamp)
			if ts.IsZero() {
				ts = time.Now()
			}
			prevProcessed := !ts.After(t.LastProcessed)
			content := extractNotorContent(msg.Content)

			if msg.Role == "assistant" {
				// Attach pending tool uses as part of this assistant message
				message := NewMessage("assistant", content, ts, prevProcessed, pendingToolUses)
				pendingToolUses = nil
				messages = append(messages, message)
			} else {
				messages = append(messages, NewMessage("user", content, ts, prevProcessed, nil))
			}
			lastRole = msg.Role
		}
	}
	flushPendingToolUses()

	return &Conversation{Header: header, Messages: messages}, nil
}

// extractNotorContent extracts plain text from a Notor content field (string or ContentBlock[]).
func extractNotorContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try plain string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// Try ContentBlock array
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}

	return ""
}

func summarizeNotorToolUse(call *notorToolCall, result *notorToolResult) string {
	if call == nil {
		return "unknown tool"
	}
	status := "ok"
	if result != nil && !result.Success {
		status = "error"
		if result.Error != nil {
			status = fmt.Sprintf("error: %s", *result.Error)
		}
	}

	// Brief parameter summary
	var params map[string]json.RawMessage
	_ = json.Unmarshal(call.Parameters, &params)

	switch call.ToolName {
	case "search_vault":
		if q, ok := params["query"]; ok {
			var qs string
			_ = json.Unmarshal(q, &qs)
			return fmt.Sprintf("Searched vault for %q — %s", qs, status)
		}
	case "read_note":
		if p, ok := params["path"]; ok {
			var ps string
			_ = json.Unmarshal(p, &ps)
			return fmt.Sprintf("Read note %s — %s", ps, status)
		}
	case "create_note", "update_note":
		if p, ok := params["path"]; ok {
			var ps string
			_ = json.Unmarshal(p, &ps)
			return fmt.Sprintf("%s %s — %s", call.ToolName, ps, status)
		}
	}

	return fmt.Sprintf("%s — %s", call.ToolName, status)
}
