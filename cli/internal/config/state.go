package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// DirectoryState tracks per-directory processing timestamps.
type DirectoryState struct {
	LastProcessed string `yaml:"last_processed"`
}

// State is the full state.yaml schema (machine-managed, never user-edited).
type State struct {
	Directories    map[string]DirectoryState `yaml:"directories"`
	LastDreamCycle string                    `yaml:"last_dream_cycle,omitempty"`
}

// DefaultStatePath returns the default state file path.
func DefaultStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".multi-kb/state.yaml"
	}
	return filepath.Join(home, ".multi-kb", "state.yaml")
}

// LoadState reads or initialises the state file.
func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &State{Directories: make(map[string]DirectoryState)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read state file %q: %w", path, err)
	}

	var s State
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("cannot parse state file %q: %w", path, err)
	}

	if s.Directories == nil {
		s.Directories = make(map[string]DirectoryState)
	}

	if err := validateState(&s); err != nil {
		return nil, err
	}

	return &s, nil
}

// SaveState atomically writes the state file (write temp → rename).
func SaveState(path string, s *State) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("cannot marshal state: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create state directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("cannot create temp state file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("cannot write temp state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("cannot close temp state file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("cannot atomically rename state file: %w", err)
	}

	return nil
}

func validateState(s *State) error {
	for dir, ds := range s.Directories {
		if !filepath.IsAbs(dir) {
			return fmt.Errorf("state: directory path %q must be absolute", dir)
		}
		if ds.LastProcessed != "" {
			if _, err := time.Parse(time.RFC3339, ds.LastProcessed); err != nil {
				return fmt.Errorf("state: directories[%q].last_processed %q is not valid ISO 8601", dir, ds.LastProcessed)
			}
		}
	}
	if s.LastDreamCycle != "" {
		if _, err := time.Parse(time.RFC3339, s.LastDreamCycle); err != nil {
			return fmt.Errorf("state: last_dream_cycle %q is not valid ISO 8601", s.LastDreamCycle)
		}
	}
	return nil
}
