package approve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/route"
)

// safeBuffer is a goroutine-safe bytes.Buffer for use in tests.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// pollURL waits up to maxWait for the buffer to contain "Approval UI: http://",
// then extracts and returns the URL. Returns "" on timeout.
func pollURL(buf *safeBuffer, maxWait time.Duration) string {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		line := buf.String()
		if idx := strings.Index(line, "Approval UI: "); idx >= 0 {
			rest := line[idx+len("Approval UI: "):]
			if end := strings.IndexAny(rest, "\n\r"); end >= 0 {
				return strings.TrimSpace(rest[:end])
			}
			return strings.TrimSpace(rest)
		}
		time.Sleep(5 * time.Millisecond)
	}
	return ""
}

// TestRunServer_BindsAndReportsURL verifies that runServer binds to a random port
// and prints "Approval UI: http://127.0.0.1:<port>" before blocking.
func TestRunServer_BindsAndReportsURL(t *testing.T) {
	dir := t.TempDir()
	writePendingEntry(t, dir, "note1.json", route.PendingEntry{
		Title:     "Test",
		Content:   "Body",
		Author:    "tester",
		TargetKBs: []string{"local/dev"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	var out safeBuffer
	errCh := make(chan error, 1)

	go func() {
		errCh <- runServer(ctx, dir, &config.Config{}, 30*time.Second, &out, false)
	}()

	url := pollURL(&out, 2*time.Second)
	if url == "" || !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Errorf("expected URL in output, got: %q (full output: %q)", url, out.String())
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

// TestRunServer_IdleTimeout verifies that the server shuts down after the idle
// timeout fires with no HTTP activity.
func TestRunServer_IdleTimeout(t *testing.T) {
	dir := t.TempDir()
	writePendingEntry(t, dir, "note1.json", route.PendingEntry{
		Title:     "Test",
		Content:   "Body",
		Author:    "tester",
		TargetKBs: []string{"local/dev"},
	})

	ctx := context.Background()
	var out safeBuffer
	start := time.Now()

	err := runServer(ctx, dir, &config.Config{}, 50*time.Millisecond, &out, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Errorf("server took too long to shut down via idle timeout: %v", elapsed)
	}

	if !strings.Contains(out.String(), "idle timeout") {
		t.Errorf("expected 'idle timeout' in output, got: %q", out.String())
	}
}

// TestRunServer_AllResolved verifies that after a POST action resolves all pending
// notes, the server shuts down automatically.
func TestRunServer_AllResolved(t *testing.T) {
	dir := t.TempDir()
	writePendingEntry(t, dir, "note1.json", route.PendingEntry{
		Title:     "Test",
		Content:   "Body",
		Author:    "tester",
		TargetKBs: []string{"local/dev"},
	})

	ctx := context.Background()
	var out safeBuffer
	errCh := make(chan error, 1)

	go func() {
		errCh <- runServer(ctx, dir, &config.Config{}, 30*time.Second, &out, false)
	}()

	serverURL := pollURL(&out, 2*time.Second)
	if serverURL == "" {
		t.Fatal("server did not report URL in time")
	}

	// POST a reject to resolve the only pending note.
	resp, err := http.Post(serverURL+"/api/notes/note1.json/reject", "application/json",
		strings.NewReader(`{"target_kb":"local/dev"}`))
	if err != nil {
		t.Fatalf("POST reject failed: %v", err)
	}
	resp.Body.Close()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down after all notes resolved")
	}

	if !strings.Contains(out.String(), "all notes resolved") {
		t.Errorf("expected 'all notes resolved' in output, got: %q", out.String())
	}
}

// TestRunServer_ContextCancel verifies that cancelling the context shuts the server down.
func TestRunServer_ContextCancel(t *testing.T) {
	dir := t.TempDir()
	writePendingEntry(t, dir, "note1.json", route.PendingEntry{
		Title:     "Test",
		Content:   "Body",
		Author:    "tester",
		TargetKBs: []string{"local/dev"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	var out safeBuffer
	errCh := make(chan error, 1)

	go func() {
		errCh <- runServer(ctx, dir, &config.Config{}, 30*time.Second, &out, false)
	}()

	url := pollURL(&out, 2*time.Second)
	if url == "" {
		t.Fatal("server did not start in time")
	}
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("unexpected error after context cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down after context cancel")
	}

	if !strings.Contains(out.String(), "interrupt") {
		t.Errorf("expected 'interrupt' in output, got: %q", out.String())
	}
}

// TestRunServer_ServesAPIEndpoint verifies that GET /api/notes returns pending notes.
func TestRunServer_ServesAPIEndpoint(t *testing.T) {
	dir := t.TempDir()
	writePendingEntry(t, dir, "note1.json", route.PendingEntry{
		Title:     "Test",
		Content:   "Body",
		Author:    "tester",
		TargetKBs: []string{"local/dev"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out safeBuffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- runServer(ctx, dir, &config.Config{}, 30*time.Second, &out, false)
	}()

	serverURL := pollURL(&out, 2*time.Second)
	if serverURL == "" {
		t.Fatal("server did not start in time")
	}

	resp, err := http.Get(serverURL + "/api/notes")
	if err != nil {
		t.Fatalf("GET /api/notes failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var notes []noteJSON
	if err := json.NewDecoder(resp.Body).Decode(&notes); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(notes) != 1 {
		t.Errorf("expected 1 note, got %d", len(notes))
	}
}
