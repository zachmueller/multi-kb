package dreamcycle

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zmueller/multi-kb/internal/dreamcycle/prompts"
)

// llmInvoker is satisfied by *bedrock.Client and any test double.
type llmInvoker interface {
	InvokeModel(ctx context.Context, systemPrompt, userMessage string) (string, error)
}

// consolidationOutput is the expected LLM response structure.
type consolidationOutput struct {
	Actions []consolidationAction `json:"actions"`
}

type consolidationAction struct {
	Type       string `json:"type"`
	SourceUID  string `json:"source_uid,omitempty"`
	TargetUID  string `json:"target_uid,omitempty"`
	SourceUIDs []string `json:"source_uids,omitempty"`
	Reason     string `json:"reason"`

	// merge fields
	MergedContent string `json:"merged_content,omitempty"`
	MergedTitle   string `json:"merged_title,omitempty"`

	// split fields
	NewNotes []newNoteSpec `json:"new_notes,omitempty"`

	// consolidate fields
	ConsolidatedNote *newNoteSpec `json:"consolidated_note,omitempty"`
}

type newNoteSpec struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// ConsolidateBatch sends a batch to the LLM, parses the response, and applies actions.
// Returns a map of action type → count, or an error.
func ConsolidateBatch(ctx context.Context, client llmInvoker, store NoteStore, batch Batch) (map[string]int, error) {
	userMessage := buildConsolidationMessage(batch)

	response, err := client.InvokeModel(ctx, prompts.ConsolidationSystemPrompt, userMessage)
	if err != nil {
		return nil, fmt.Errorf("phase3: LLM call failed: %w", err)
	}

	output, err := parseConsolidationOutput(response)
	if err != nil {
		return nil, fmt.Errorf("phase3: parse response: %w", err)
	}

	// Validate: every pending note UID must appear in exactly one action
	pendingUIDs := map[string]bool{batch.PendingNote.UID: true}
	coveredUIDs := make(map[string]bool)
	for _, action := range output.Actions {
		switch action.Type {
		case "keep", "merge", "split":
			coveredUIDs[action.SourceUID] = true
		case "consolidate":
			for _, uid := range action.SourceUIDs {
				if pendingUIDs[uid] {
					coveredUIDs[uid] = true
				}
			}
		}
	}

	for uid := range pendingUIDs {
		if !coveredUIDs[uid] {
			return nil, fmt.Errorf("phase3: pending note %q not addressed by any action", uid)
		}
	}

	actionCounts, err := ApplyActions(store, output.Actions, batch)
	if err != nil {
		return actionCounts, fmt.Errorf("phase3: apply actions: %w", err)
	}

	// Commit applied actions
	commitMsg := formatCommitMessage(actionCounts)
	if err := store.CommitBatch(commitMsg); err != nil {
		return actionCounts, fmt.Errorf("phase3: commit: %w", err)
	}

	return actionCounts, nil
}

func buildConsolidationMessage(batch Batch) string {
	var sb strings.Builder

	sb.WriteString("## Pending Notes (to evaluate)\n\n")
	sb.WriteString(formatNote(batch.PendingNote))

	if len(batch.RelatedNotes) > 0 {
		sb.WriteString("## Related Active Notes (for context)\n\n")
		for _, note := range batch.RelatedNotes {
			sb.WriteString(formatNote(note))
		}
	}

	return sb.String()
}

func formatNote(note Note) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Note: %s\n", note.UID))
	sb.WriteString(fmt.Sprintf("**Title:** %s\n", note.Title))
	if note.Author != "" {
		sb.WriteString(fmt.Sprintf("**Author:** %s\n", note.Author))
	}
	sb.WriteString(note.Content)
	sb.WriteString("\n\n")
	return sb.String()
}

func parseConsolidationOutput(response string) (*consolidationOutput, error) {
	response = strings.TrimSpace(response)

	var output consolidationOutput

	// Strategy 1: entire response is valid JSON
	if err := json.Unmarshal([]byte(response), &output); err == nil && len(output.Actions) > 0 {
		return &output, nil
	}

	// Strategy 2: extract JSON from markdown code fence anywhere in the response
	for _, fence := range []string{"```json", "```\n"} {
		if idx := strings.Index(response, fence); idx >= 0 {
			inner := response[idx+len(fence):]
			if end := strings.Index(inner, "```"); end >= 0 {
				inner = strings.TrimSpace(inner[:end])
				if err := json.Unmarshal([]byte(inner), &output); err == nil && len(output.Actions) > 0 {
					return &output, nil
				}
			}
		}
	}

	// Strategy 3: find outermost JSON object via first '{' and last '}'
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start >= 0 && end > start {
		candidate := response[start : end+1]
		if err := json.Unmarshal([]byte(candidate), &output); err == nil && len(output.Actions) > 0 {
			return &output, nil
		}
	}

	return nil, fmt.Errorf("invalid JSON: could not extract actions from response (len=%d, first 200 chars: %s)",
		len(response), truncateStr(response, 200))
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func formatCommitMessage(actionCounts map[string]int) string {
	return fmt.Sprintf("dream-cycle: %d actions applied (%dK/%dM/%dS/%dC)",
		actionCounts["keep"]+actionCounts["merge"]+actionCounts["split"]+actionCounts["consolidate"],
		actionCounts["keep"], actionCounts["merge"], actionCounts["split"], actionCounts["consolidate"])
}
