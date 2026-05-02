package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultLogsDir returns the default logs directory.
func DefaultLogsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".multi-kb/logs"
	}
	return filepath.Join(home, ".multi-kb", "logs")
}

// RunEntry is a capture or dream-cycle run log entry.
type RunEntry struct {
	Timestamp              string         `json:"timestamp"`
	Type                   string         `json:"type"`    // "capture" | "dream_cycle"
	Trigger                string         `json:"trigger"` // "cron" | "manual"
	DirectoriesScanned     int            `json:"directories_scanned,omitempty"`
	ConversationsProcessed int            `json:"conversations_processed,omitempty"`
	NotesExtracted         int            `json:"notes_extracted,omitempty"`
	NotesRouted            map[string]int `json:"notes_routed,omitempty"`
	BatchesProcessed       int            `json:"batches_processed,omitempty"`
	Actions                map[string]int `json:"actions,omitempty"`
	Errors                 int            `json:"errors"`
	DurationMS             int64          `json:"duration_ms"`
}

// ExtractionErrorEntry is an extraction error log entry.
type ExtractionErrorEntry struct {
	Timestamp      string `json:"timestamp"`
	ConversationID string `json:"conversation_id"`
	SourcePath     string `json:"source_path"`
	Error          string `json:"error"`
	Retries        int    `json:"retries"`
}

// HookErrorEntry is a hook error log entry.
type HookErrorEntry struct {
	Timestamp      string `json:"timestamp"`
	Harness        string `json:"harness"`
	Directory      string `json:"directory"`
	Error          string `json:"error"`
	PartialResults bool   `json:"partial_results"`
}

// AppendRunLog appends a run entry to runs.jsonl.
func AppendRunLog(logsDir string, entry RunEntry) error {
	return appendJSONL(filepath.Join(logsDir, "runs.jsonl"), entry)
}

// AppendExtractionError appends an extraction error entry.
func AppendExtractionError(logsDir string, entry ExtractionErrorEntry) error {
	return appendJSONL(filepath.Join(logsDir, "extraction-errors.jsonl"), entry)
}

// AppendHookError appends a hook error entry.
func AppendHookError(logsDir string, entry HookErrorEntry) error {
	return appendJSONL(filepath.Join(logsDir, "hook-errors.jsonl"), entry)
}

// ReadRunLog reads the last n entries from runs.jsonl.
func ReadRunLog(logsDir string, n int) ([]RunEntry, error) {
	path := filepath.Join(logsDir, "runs.jsonl")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("logging: cannot read runs.jsonl: %w", err)
	}

	var entries []RunEntry
	dec := json.NewDecoder(jsonlReader(data))
	for dec.More() {
		var e RunEntry
		if err := dec.Decode(&e); err == nil {
			entries = append(entries, e)
		}
	}

	if n > 0 && len(entries) > n {
		entries = entries[len(entries)-n:]
	}
	return entries, nil
}

func appendJSONL(path string, entry any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("logging: cannot create logs directory: %w", err)
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("logging: cannot marshal entry: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("logging: cannot open %q: %w", path, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("logging: cannot write to %q: %w", path, err)
	}
	return nil
}

// jsonlReader wraps a byte slice as an io.Reader for json.Decoder.
type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, fmt.Errorf("EOF")
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func jsonlReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}
