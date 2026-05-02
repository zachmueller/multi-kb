package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestLoadState_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")

	ts := time.Now().UTC().Format(time.RFC3339)
	want := &State{
		Directories: map[string]DirectoryState{
			"/home/user/docs": {LastProcessed: ts},
		},
		LastDreamCycle: ts,
	}

	data, err := yaml.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if got.LastDreamCycle != want.LastDreamCycle {
		t.Errorf("LastDreamCycle = %q, want %q", got.LastDreamCycle, want.LastDreamCycle)
	}
	ds, ok := got.Directories["/home/user/docs"]
	if !ok {
		t.Fatal("expected directory /home/user/docs in state")
	}
	if ds.LastProcessed != ts {
		t.Errorf("LastProcessed = %q, want %q", ds.LastProcessed, ts)
	}
}

func TestLoadState_NonExistentPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "state.yaml")

	got, err := LoadState(path)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil state")
	}
	if got.Directories == nil {
		t.Fatal("expected initialized Directories map")
	}
	if len(got.Directories) != 0 {
		t.Errorf("expected empty Directories, got %d entries", len(got.Directories))
	}
}

func TestSaveState_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "state.yaml")

	ts := time.Now().UTC().Format(time.RFC3339)
	s := &State{
		Directories: map[string]DirectoryState{
			"/abs/path": {LastProcessed: ts},
		},
		LastDreamCycle: ts,
	}

	if err := SaveState(path, s); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// Verify file exists
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after save: %v", err)
	}
	if info.Size() == 0 {
		t.Error("saved file is empty")
	}

	// Reload and verify
	got, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState after save: %v", err)
	}
	if got.LastDreamCycle != ts {
		t.Errorf("LastDreamCycle = %q, want %q", got.LastDreamCycle, ts)
	}
	ds, ok := got.Directories["/abs/path"]
	if !ok {
		t.Fatal("expected directory /abs/path")
	}
	if ds.LastProcessed != ts {
		t.Errorf("LastProcessed = %q, want %q", ds.LastProcessed, ts)
	}
}

func TestLoadState_InvalidTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")

	// Manually write a state file with an invalid timestamp
	content := `directories:
  /some/abs/dir:
    last_processed: "not-a-date"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := LoadState(path)
	if err == nil {
		t.Fatal("expected error for invalid timestamp, got nil")
	}
	if got != nil {
		t.Error("expected nil state on error")
	}
	if !strings.Contains(err.Error(), "not valid ISO 8601") {
		t.Errorf("expected ISO 8601 error, got: %v", err)
	}
}

func TestLoadState_RelativePathKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")

	ts := time.Now().UTC().Format(time.RFC3339)
	// Write state file with a relative directory key
	content := "directories:\n  relative/path:\n    last_processed: \"" + ts + "\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := LoadState(path)
	if err == nil {
		t.Fatal("expected error for relative path key, got nil")
	}
	if got != nil {
		t.Error("expected nil state on error")
	}
	if !strings.Contains(err.Error(), "must be absolute") {
		t.Errorf("expected absolute-path error, got: %v", err)
	}
}

func TestSaveState_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")

	ts1 := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	ts2 := time.Now().UTC().Format(time.RFC3339)

	original := &State{
		Directories: map[string]DirectoryState{
			"/first/dir":  {LastProcessed: ts1},
			"/second/dir": {LastProcessed: ts2},
		},
		LastDreamCycle: ts2,
	}

	if err := SaveState(path, original); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if len(loaded.Directories) != 2 {
		t.Fatalf("expected 2 directories, got %d", len(loaded.Directories))
	}
	for key, want := range original.Directories {
		got, ok := loaded.Directories[key]
		if !ok {
			t.Errorf("missing directory %q", key)
			continue
		}
		if got.LastProcessed != want.LastProcessed {
			t.Errorf("directories[%q].last_processed = %q, want %q", key, got.LastProcessed, want.LastProcessed)
		}
	}
	if loaded.LastDreamCycle != original.LastDreamCycle {
		t.Errorf("LastDreamCycle = %q, want %q", loaded.LastDreamCycle, original.LastDreamCycle)
	}
}

func TestSaveState_NoTempFileLeftBehind(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "nested")
	path := filepath.Join(subdir, "state.yaml")

	s := &State{
		Directories: map[string]DirectoryState{},
	}

	if err := SaveState(path, s); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// List files in the directory; should only be state.yaml
	entries, err := os.ReadDir(subdir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "state.yaml" {
			t.Errorf("unexpected leftover file: %s", e.Name())
		}
	}
}
