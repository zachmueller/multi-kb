package route

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PendingEntry represents a note awaiting approval.
type PendingEntry struct {
	Title              string   `json:"title"`
	Content            string   `json:"content"`
	Author             string   `json:"author"`
	TargetKBs          []string `json:"target_kbs"`
	SourceConversation string   `json:"source_conversation"`
	ExtractedAt        string   `json:"extracted_at"`
}

// DefaultPendingDir returns the default pending queue directory.
func DefaultPendingDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".multi-kb/pending"
	}
	return filepath.Join(home, ".multi-kb", "pending")
}

// PendingFilename builds the canonical filename for a pending entry.
func PendingFilename(extractedAt time.Time, title, content string) string {
	ts := extractedAt.UTC().Format("20060102T150405")
	h := sha256.Sum256([]byte(title + content))
	return fmt.Sprintf("%s-%x.json", ts, h[:4])
}

// CreatePending writes a new pending entry and returns the filename.
func CreatePending(dir string, entry PendingEntry) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("pending: cannot create directory: %w", err)
	}

	now, _ := time.Parse(time.RFC3339, entry.ExtractedAt)
	if now.IsZero() {
		now = time.Now().UTC()
		entry.ExtractedAt = now.Format(time.RFC3339)
	}

	filename := PendingFilename(now, entry.Title, entry.Content)
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return "", fmt.Errorf("pending: cannot marshal entry: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("pending: cannot write file: %w", err)
	}

	return filename, nil
}

// ListPending returns all pending filenames in the directory.
func ListPending(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pending: cannot list directory: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// ReadPending reads a pending entry by filename.
func ReadPending(dir, filename string) (*PendingEntry, error) {
	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		return nil, fmt.Errorf("pending: cannot read %q: %w", filename, err)
	}
	var entry PendingEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("pending: cannot parse %q: %w", filename, err)
	}
	return &entry, nil
}

// UpdatePending removes a target KB from target_kbs.
// If target_kbs becomes empty, the file is deleted.
func UpdatePending(dir, filename, removeTarget string) error {
	entry, err := ReadPending(dir, filename)
	if err != nil {
		return err
	}

	filtered := entry.TargetKBs[:0]
	for _, kb := range entry.TargetKBs {
		if kb != removeTarget {
			filtered = append(filtered, kb)
		}
	}
	entry.TargetKBs = filtered

	if len(entry.TargetKBs) == 0 {
		return DeletePending(dir, filename)
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("pending: cannot marshal updated entry: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, filename), data, 0o600)
}

// DeletePending removes a pending entry file.
func DeletePending(dir, filename string) error {
	return os.Remove(filepath.Join(dir, filename))
}

// PendingCount returns the number of pending entries.
func PendingCount(dir string) (int, error) {
	names, err := ListPending(dir)
	return len(names), err
}
