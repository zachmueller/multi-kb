package approve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/route"
)

func writePendingEntry(t *testing.T, dir, filename string, entry route.PendingEntry) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(entry)
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestHandleListNotes_Empty(t *testing.T) {
	dir := t.TempDir()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/notes", func(w http.ResponseWriter, r *http.Request) {
		handleListNotes(w, r, dir)
	})

	req := httptest.NewRequest("GET", "/api/notes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var notes []noteJSON
	if err := json.Unmarshal(w.Body.Bytes(), &notes); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(notes))
	}
}

func TestHandleListNotes_WithEntries(t *testing.T) {
	dir := t.TempDir()

	writePendingEntry(t, dir, "note1.json", route.PendingEntry{
		Title:   "Test Note",
		Content: "Body",
		Author:  "tester",
		TargetKBs: []string{"local/dev"},
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/notes", func(w http.ResponseWriter, r *http.Request) {
		handleListNotes(w, r, dir)
	})

	req := httptest.NewRequest("GET", "/api/notes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var notes []noteJSON
	if err := json.Unmarshal(w.Body.Bytes(), &notes); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Title != "Test Note" {
		t.Errorf("expected title %q, got %q", "Test Note", notes[0].Title)
	}
}

func TestHandleReject(t *testing.T) {
	dir := t.TempDir()

	writePendingEntry(t, dir, "note1.json", route.PendingEntry{
		Title:     "Test",
		Content:   "Body",
		Author:    "tester",
		TargetKBs: []string{"local/dev", "remote/team"},
	})

	mux := http.NewServeMux()
	cfg := &config.Config{}
	mux.HandleFunc("/api/notes/", func(w http.ResponseWriter, r *http.Request) {
		handleNoteAction(w, r, dir, cfg)
	})

	body := `{"target_kb":"local/dev"}`
	req := httptest.NewRequest("POST", "/api/notes/note1.json/reject", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp actionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(resp.RemainingTargets) != 1 || resp.RemainingTargets[0] != "remote/team" {
		t.Errorf("expected remaining [remote/team], got %v", resp.RemainingTargets)
	}
}

func TestHandleReject_InvalidTargetKB(t *testing.T) {
	dir := t.TempDir()

	writePendingEntry(t, dir, "note1.json", route.PendingEntry{
		Title:     "Test",
		Content:   "Body",
		Author:    "tester",
		TargetKBs: []string{"local/dev"},
	})

	mux := http.NewServeMux()
	cfg := &config.Config{}
	mux.HandleFunc("/api/notes/", func(w http.ResponseWriter, r *http.Request) {
		handleNoteAction(w, r, dir, cfg)
	})

	body := `{"target_kb":"nonexistent/kb"}`
	req := httptest.NewRequest("POST", "/api/notes/note1.json/reject", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for invalid target, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReject_MissingTargetKB(t *testing.T) {
	dir := t.TempDir()

	writePendingEntry(t, dir, "note1.json", route.PendingEntry{
		Title:     "Test",
		Content:   "Body",
		Author:    "tester",
		TargetKBs: []string{"local/dev"},
	})

	mux := http.NewServeMux()
	cfg := &config.Config{}
	mux.HandleFunc("/api/notes/", func(w http.ResponseWriter, r *http.Request) {
		handleNoteAction(w, r, dir, cfg)
	})

	body := `{"target_kb":""}`
	req := httptest.NewRequest("POST", "/api/notes/note1.json/reject", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for empty target_kb, got %d", w.Code)
	}
}

func TestHandleNoteAction_UnknownAction(t *testing.T) {
	dir := t.TempDir()

	writePendingEntry(t, dir, "note1.json", route.PendingEntry{
		Title:     "Test",
		Content:   "Body",
		Author:    "tester",
		TargetKBs: []string{"local/dev"},
	})

	mux := http.NewServeMux()
	cfg := &config.Config{}
	mux.HandleFunc("/api/notes/", func(w http.ResponseWriter, r *http.Request) {
		handleNoteAction(w, r, dir, cfg)
	})

	req := httptest.NewRequest("POST", "/api/notes/note1.json/unknown", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for unknown action, got %d", w.Code)
	}
}

func TestIsLocalKB(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"local/dev", true},
		{"local/", true},
		{"remote/team", false},
		{"team-kb", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLocalKB(tc.name); got != tc.want {
				t.Errorf("isLocalKB(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestContainsTarget(t *testing.T) {
	targets := []string{"local/dev", "remote/team"}
	if !containsTarget(targets, "local/dev") {
		t.Error("expected true for existing target")
	}
	if containsTarget(targets, "nonexistent") {
		t.Error("expected false for non-existing target")
	}
	if containsTarget(nil, "anything") {
		t.Error("expected false for nil targets")
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, 200, map[string]string{"key": "value"})

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, 404, "not found")

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	var resp errorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Error != "not found" {
		t.Errorf("expected error %q, got %q", "not found", resp.Error)
	}
}
