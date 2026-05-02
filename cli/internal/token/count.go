package token

// ChunkingThreshold is intentionally below the ~800K spec target to provide a
// safety margin for the approximate token counter. Actual model context windows
// are larger.
const ChunkingThreshold = 700_000

// EstimateTokens approximates the token count for a string.
// Uses ~4 chars/token heuristic for English/prose text, slightly tighter for
// code-heavy content. Accuracy is within ±20% for typical conversation content.
func EstimateTokens(s string) int {
	if len(s) == 0 {
		return 0
	}

	// Count code blocks for adjusted estimate
	codeChars := countCodeChars(s)
	proseChars := len(s) - codeChars

	// Code is denser in tokens: ~3.5 chars/token
	// Prose: ~4 chars/token
	tokens := float64(proseChars)/4.0 + float64(codeChars)/3.5
	return int(tokens + 0.5)
}

// ExceedsChunkingThreshold reports whether the token count exceeds the threshold.
func ExceedsChunkingThreshold(s string) bool {
	return EstimateTokens(s) > ChunkingThreshold
}

// countCodeChars estimates how many characters are inside code blocks (```) for
// better per-token weighting.
func countCodeChars(s string) int {
	var count int
	inCode := false
	i := 0
	for i < len(s) {
		if i+2 < len(s) && s[i] == '`' && s[i+1] == '`' && s[i+2] == '`' {
			inCode = !inCode
			i += 3
			continue
		}
		if inCode {
			count++
		}
		i++
	}
	return count
}
