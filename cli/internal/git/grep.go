package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// GrepResult is a single note match with its metadata.
type GrepResult struct {
	UID        string
	Title      string
	Content    string
	MatchCount int
}

// noteFrontmatter is used to parse status and title from a note file.
type noteFrontmatter struct {
	UID    string `yaml:"uid"`
	Title  string `yaml:"title"`
	Status string `yaml:"status"`
}

// GrepNotes runs git grep against the repo for each keyword, counts matches per
// note (title lines weighted 3x), and returns active notes sorted by descending score.
//
// keywords must be pre-sanitized by the caller (no shell metacharacters).
func GrepNotes(repoDir string, keywords []string) ([]GrepResult, error) {
	if len(keywords) == 0 {
		return nil, nil
	}

	matchCounts := make(map[string]int) // uid → count

	for _, kw := range keywords {
		if err := validateKeyword(kw); err != nil {
			return nil, err
		}

		// git grep -c <keyword> -- '*.md' prints "filename:count" lines
		cmd := exec.Command("git", "grep", "-c", "--", kw, "*.md")
		cmd.Dir = repoDir
		out, err := cmd.Output()
		if err != nil {
			// exit code 1 = no matches (not an error)
			if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
				continue
			}
			return nil, fmt.Errorf("git grep: %w", err)
		}

		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			line := scanner.Text()
			// format: "filename:count"
			colon := strings.LastIndex(line, ":")
			if colon < 0 {
				continue
			}
			filename := line[:colon]
			count, err := strconv.Atoi(line[colon+1:])
			if err != nil {
				continue
			}
			uid := strings.TrimSuffix(filepath.Base(filename), ".md")
			matchCounts[uid] += count
		}

		// Extra weight for title matches: grep title line specifically
		cmd2 := exec.Command("git", "grep", "-c", "--", kw, "*.md")
		cmd2.Dir = repoDir
		// Re-run with -F for exact match on title lines (title: <value>)
		// Actually we need to grep the title: line specifically
		// Use a fixed pattern against the title: frontmatter line
		titlePattern := fmt.Sprintf("^title:.*%s", kw)
		cmd3 := exec.Command("git", "grep", "-c", "-i", "-E", titlePattern, "--", "*.md")
		cmd3.Dir = repoDir
		out3, err3 := cmd3.Output()
		if err3 == nil {
			scanner3 := bufio.NewScanner(strings.NewReader(string(out3)))
			for scanner3.Scan() {
				line := scanner3.Text()
				colon := strings.LastIndex(line, ":")
				if colon < 0 {
					continue
				}
				filename := line[:colon]
				count, err := strconv.Atoi(line[colon+1:])
				if err != nil {
					continue
				}
				uid := strings.TrimSuffix(filepath.Base(filename), ".md")
				// Title matches weighted 3x total: already counted once in regular grep,
				// add 2 more here (3x - 1 = 2 extra).
				matchCounts[uid] += count * 2
			}
		}
	}

	if len(matchCounts) == 0 {
		return nil, nil
	}

	// Build results for active notes only
	var results []GrepResult
	for uid, count := range matchCounts {
		notePath := filepath.Join(repoDir, uid+".md")
		fm, content, err := readNoteFrontmatter(notePath)
		if err != nil {
			continue
		}
		if fm.Status != "active" {
			continue
		}
		results = append(results, GrepResult{
			UID:        uid,
			Title:      fm.Title,
			Content:    content,
			MatchCount: count,
		})
	}

	// Sort by descending match count
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].MatchCount > results[j-1].MatchCount; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	return results, nil
}

func readNoteFrontmatter(path string) (*noteFrontmatter, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}

	content := string(data)

	// Extract YAML frontmatter between --- delimiters
	if !strings.HasPrefix(content, "---\n") {
		return &noteFrontmatter{}, content, nil
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return &noteFrontmatter{}, content, nil
	}
	fmYAML := content[4 : end+4]
	body := content[end+9:] // skip "---\n" + fmYAML + "\n---\n"

	var fm noteFrontmatter
	if err := yaml.Unmarshal([]byte(fmYAML), &fm); err != nil {
		return &noteFrontmatter{}, content, nil
	}

	return &fm, strings.TrimSpace(body), nil
}

func validateKeyword(kw string) error {
	// Reject shell metacharacters in keywords (they come from LLM output)
	forbidden := "|;&$`><!\\\n\r"
	if strings.ContainsAny(kw, forbidden) {
		return fmt.Errorf("git: keyword contains forbidden characters: %q", kw)
	}
	return nil
}
