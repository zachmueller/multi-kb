package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zmueller/multi-kb/internal/bedrock"
	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/extract"
	"github.com/zmueller/multi-kb/internal/lock"
	"github.com/zmueller/multi-kb/internal/logging"
	"github.com/zmueller/multi-kb/internal/route"
	"github.com/zmueller/multi-kb/internal/submit"
	"github.com/zmueller/multi-kb/internal/translate"
)

func newProcessCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "process",
		Short: "Scan conversations, extract knowledge, and route to KBs",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, _ := cmd.Root().PersistentFlags().GetString("config")
			return runProcess(cmd.Context(), cfgPath, "manual")
		},
	}
}

// runProcess is the main capture pipeline: scan → translate → extract → route → submit/stage.
// trigger is "manual" or "cron".
func runProcess(ctx context.Context, cfgPath, trigger string) error {
	start := time.Now()

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("process: load config: %w", err)
	}

	statePath := config.DefaultStatePath()
	state, err := config.LoadState(statePath)
	if err != nil {
		return fmt.Errorf("process: load state: %w", err)
	}

	lockPath := lock.DefaultLockPath()
	l, err := lock.Acquire(lockPath, "capture")
	if err != nil {
		return fmt.Errorf("process: %w", err)
	}
	defer l.Release()

	logsDir := logging.DefaultLogsDir()
	bedrockClient, err := bedrock.NewClient(ctx,
		cfg.Extraction.AWSProfile,
		cfg.Extraction.AWSRegion,
		cfg.Extraction.ModelID,
	)
	if err != nil {
		return fmt.Errorf("process: create Bedrock client: %w", err)
	}

	extractor := extract.NewExtractor(bedrockClient, cfg.ExclusionRules, logsDir)

	var (
		dirsScanned    int
		convsProcessed int
		notesExtracted int
		notesRouted    = make(map[string]int)
		errorCount     int
	)

	// Track KBs that returned auth errors — skip them for the rest of the run
	skippedKBs := make(map[string]bool)

	for _, source := range cfg.Sources {
		dirsScanned++
		dirState := state.Directories[source.Directory]

		var lastProcessed time.Time
		if dirState.LastProcessed != "" {
			lastProcessed, _ = time.Parse(time.RFC3339, dirState.LastProcessed)
		}

		sessions, err := discoverSessions(source, lastProcessed)
		if err != nil {
			fmt.Fprintf(os.Stderr, "process: discover sessions for %q: %v\n", source.Directory, err)
			errorCount++
			continue
		}

		var latestModTime time.Time
		for _, session := range sessions {
			convsProcessed++

			jsonl, convID, modTime, err := translateSession(session, source, lastProcessed)
			if err != nil {
				fmt.Fprintf(os.Stderr, "process: translate %q: %v\n", session.path, err)
				errorCount++
				continue
			}

			if modTime.After(latestModTime) {
				latestModTime = modTime
			}

			notes, err := extractor.ExtractChunked(ctx, convID, session.path, jsonl)
			if err != nil {
				errorCount++
				continue
			}
			notesExtracted += len(notes)

			routed := route.RouteNotes(cfg, notes, source.Directory, session.harness, session.persona)
			for _, rn := range routed {
				for _, target := range rn.Targets {
					if err := submitNote(ctx, cfg, target, rn.Note, skippedKBs); err != nil {
						var authErr *submit.AuthError
						if errors.As(err, &authErr) {
							skippedKBs[target.KB] = true
							fmt.Fprintf(os.Stderr, "process: auth error for KB %q — skipping for this run: %v\n", target.KB, err)
						} else {
							fmt.Fprintf(os.Stderr, "process: submit note to %q: %v\n", target.KB, err)
							errorCount++
						}
					} else {
						notesRouted[target.KB]++
					}
				}
			}
		}

		// Update last_processed to the last-modified file time for this directory
		if !latestModTime.IsZero() {
			if state.Directories == nil {
				state.Directories = make(map[string]config.DirectoryState)
			}
			state.Directories[source.Directory] = config.DirectoryState{
				LastProcessed: latestModTime.Format(time.RFC3339),
			}
		}
	}

	// Atomic state save
	if err := config.SaveState(statePath, state); err != nil {
		fmt.Fprintf(os.Stderr, "process: save state: %v\n", err)
	}

	_ = logging.AppendRunLog(logsDir, logging.RunEntry{
		Timestamp:              time.Now().UTC().Format(time.RFC3339),
		Type:                   "capture",
		Trigger:                trigger,
		DirectoriesScanned:     dirsScanned,
		ConversationsProcessed: convsProcessed,
		NotesExtracted:         notesExtracted,
		NotesRouted:            notesRouted,
		Errors:                 errorCount,
		DurationMS:             time.Since(start).Milliseconds(),
	})

	return nil
}

// sessionEntry holds metadata about a discovered session file.
type sessionEntry struct {
	path    string
	harness string
	persona string
}

// discoverSessions finds all session files to process for a source directory.
func discoverSessions(source config.Source, lastProcessed time.Time) ([]sessionEntry, error) {
	var sessions []sessionEntry

	for _, harness := range source.Harnesses {
		switch harness {
		case "claude-code":
			t, err := translate.NewClaudeCodeTranslator(source.Directory, lastProcessed)
			if err != nil {
				return nil, err
			}
			paths, err := t.Discover()
			if err != nil {
				return nil, err
			}
			for _, p := range paths {
				sessions = append(sessions, sessionEntry{path: p, harness: "claude-code"})
			}

		case "notor":
			t, err := translate.NewNotorTranslator(source.Directory, lastProcessed)
			if err != nil {
				return nil, err
			}
			paths, err := t.Discover()
			if err != nil {
				return nil, err
			}
			for _, p := range paths {
				sessions = append(sessions, sessionEntry{path: p, harness: "notor"})
			}
		}
	}

	return sessions, nil
}

// translateSession translates a session file to JSONL.
// Returns the JSONL string, conversation ID, last-modified file time, and any error.
func translateSession(session sessionEntry, source config.Source, lastProcessed time.Time) (string, string, time.Time, error) {
	var (
		conv *translate.Conversation
		err  error
	)

	switch session.harness {
	case "claude-code":
		t, terr := translate.NewClaudeCodeTranslator(source.Directory, lastProcessed)
		if terr != nil {
			return "", "", time.Time{}, terr
		}
		conv, err = t.TranslateSession(session.path)

	case "notor":
		t, terr := translate.NewNotorTranslator(source.Directory, lastProcessed)
		if terr != nil {
			return "", "", time.Time{}, terr
		}
		conv, err = t.TranslateSession(session.path)

	default:
		return "", "", time.Time{}, fmt.Errorf("unknown harness: %s", session.harness)
	}

	if err != nil {
		return "", "", time.Time{}, err
	}

	jsonl, err := conv.SerializeToString()
	if err != nil {
		return "", "", time.Time{}, err
	}

	fi, _ := os.Stat(session.path)
	var modTime time.Time
	if fi != nil {
		modTime = fi.ModTime()
	}

	return jsonl, conv.Header.ID, modTime, nil
}

// submitNote dispatches a routed note to its target KB based on approval mode.
func submitNote(
	ctx context.Context,
	cfg *config.Config,
	target route.ResolvedTarget,
	note extract.Note,
	skippedKBs map[string]bool,
) error {
	if skippedKBs[target.KB] {
		return nil
	}

	pendingDir := filepath.Join(homeDir(), ".multi-kb", "pending")

	if target.ApprovalMode == "require-manual-approval" {
		_, err := route.CreatePending(pendingDir, route.PendingEntry{
			Title:     note.Title,
			Content:   note.Content,
			Author:    cfg.Author,
			TargetKBs: []string{target.KB},
		})
		return err
	}

	// Auto-approve — submit directly
	if strings.HasPrefix(target.KB, "local/") {
		kbName := target.KB[6:]
		kbDir := filepath.Join(homeDir(), ".multi-kb", "local", kbName)
		_, err := submit.WriteNote(kbDir, submit.NoteFields{
			Title:   note.Title,
			Content: note.Content,
			Author:  cfg.Author,
		})
		return err
	}

	// Remote KB
	kb := findKBConfig(cfg, target.KB)
	if kb == nil {
		return fmt.Errorf("submit: KB %q not found in config", target.KB)
	}
	_, err := submit.SubmitToRemoteKB(ctx, kb.Endpoint, kb.Auth, kb.AWSProfile, kb.AWSRegion,
		submit.RemoteSubmitRequest{
			Title:   note.Title,
			Content: note.Content,
			Author:  cfg.Author,
		},
	)
	return err
}

func findKBConfig(cfg *config.Config, name string) *config.KnowledgeBase {
	for i := range cfg.KnowledgeBases {
		if cfg.KnowledgeBases[i].Name == name {
			return &cfg.KnowledgeBases[i]
		}
	}
	return nil
}

func homeDir() string {
	home, _ := os.UserHomeDir()
	return home
}
