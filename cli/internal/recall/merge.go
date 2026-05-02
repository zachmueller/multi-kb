package recall

// MergedResult is a recall result from any KB source (local or remote).
type MergedResult struct {
	UID      string
	Title    string
	Content  string
	Score    float64 // 0.0 for local results (match-count based)
	SourceKB string  // name of the KB this result came from; set by callers
}

// InterleaveResults merges ranked result lists from multiple KBs via round-robin interleaving.
// Results are interleaved by rank: top from each KB, then second from each, etc.
// Deduplicates by UID and truncates to maxResults.
func InterleaveResults(lists [][]MergedResult, maxResults int) []MergedResult {
	if len(lists) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var result []MergedResult

	maxLen := 0
	for _, l := range lists {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}

	for i := 0; i < maxLen && len(result) < maxResults; i++ {
		for _, list := range lists {
			if i >= len(list) {
				continue
			}
			r := list[i]
			if seen[r.UID] {
				continue
			}
			seen[r.UID] = true
			result = append(result, r)
			if len(result) >= maxResults {
				break
			}
		}
	}

	return result
}

// LocalResultsToMerged converts local recall results to MergedResult.
func LocalResultsToMerged(results []LocalResult) []MergedResult {
	merged := make([]MergedResult, len(results))
	for i, r := range results {
		merged[i] = MergedResult{
			UID:     r.UID,
			Title:   r.Title,
			Content: r.Content,
		}
	}
	return merged
}

// RemoteResultsToMerged converts remote recall results to MergedResult.
func RemoteResultsToMerged(results []RemoteResult) []MergedResult {
	merged := make([]MergedResult, len(results))
	for i, r := range results {
		merged[i] = MergedResult{
			UID:     r.UID,
			Title:   r.Title,
			Content: r.Content,
			Score:   r.Score,
		}
	}
	return merged
}
