package dreamcycle

// NoteStore abstracts note file operations for action application.
// Local mode implements with git repo operations; server mode uses CodeCommit.
type NoteStore interface {
	ReadNote(uid string) (*Note, error)
	WriteNote(note Note) error
	DeleteNote(uid string) error
	CommitBatch(message string) error
}
