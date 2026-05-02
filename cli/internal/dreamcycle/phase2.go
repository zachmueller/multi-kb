package dreamcycle

import (
	"strings"

	"github.com/zmueller/multi-kb/internal/git"
)

// FindRelatedNotes finds up to 10 active notes related to the pending note in a batch
// via keyword-based git grep. Keywords are derived mechanically from the note's title.
func FindRelatedNotes(kbDir string, batch Batch) ([]Note, error) {
	keywords := deriveKeywordsFromTitle(batch.PendingNote.Title)
	if len(keywords) == 0 {
		return nil, nil
	}

	results, err := git.GrepNotes(kbDir, keywords)
	if err != nil {
		return nil, err
	}

	var notes []Note
	for _, r := range results {
		// Exclude the pending note itself from related notes
		if r.UID == batch.PendingNote.UID {
			continue
		}
		notes = append(notes, Note{
			UID:     r.UID,
			Title:   r.Title,
			Content: r.Content,
			Status:  "active",
		})
		if len(notes) >= 10 {
			break
		}
	}

	return notes, nil
}

// deriveKeywordsFromTitle mechanically extracts keywords from a note title.
// Splits on whitespace and common separators, removes stop words and short words.
func deriveKeywordsFromTitle(title string) []string {
	// Replace common separators with spaces
	title = strings.NewReplacer(
		"-", " ", "_", " ", "/", " ", ":", " ", ",", " ", ".", " ",
		"(", " ", ")", " ", "[", " ", "]", " ",
	).Replace(title)

	words := strings.Fields(strings.ToLower(title))
	var keywords []string
	for _, w := range words {
		if len(w) < 3 || isStopWord(w) {
			continue
		}
		keywords = append(keywords, w)
	}

	if len(keywords) > 5 {
		keywords = keywords[:5]
	}

	return keywords
}

func isStopWord(w string) bool {
	stops := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "from": true,
		"that": true, "this": true, "are": true, "was": true, "were": true,
		"been": true, "being": true, "have": true, "has": true, "had": true,
		"does": true, "did": true, "will": true, "would": true, "could": true,
		"should": true, "may": true, "might": true, "can": true, "not": true,
		"but": true, "about": true, "into": true, "through": true, "during": true,
		"before": true, "after": true, "above": true, "below": true, "between": true,
		"each": true, "how": true, "when": true, "where": true, "why": true,
		"what": true, "which": true, "who": true, "whom": true, "then": true,
		"than": true, "too": true, "very": true, "just": true, "also": true,
		"use": true, "using": true, "used": true,
	}
	return stops[w]
}
