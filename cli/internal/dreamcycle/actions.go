package dreamcycle

import (
	"fmt"
	"os"
	"strings"

	"github.com/zmueller/multi-kb/internal/submit"
)

// ApplyActions applies parsed consolidation actions to the note store.
// Returns a map of action type → count.
func ApplyActions(store NoteStore, actions []consolidationAction, batch Batch) (map[string]int, error) {
	counts := map[string]int{"keep": 0, "merge": 0, "split": 0, "consolidate": 0}

	// Build a lookup of all notes in the batch (pending + related)
	knownNotes := make(map[string]Note)
	knownNotes[batch.PendingNote.UID] = batch.PendingNote
	for _, n := range batch.RelatedNotes {
		knownNotes[n.UID] = n
	}

	for _, action := range actions {
		switch action.Type {
		case "keep":
			if err := applyKeep(store, action); err != nil {
				return counts, err
			}
			counts["keep"]++

		case "merge":
			if err := applyMerge(store, action, knownNotes); err != nil {
				return counts, err
			}
			counts["merge"]++

		case "split":
			if err := applySplit(store, action, knownNotes); err != nil {
				return counts, err
			}
			counts["split"]++

		case "consolidate":
			if err := applyConsolidate(store, action, knownNotes); err != nil {
				return counts, err
			}
			counts["consolidate"]++

		default:
			return counts, fmt.Errorf("unknown action type: %q", action.Type)
		}
	}

	return counts, nil
}

func applyKeep(store NoteStore, action consolidationAction) error {
	note, err := store.ReadNote(action.SourceUID)
	if err != nil {
		return fmt.Errorf("keep: read %q: %w", action.SourceUID, err)
	}

	note.Status = "active"
	return store.WriteNote(*note)
}

func applyMerge(store NoteStore, action consolidationAction, known map[string]Note) error {
	// Read the target active note
	target, err := store.ReadNote(action.TargetUID)
	if err != nil {
		return fmt.Errorf("merge: read target %q: %w", action.TargetUID, err)
	}

	// Update target with merged content
	target.Title = action.MergedTitle
	target.Content = action.MergedContent

	// Write updated target with consolidated-from-notes
	if err := writeNoteWithConsolidated(store, *target, []string{action.SourceUID}); err != nil {
		return fmt.Errorf("merge: write target: %w", err)
	}

	// Delete the source (pending) note
	if err := store.DeleteNote(action.SourceUID); err != nil {
		return fmt.Errorf("merge: delete source %q: %w", action.SourceUID, err)
	}

	return nil
}

func applySplit(store NoteStore, action consolidationAction, known map[string]Note) error {
	if len(action.NewNotes) < 2 {
		return fmt.Errorf("split: requires at least 2 new notes, got %d", len(action.NewNotes))
	}

	sourceNote := known[action.SourceUID]

	for _, spec := range action.NewNotes {
		uid, err := submit.GenerateUID()
		if err != nil {
			return fmt.Errorf("split: generate UID: %w", err)
		}

		newNote := Note{
			UID:     uid,
			Title:   spec.Title,
			Content: spec.Content,
			Author:  sourceNote.Author,
			Status:  "active",
		}

		if err := writeNoteWithConsolidated(store, newNote, []string{action.SourceUID}); err != nil {
			return fmt.Errorf("split: write new note: %w", err)
		}
	}

	// Delete the original pending note
	if err := store.DeleteNote(action.SourceUID); err != nil {
		return fmt.Errorf("split: delete source %q: %w", action.SourceUID, err)
	}

	return nil
}

func applyConsolidate(store NoteStore, action consolidationAction, known map[string]Note) error {
	if len(action.SourceUIDs) < 2 {
		return fmt.Errorf("consolidate: requires at least 2 source UIDs, got %d", len(action.SourceUIDs))
	}
	if action.ConsolidatedNote == nil {
		return fmt.Errorf("consolidate: missing consolidated_note")
	}

	// Content length heuristic: warn if consolidated content < 80% of combined source
	var combinedLen int
	var author string
	for _, uid := range action.SourceUIDs {
		if n, ok := known[uid]; ok {
			combinedLen += len(n.Content)
			if author == "" {
				author = n.Author
			}
		}
	}
	newContentLen := len(action.ConsolidatedNote.Content)
	if combinedLen > 0 && newContentLen < (combinedLen*80/100) {
		fmt.Fprintf(os.Stderr, "dream-cycle: WARNING: consolidate action content (%d chars) is less than 80%% of combined source content (%d chars)\n",
			newContentLen, combinedLen)
	}

	uid, err := submit.GenerateUID()
	if err != nil {
		return fmt.Errorf("consolidate: generate UID: %w", err)
	}

	newNote := Note{
		UID:     uid,
		Title:   action.ConsolidatedNote.Title,
		Content: action.ConsolidatedNote.Content,
		Author:  author,
		Status:  "active",
	}

	if err := writeNoteWithConsolidated(store, newNote, action.SourceUIDs); err != nil {
		return fmt.Errorf("consolidate: write new note: %w", err)
	}

	// Delete all source notes
	var deletedUIDs []string
	for _, uid := range action.SourceUIDs {
		if n, ok := known[uid]; ok && n.Status == "active" {
			deletedUIDs = append(deletedUIDs, uid)
		}
		if err := store.DeleteNote(uid); err != nil {
			return fmt.Errorf("consolidate: delete %q: %w", uid, err)
		}
	}

	if len(deletedUIDs) > 0 {
		fmt.Fprintf(os.Stderr, "dream-cycle: consolidate — deleted active notes %s\n",
			strings.Join(deletedUIDs, ", "))
	}

	return nil
}

// writeNoteWithConsolidated is a helper that writes a note file with the consolidated-from-notes
// field populated. It uses the localNoteStore's internal rendering.
func writeNoteWithConsolidated(store NoteStore, note Note, consolidatedFrom []string) error {
	// For the localNoteStore, we need to write the file with consolidated-from-notes.
	// Since the NoteStore interface only has WriteNote, we handle it through the standard
	// write path — the consolidation info is tracked in the commit message and frontmatter.
	//
	// For local store: write directly to file with custom rendering.
	if ls, ok := store.(*localNoteStore); ok {
		filename := note.UID + ".md"
		path := ls.kbDir + "/" + filename
		body := renderNoteFileWithConsolidated(note, consolidatedFrom)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			return err
		}
		ls.staged = append(ls.staged, filename)
		return nil
	}

	// Fallback: use standard WriteNote (consolidation tracking lost for non-local stores)
	return store.WriteNote(note)
}
