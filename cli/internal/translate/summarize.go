package translate

import (
	"context"
	"fmt"

	"github.com/zmueller/multi-kb/internal/token"
)

const smallInteractionThreshold = 1000 // ~1K tokens

// LLMSummarizer is the interface for LLM-based summarization (implemented by bedrock.Client).
type LLMSummarizer interface {
	Summarize(ctx context.Context, systemPrompt, userContent string) (string, error)
}

// SummarizeTool produces a summary string for a tool name + input + result.
//
// Small interactions (<~1K tokens): uses a mechanical template.
// Large interactions (≥~1K tokens): delegates to the LLM summarizer if provided.
// If summarizer is nil, falls back to a truncated template regardless of size.
func SummarizeTool(toolName, input, result string, summarizer LLMSummarizer) string {
	combined := toolName + input + result
	if token.EstimateTokens(combined) < smallInteractionThreshold || summarizer == nil {
		return mechanicalSummary(toolName, input, result)
	}

	summary, err := summarizer.Summarize(
		context.Background(),
		"Summarize the following tool interaction in 1-2 sentences. Be concise and focus on what was done and the outcome.",
		fmt.Sprintf("Tool: %s\nInput: %s\nResult: %s", toolName, truncate(input, 500), truncate(result, 500)),
	)
	if err != nil {
		return mechanicalSummary(toolName, input, result)
	}
	return summary
}

// mechanicalSummary produces a template-based summary for common tools.
func mechanicalSummary(toolName, input, result string) string {
	switch toolName {
	case "Read":
		if len(input) > 0 {
			return fmt.Sprintf("Read file %s", truncate(input, 80))
		}
	case "Write":
		if len(input) > 0 {
			return fmt.Sprintf("Wrote file %s", truncate(input, 80))
		}
	case "Edit", "MultiEdit":
		if len(input) > 0 {
			return fmt.Sprintf("Edited file %s", truncate(input, 80))
		}
	case "Bash":
		resultSummary := truncate(result, 60)
		return fmt.Sprintf("Ran '%s' — %s", truncate(input, 60), resultSummary)
	}

	if len(result) > 0 {
		return fmt.Sprintf("%s — %s", toolName, truncate(result, 80))
	}
	return toolName
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
