package recall

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormatInjection produces a Markdown string with notes formatted for injection.
// Empty results produce an empty string (silent — no "no results" message).
func FormatInjection(results []MergedResult, kbName string, pendingCount int) string {
	if len(results) == 0 && pendingCount == 0 {
		return ""
	}

	var sb strings.Builder
	if len(results) > 0 {
		sb.WriteString("## Relevant Knowledge\n\n")
		for _, r := range results {
			sb.WriteString(fmt.Sprintf("### %s\n", r.Title))
			if kbName != "" {
				sb.WriteString(fmt.Sprintf("*Source: %s*\n\n", kbName))
			}
			sb.WriteString(r.Content)
			sb.WriteString("\n\n")
		}
	}

	if pendingCount > 0 {
		sb.WriteString(fmt.Sprintf("---\n*%d note(s) awaiting approval — run `multi-kb approve` to review*\n", pendingCount))
	}

	return strings.TrimSpace(sb.String())
}

// FormatHookOutput wraps the injection Markdown in the harness-specific output format.
// For claude-code: wraps in JSON {"systemMessage": "..."}.
// For notor: returns raw Markdown (no wrapper).
func FormatHookOutput(markdown, harness string) string {
	if markdown == "" {
		return ""
	}

	switch harness {
	case "claude-code":
		out, _ := json.Marshal(map[string]string{"systemMessage": markdown})
		return string(out)
	default:
		// notor and any future harnesses: raw markdown
		return markdown
	}
}
