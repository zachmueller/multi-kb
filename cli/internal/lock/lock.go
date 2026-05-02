package lock

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	heartbeatInterval = 60 * time.Second
	staleTTL          = 30 * time.Minute
)

// ErrLockHeld is returned when the lock is actively held by another process.
var ErrLockHeld = errors.New("lock is held by another process")

type lockFile struct {
	PID       int    `json:"pid"`
	StartedAt string `json:"started_at"`
	Heartbeat string `json:"heartbeat"`
	Activity  string `json:"activity"`
}

// Lock represents an acquired lock.
type Lock struct {
	path     string
	stopCh   chan struct{}
	mu       sync.Mutex
	released bool
}

// DefaultLockPath returns the default lock file path.
func DefaultLockPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".multi-kb/lock"
	}
	return filepath.Join(home, ".multi-kb", "lock")
}

// Acquire attempts to acquire the lock at path for the given activity.
// Returns ErrLockHeld if another active process holds the lock.
func Acquire(path string, activity string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("lock: cannot create directory: %w", err)
	}

	// Check for existing lock
	if data, err := os.ReadFile(path); err == nil {
		var existing lockFile
		if json.Unmarshal(data, &existing) == nil {
			hb, err := time.Parse(time.RFC3339, existing.Heartbeat)
			if err == nil && time.Since(hb) < staleTTL {
				return nil, fmt.Errorf("%w: pid=%d activity=%q last_heartbeat=%s",
					ErrLockHeld, existing.PID, existing.Activity, existing.Heartbeat)
			}
			// stale — fall through and overwrite
		}
	}

	now := time.Now().UTC()
	lf := lockFile{
		PID:       os.Getpid(),
		StartedAt: now.Format(time.RFC3339),
		Heartbeat: now.Format(time.RFC3339),
		Activity:  activity,
	}

	if err := writeLockFile(path, lf); err != nil {
		return nil, fmt.Errorf("lock: cannot write lock file: %w", err)
	}

	l := &Lock{
		path:   path,
		stopCh: make(chan struct{}),
	}

	go l.heartbeatLoop()

	return l, nil
}

// Release releases the lock and stops the heartbeat goroutine.
func (l *Lock) Release() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.released {
		return nil
	}
	l.released = true
	close(l.stopCh)
	return os.Remove(l.path)
}

// IsHeld reports whether the lock file at path is actively held.
func IsHeld(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var lf lockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return false
	}
	hb, err := time.Parse(time.RFC3339, lf.Heartbeat)
	if err != nil {
		return false
	}
	return time.Since(hb) < staleTTL
}

func (l *Lock) heartbeatLoop() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l.stopCh:
			return
		case <-ticker.C:
			l.mu.Lock()
			if l.released {
				l.mu.Unlock()
				return
			}
			// Read-modify-write heartbeat
			data, err := os.ReadFile(l.path)
			if err == nil {
				var lf lockFile
				if json.Unmarshal(data, &lf) == nil {
					lf.Heartbeat = time.Now().UTC().Format(time.RFC3339)
					_ = writeLockFile(l.path, lf)
				}
			}
			l.mu.Unlock()
		}
	}
}

func writeLockFile(path string, lf lockFile) error {
	data, err := json.Marshal(lf)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
