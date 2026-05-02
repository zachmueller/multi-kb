package approve

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/zmueller/multi-kb/internal/approve/assets"
	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/git"
	"github.com/zmueller/multi-kb/internal/route"
	"github.com/zmueller/multi-kb/internal/submit"
)

// noteJSON is the JSON representation of a pending note returned by GET /api/notes.
type noteJSON struct {
	Filename           string   `json:"filename"`
	Title              string   `json:"title"`
	Content            string   `json:"content"`
	Author             string   `json:"author"`
	TargetKBs          []string `json:"target_kbs"`
	SourceConversation string   `json:"source_conversation"`
	ExtractedAt        string   `json:"extracted_at"`
}

// approveRequest is the JSON body for POST /api/notes/:filename/approve.
type approveRequest struct {
	TargetKB string `json:"target_kb"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

// rejectRequest is the JSON body for POST /api/notes/:filename/reject.
type rejectRequest struct {
	TargetKB string `json:"target_kb"`
}

// actionResponse is returned after approve/reject.
type actionResponse struct {
	RemainingTargets []string `json:"remaining_targets"`
}

// errorResponse is the JSON error body.
type errorResponse struct {
	Error string `json:"error"`
}

// registerRoutes sets up the HTTP routes on the given mux.
func registerRoutes(mux *http.ServeMux, pendingDir string, cfg *config.Config) {
	// Serve embedded assets
	assetFS := http.FileServer(http.FS(assets.Assets))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			data, err := assets.Assets.ReadFile("index.html")
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}
		// For /assets/styles.css etc, strip /assets/ prefix
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/assets")
			assetFS.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("/api/notes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		handleListNotes(w, r, pendingDir)
	})

	mux.HandleFunc("/api/notes/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		handleNoteAction(w, r, pendingDir, cfg)
	})
}

// handleListNotes returns all pending notes as JSON.
func handleListNotes(w http.ResponseWriter, r *http.Request, pendingDir string) {
	filenames, err := route.ListPending(pendingDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("cannot list pending: %v", err))
		return
	}

	notes := make([]noteJSON, 0, len(filenames))
	for _, fn := range filenames {
		entry, err := route.ReadPending(pendingDir, fn)
		if err != nil {
			continue // skip unreadable entries
		}
		notes = append(notes, noteJSON{
			Filename:           fn,
			Title:              entry.Title,
			Content:            entry.Content,
			Author:             entry.Author,
			TargetKBs:          entry.TargetKBs,
			SourceConversation: entry.SourceConversation,
			ExtractedAt:        entry.ExtractedAt,
		})
	}

	writeJSON(w, http.StatusOK, notes)
}

// handleNoteAction routes /api/notes/:filename/approve and /api/notes/:filename/reject.
func handleNoteAction(w http.ResponseWriter, r *http.Request, pendingDir string, cfg *config.Config) {
	// Parse path: /api/notes/{filename}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/notes/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		writeError(w, http.StatusBadRequest, "invalid path: expected /api/notes/{filename}/{action}")
		return
	}
	filename := parts[0]
	action := parts[1]

	switch action {
	case "approve":
		handleApprove(w, r, pendingDir, cfg, filename)
	case "reject":
		handleReject(w, r, pendingDir, filename)
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown action: %s", action))
	}
}

// handleApprove processes an approval for one target KB.
func handleApprove(w http.ResponseWriter, r *http.Request, pendingDir string, cfg *config.Config, filename string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read request body")
		return
	}

	var req approveRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if req.TargetKB == "" {
		writeError(w, http.StatusBadRequest, "target_kb is required")
		return
	}

	// Read the pending entry
	entry, err := route.ReadPending(pendingDir, filename)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("pending note not found: %s", filename))
		return
	}

	// Verify target_kb is in the entry's target list
	if !containsTarget(entry.TargetKBs, req.TargetKB) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("target %q not in pending targets", req.TargetKB))
		return
	}

	// Use edited title/content if provided, otherwise use originals
	title := req.Title
	if title == "" {
		title = entry.Title
	}
	content := req.Content
	if content == "" {
		content = entry.Content
	}

	// Determine if local or remote
	if isLocalKB(req.TargetKB) {
		err = submitToLocal(req.TargetKB, title, content, entry.Author)
	} else {
		err = submitToRemote(r.Context(), cfg, req.TargetKB, title, content, entry.Author)
	}

	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("submission failed: %v", err))
		return
	}

	// Remove target from pending entry
	if err := route.UpdatePending(pendingDir, filename, req.TargetKB); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("cannot update pending entry: %v", err))
		return
	}

	// Read remaining targets (file may have been deleted if none remain)
	remaining := remainingTargets(pendingDir, filename)
	writeJSON(w, http.StatusOK, actionResponse{RemainingTargets: remaining})
}

// handleReject processes a rejection for one target KB.
func handleReject(w http.ResponseWriter, r *http.Request, pendingDir string, filename string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read request body")
		return
	}

	var req rejectRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if req.TargetKB == "" {
		writeError(w, http.StatusBadRequest, "target_kb is required")
		return
	}

	// Read the pending entry
	entry, err := route.ReadPending(pendingDir, filename)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("pending note not found: %s", filename))
		return
	}

	// Verify target_kb is in the entry's target list
	if !containsTarget(entry.TargetKBs, req.TargetKB) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("target %q not in pending targets", req.TargetKB))
		return
	}

	// Remove target from pending entry
	if err := route.UpdatePending(pendingDir, filename, req.TargetKB); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("cannot update pending entry: %v", err))
		return
	}

	remaining := remainingTargets(pendingDir, filename)
	writeJSON(w, http.StatusOK, actionResponse{RemainingTargets: remaining})
}

// isLocalKB returns true if the KB name indicates a local repository.
func isLocalKB(name string) bool {
	return strings.HasPrefix(name, "local/")
}

// submitToLocal writes a note to the local KB git repo.
func submitToLocal(kbTarget, title, content, author string) error {
	// Extract the KB name after "local/"
	kbName := strings.TrimPrefix(kbTarget, "local/")

	kbDir, err := git.LocalKBDir(kbName)
	if err != nil {
		return fmt.Errorf("cannot resolve local KB directory: %w", err)
	}

	// Ensure the repo exists
	if err := git.InitRepo(kbDir); err != nil {
		return fmt.Errorf("cannot init local KB repo: %w", err)
	}

	_, err = submit.WriteNote(kbDir, submit.NoteFields{
		Title:   title,
		Content: content,
		Author:  author,
	})
	return err
}

// submitToRemote submits a note to a remote KB endpoint.
func submitToRemote(ctx context.Context, cfg *config.Config, kbTarget, title, content, author string) error {
	// Find the KB config
	var kb *config.KnowledgeBase
	for i := range cfg.KnowledgeBases {
		if cfg.KnowledgeBases[i].Name == kbTarget {
			kb = &cfg.KnowledgeBases[i]
			break
		}
	}
	if kb == nil {
		return fmt.Errorf("knowledge base %q not found in config", kbTarget)
	}

	_, err := submit.SubmitToRemoteKB(ctx, kb.Endpoint, kb.Auth, kb.AWSProfile, kb.AWSRegion,
		submit.RemoteSubmitRequest{
			Title:   title,
			Content: content,
			Author:  author,
		})
	return err
}

// containsTarget checks if a target KB is in the list.
func containsTarget(targets []string, target string) bool {
	for _, t := range targets {
		if t == target {
			return true
		}
	}
	return false
}

// remainingTargets reads the remaining targets for a pending entry.
// Returns nil if the file has been deleted (all targets resolved).
func remainingTargets(pendingDir, filename string) []string {
	entry, err := route.ReadPending(pendingDir, filename)
	if err != nil {
		return nil
	}
	return entry.TargetKBs
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{Error: msg})
}
