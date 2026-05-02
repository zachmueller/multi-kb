package extract

import (
	"context"
	"fmt"
	"strings"

	"github.com/zmueller/multi-kb/internal/bedrock"
	"github.com/zmueller/multi-kb/internal/extract/prompts"
	"github.com/zmueller/multi-kb/internal/logging"
	"github.com/zmueller/multi-kb/internal/token"
)

// Extractor performs knowledge extraction from translated conversations.
type Extractor struct {
	client         *bedrock.Client
	exclusionRules []string
	logDir         string
}

// NewExtractor creates an Extractor with the given Bedrock client, exclusion rules, and log directory.
func NewExtractor(client *bedrock.Client, exclusionRules []string, logDir string) *Extractor {
	return &Extractor{
		client:         client,
		exclusionRules: exclusionRules,
		logDir:         logDir,
	}
}

// Extract sends a translated conversation (JSONL string) to Bedrock for knowledge extraction.
// Returns validated notes and any parse warnings.
// Retries up to 3 times on API failure or malformed output before logging to extraction errors.
func (e *Extractor) Extract(ctx context.Context, conversationID, sourcePath, jsonlConversation string) ([]Note, error) {
	systemPrompt := BuildExtractionPrompt(e.exclusionRules)

	var notes []Note
	var lastErr error

	for attempt := 1; attempt <= 3; attempt++ {
		text, err := e.client.InvokeModel(ctx, systemPrompt, jsonlConversation)
		if err != nil {
			lastErr = err
			continue
		}

		parsed, warnings := ParseExtractionOutput(text)
		for _, w := range warnings {
			_ = logging.AppendExtractionError(e.logDir, logging.ExtractionErrorEntry{
				ConversationID: conversationID,
				SourcePath:     sourcePath,
				Error:          fmt.Sprintf("parse warning: %s", w),
				Retries:        attempt - 1,
			})
		}

		notes = parsed
		lastErr = nil
		break
	}

	if lastErr != nil {
		_ = logging.AppendExtractionError(e.logDir, logging.ExtractionErrorEntry{
			ConversationID: conversationID,
			SourcePath:     sourcePath,
			Error:          lastErr.Error(),
			Retries:        3,
		})
		return nil, fmt.Errorf("extract: conversation %q: %w", conversationID, lastErr)
	}

	return notes, nil
}

// ExtractChunked handles oversized conversations by splitting at message boundaries,
// processing each chunk, and carrying a rolling summary as context.
// Each processed chunk's summary replaces the previous (not accumulated) to keep context bounded.
func (e *Extractor) ExtractChunked(ctx context.Context, conversationID, sourcePath, jsonlConversation string) ([]Note, error) {
	if token.EstimateTokens(jsonlConversation) <= token.ChunkingThreshold {
		return e.Extract(ctx, conversationID, sourcePath, jsonlConversation)
	}

	lines := splitAtMessageBoundaries(jsonlConversation, token.ChunkingThreshold)
	if len(lines) <= 1 {
		return e.Extract(ctx, conversationID, sourcePath, jsonlConversation)
	}

	var allNotes []Note
	var prevSummary string

	for i, chunk := range lines {
		userMsg := chunk
		if prevSummary != "" {
			userMsg = fmt.Sprintf("## Context from previous conversation chunks\n\n%s\n\n## Conversation chunk %d of %d\n\n%s",
				prevSummary, i+1, len(lines), chunk)
		}

		notes, err := e.Extract(ctx, conversationID, sourcePath, userMsg)
		if err != nil {
			// Skip failed chunks but continue processing remaining
			continue
		}
		allNotes = append(allNotes, notes...)

		// Summarize this chunk to carry as context for the next one (latest summary only)
		if i < len(lines)-1 {
			summary, err := e.client.InvokeModel(ctx, prompts.ChunkSummarizationPrompt, chunk)
			if err != nil {
				// If summarization fails, carry no summary (better than aborting)
				prevSummary = ""
			} else {
				prevSummary = summary
			}
		}
	}

	return allNotes, nil
}

// splitAtMessageBoundaries splits the JSONL conversation into chunks that fit within the token
// threshold, always splitting at line (message) boundaries.
func splitAtMessageBoundaries(jsonl string, threshold int) []string {
	lines := strings.Split(strings.TrimSpace(jsonl), "\n")
	if len(lines) == 0 {
		return nil
	}

	var chunks []string
	var current strings.Builder
	currentTokens := 0

	for _, line := range lines {
		lineTokens := token.EstimateTokens(line)

		if currentTokens > 0 && currentTokens+lineTokens > threshold {
			chunks = append(chunks, current.String())
			current.Reset()
			currentTokens = 0
		}

		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		current.WriteString(line)
		currentTokens += lineTokens
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}
