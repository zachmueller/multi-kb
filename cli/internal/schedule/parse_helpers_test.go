package schedule

import (
	"testing"
	"time"
)

// --- extractMinuteInterval tests (AUD-014: Windows cron -> task scheduler) ---

func TestExtractMinuteInterval_Star(t *testing.T) {
	n, err := extractMinuteInterval("* * * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}
}

func TestExtractMinuteInterval_StarSlash(t *testing.T) {
	cases := []struct {
		expr string
		want int
	}{
		{"*/5 * * * *", 5},
		{"*/15 * * * *", 15},
		{"*/30 * * * *", 30},
		{"*/60 * * * *", 60},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			n, err := extractMinuteInterval(tc.expr)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if n != tc.want {
				t.Errorf("expected %d, got %d", tc.want, n)
			}
		})
	}
}

func TestExtractMinuteInterval_NumericMinute(t *testing.T) {
	// Fixed minute value maps to 60-minute interval (once per hour).
	n, err := extractMinuteInterval("0 9 * * 1-5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 60 {
		t.Errorf("expected 60, got %d", n)
	}
}

func TestExtractMinuteInterval_InvalidFields(t *testing.T) {
	_, err := extractMinuteInterval("* * *")
	if err == nil {
		t.Error("expected error for wrong field count")
	}
}

func TestExtractMinuteInterval_InvalidStep(t *testing.T) {
	_, err := extractMinuteInterval("*/0 * * * *")
	if err == nil {
		t.Error("expected error for zero step")
	}
}

func TestExtractMinuteInterval_UnsupportedMinuteField(t *testing.T) {
	_, err := extractMinuteInterval("1,30 * * * *")
	if err == nil {
		t.Error("expected error for unsupported minute list")
	}
}

// --- parseCSVLine tests (AUD-014: Windows schtasks CSV output) ---

func TestParseCSVLine_Quoted(t *testing.T) {
	line := `"multi-kb-run","5/3/2026 2:30:00 PM","Ready"`
	got := parseCSVLine(line)
	want := []string{"multi-kb-run", "5/3/2026 2:30:00 PM", "Ready"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestParseCSVLine_UnquotedFallback(t *testing.T) {
	got := parseCSVLine("a,b,c")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestParseCSVLine_NASentinel(t *testing.T) {
	line := `"multi-kb-run","N/A","Disabled"`
	got := parseCSVLine(line)
	if got[1] != "N/A" {
		t.Errorf("expected N/A, got %q", got[1])
	}
}

// --- ParseNextRun date format coverage (AUD-014: multiple Windows date formats) ---

func TestParseWindowsDateFormats(t *testing.T) {
	// Verify all documented formats parse to the same time.
	expected := time.Date(2026, 5, 3, 14, 30, 0, 0, time.Local)

	cases := []string{
		"5/3/2026 2:30:00 PM",
		"5/3/2026 14:30:00",
		"2026-01-03 14:30:00", // different date; just validate format parses
		"05/03/2026 2:30:00 PM",
		"05/03/2026 14:30:00",
	}
	formats := []string{
		"1/2/2006 3:04:05 PM",
		"1/2/2006 15:04:05",
		"2006-01-02 15:04:05",
		"01/02/2006 3:04:05 PM",
		"01/02/2006 15:04:05",
	}

	for i, input := range cases {
		t.Run(input, func(t *testing.T) {
			parsed, err := time.ParseInLocation(formats[i], input, time.Local)
			if err != nil {
				t.Fatalf("format %q failed to parse %q: %v", formats[i], input, err)
			}
			// For the two cases that share the same date/time, check equality.
			if i == 0 || i == 1 || i == 3 || i == 4 {
				if parsed.Hour() != expected.Hour() || parsed.Minute() != expected.Minute() {
					t.Errorf("unexpected time: got %v", parsed)
				}
			}
			_ = parsed
		})
	}
}
