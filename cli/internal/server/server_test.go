package server

import (
	"testing"
	"time"
)

func TestShouldProcessRecallLogs(t *testing.T) {
	now := time.Date(2026, 5, 3, 3, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		lastRun  time.Time
		schedule string
		want     bool
	}{
		{
			name:     "never run and past schedule",
			lastRun:  time.Time{},
			schedule: "02:00",
			want:     true,
		},
		{
			name:     "ran today already",
			lastRun:  now.Add(-2 * time.Hour),
			schedule: "02:00",
			want:     false,
		},
		{
			name:     "ran yesterday, past schedule",
			lastRun:  now.Add(-25 * time.Hour),
			schedule: "02:00",
			want:     true,
		},
		{
			name:     "ran yesterday, before schedule",
			lastRun:  now.Add(-25 * time.Hour),
			schedule: "04:00",
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldProcessRecallLogs(now, tc.lastRun, tc.schedule)
			if got != tc.want {
				t.Errorf("shouldProcessRecallLogs() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseSchedule(t *testing.T) {
	tests := []struct {
		input    string
		wantH    int
		wantM    int
	}{
		{"02:00", 2, 0},
		{"14:30", 14, 30},
		{"00:00", 0, 0},
		{"23:59", 23, 59},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			h, m := parseSchedule(tc.input)
			if h != tc.wantH || m != tc.wantM {
				t.Errorf("parseSchedule(%q) = (%d, %d), want (%d, %d)", tc.input, h, m, tc.wantH, tc.wantM)
			}
		})
	}
}

func TestSanitizeField(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"normal text", "normal text"},
		{"has/slash", "has_slash"},
		{"has\\backslash", "has_backslash"},
		{"has\"quote", "has_quote"},
		{"has`backtick", "has_backtick"},
		{"has\nnewline", "has_newline"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := sanitizeField(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeField(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestNoteFileToMarkdown(t *testing.T) {
	note := NoteFile{
		UID:         "ABCD1234EFGH5678",
		Title:       "Test Note",
		Content:     "Some content here.",
		Author:      "tester",
		SubmittedAt: "2026-05-03T10:00:00Z",
	}

	md := note.ToMarkdown()

	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
	if !contains(md, "uid: ABCD1234EFGH5678") {
		t.Error("expected UID in frontmatter")
	}
	if !contains(md, "status: pending") {
		t.Error("expected status: pending")
	}
	if !contains(md, "Some content here.") {
		t.Error("expected content in body")
	}
}

func TestUpdateLastRecalled(t *testing.T) {
	input := "---\nuid: ABC\ntitle: Test\nlast-recalled: \"\"\n---\n\nContent"
	result := updateLastRecalled(input, "2026-05-03T10:00:00Z")

	if !contains(result, "last-recalled: 2026-05-03T10:00:00Z") {
		t.Errorf("expected updated last-recalled, got:\n%s", result)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
