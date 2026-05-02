package extract

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Note is an extracted knowledge note from a conversation.
type Note struct {
	Title              string   `json:"title"`
	Content            string   `json:"content"`
	SuggestedTargetKBs []string `json:"suggested_target_kbs"`
}

type rawNote struct {
	Title              interface{} `json:"title"`
	Content            interface{} `json:"content"`
	SuggestedTargetKBs interface{} `json:"suggested_target_kbs"`
}

// ParseExtractionOutput parses the LLM's JSON array response into validated notes.
// Valid entries are accepted; invalid entries are logged and dropped (partial acceptance).
func ParseExtractionOutput(text string) ([]Note, []string) {
	text = strings.TrimSpace(text)

	// Strip markdown code fences if the LLM included them
	if strings.HasPrefix(text, "```") {
		lines := strings.SplitN(text, "\n", 2)
		if len(lines) == 2 {
			text = lines[1]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
		text = strings.TrimSpace(text)
	}

	var raw []rawNote
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, []string{fmt.Sprintf("failed to parse JSON array: %v", err)}
	}

	var notes []Note
	var warnings []string

	for i, r := range raw {
		title, ok := r.Title.(string)
		if !ok || strings.TrimSpace(title) == "" {
			warnings = append(warnings, fmt.Sprintf("note[%d]: missing or empty title — dropped", i))
			continue
		}
		if len(title) > 255 {
			warnings = append(warnings, fmt.Sprintf("note[%d]: title exceeds 255 chars — dropped", i))
			continue
		}

		content, ok := r.Content.(string)
		if !ok || strings.TrimSpace(content) == "" {
			warnings = append(warnings, fmt.Sprintf("note[%d] %q: missing or empty content — dropped", i, title))
			continue
		}
		if len(content) > 100_000 {
			warnings = append(warnings, fmt.Sprintf("note[%d] %q: content exceeds 100K chars — dropped", i, title))
			continue
		}

		var targetKBs []string
		if r.SuggestedTargetKBs != nil {
			arr, ok := r.SuggestedTargetKBs.([]interface{})
			if !ok {
				warnings = append(warnings, fmt.Sprintf("note[%d] %q: suggested_target_kbs is not an array — treating as empty", i, title))
			} else {
				for _, v := range arr {
					if s, ok := v.(string); ok && s != "" {
						targetKBs = append(targetKBs, s)
					}
				}
			}
		}

		notes = append(notes, Note{
			Title:              strings.TrimSpace(title),
			Content:            content,
			SuggestedTargetKBs: targetKBs,
		})
	}

	return notes, warnings
}
