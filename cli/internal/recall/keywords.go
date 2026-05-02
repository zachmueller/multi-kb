package recall

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/zmueller/multi-kb/internal/bedrock"
	"github.com/zmueller/multi-kb/internal/recall/prompts"
)

// DeriveKeywords calls the summarization LLM to derive 3-5 search keywords from a user query.
// Falls back to mechanical keyword extraction if the LLM call fails.
func DeriveKeywords(ctx context.Context, client *bedrock.Client, query string) ([]string, error) {
	text, err := client.InvokeModel(ctx, prompts.KeywordDerivationPrompt, query)
	if err != nil {
		return MechanicalKeywords(query), nil
	}

	text = strings.TrimSpace(text)
	// Strip markdown code fences
	if strings.HasPrefix(text, "```") {
		lines := strings.SplitN(text, "\n", 2)
		if len(lines) == 2 {
			text = lines[1]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
		text = strings.TrimSpace(text)
	}

	var keywords []string
	if err := json.Unmarshal([]byte(text), &keywords); err != nil {
		return MechanicalKeywords(query), nil
	}

	// Filter empty strings
	var result []string
	for _, k := range keywords {
		if k = strings.TrimSpace(k); k != "" {
			result = append(result, k)
		}
	}
	if len(result) == 0 {
		return MechanicalKeywords(query), nil
	}
	return result, nil
}

// MechanicalKeywords extracts keywords by splitting on whitespace and removing stop words.
func MechanicalKeywords(query string) []string {
	stopWords := map[string]bool{
		"a": true, "an": true, "the": true, "is": true, "are": true, "was": true,
		"were": true, "be": true, "been": true, "being": true, "have": true,
		"has": true, "had": true, "do": true, "does": true, "did": true,
		"will": true, "would": true, "shall": true, "should": true, "may": true,
		"might": true, "must": true, "can": true, "could": true, "of": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "with": true,
		"by": true, "from": true, "as": true, "into": true, "through": true,
		"during": true, "and": true, "or": true, "but": true, "if": true,
		"i": true, "you": true, "he": true, "she": true, "it": true, "we": true,
		"they": true, "this": true, "that": true, "these": true, "those": true,
		"how": true, "what": true, "when": true, "where": true, "why": true,
	}

	words := strings.FieldsFunc(query, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	seen := make(map[string]bool)
	var keywords []string
	for _, w := range words {
		lower := strings.ToLower(w)
		if !stopWords[lower] && len(w) > 2 && !seen[lower] {
			seen[lower] = true
			keywords = append(keywords, w)
			if len(keywords) >= 5 {
				break
			}
		}
	}

	if len(keywords) == 0 {
		if query != "" {
			return []string{query}
		}
		return nil
	}
	return keywords
}
