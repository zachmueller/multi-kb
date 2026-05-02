package recall

import (
	"github.com/zmueller/multi-kb/internal/git"
)

// LocalResult is a single recall result from the local KB.
type LocalResult struct {
	UID        string
	Title      string
	Content    string
	MatchCount int
}

// LocalRecall searches the local KB using git grep and returns results
// sorted by descending match count.
//
// keywords must not contain shell metacharacters (validated by git.GrepNotes).
func LocalRecall(repoDir string, keywords []string) ([]LocalResult, error) {
	grepResults, err := git.GrepNotes(repoDir, keywords)
	if err != nil {
		return nil, err
	}

	results := make([]LocalResult, len(grepResults))
	for i, r := range grepResults {
		results[i] = LocalResult{
			UID:        r.UID,
			Title:      r.Title,
			Content:    r.Content,
			MatchCount: r.MatchCount,
		}
	}
	return results, nil
}
