package dreamcycle

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Batch represents a singleton batch of one pending note with its related active notes.
type Batch struct {
	PendingNote  Note
	RelatedNotes []Note
}

// Note represents a knowledge note read from the local KB.
type Note struct {
	UID     string
	Title   string
	Content string
	Author  string
	Status  string
}

type frontmatter struct {
	UID                  string   `yaml:"uid"`
	Title                string   `yaml:"title"`
	Status               string   `yaml:"status"`
	Author               string   `yaml:"author"`
	LastUpdated          string   `yaml:"last-updated"`
	LastLinkedTo         string   `yaml:"last-linked-to"`
	LastRecalled         string   `yaml:"last-recalled"`
	ConsolidatedFromNotes []string `yaml:"consolidated-from-notes"`
}

// CreateBatches scans the local KB for all pending notes and creates singleton batches.
func CreateBatches(kbDir string) ([]Batch, error) {
	entries, err := os.ReadDir(kbDir)
	if err != nil {
		return nil, err
	}

	var batches []Batch
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == ".gitkeep" {
			continue
		}

		note, err := readNote(filepath.Join(kbDir, e.Name()))
		if err != nil {
			continue
		}

		if note.Status == "pending" {
			batches = append(batches, Batch{PendingNote: *note})
		}
	}

	return batches, nil
}

// readNote reads a note file and parses its frontmatter and body.
func readNote(path string) (*Note, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	fm, body, err := parseFrontmatter(content)
	if err != nil {
		return nil, err
	}

	return &Note{
		UID:     fm.UID,
		Title:   fm.Title,
		Content: strings.TrimSpace(body),
		Author:  fm.Author,
		Status:  fm.Status,
	}, nil
}

// parseFrontmatter extracts YAML frontmatter and body from note content.
func parseFrontmatter(content string) (*frontmatter, string, error) {
	if !strings.HasPrefix(content, "---\n") {
		return &frontmatter{}, content, nil
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return &frontmatter{}, content, nil
	}
	fmYAML := content[4 : end+4]
	body := content[end+9:]

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(fmYAML), &fm); err != nil {
		return &frontmatter{}, content, nil
	}

	return &fm, body, nil
}
