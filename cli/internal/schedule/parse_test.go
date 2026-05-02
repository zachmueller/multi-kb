package schedule

import (
	"testing"
	"time"
)

func TestParseCronExpr_Basic(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"every minute", "* * * * *", false},
		{"every 5 minutes", "*/5 * * * *", false},
		{"every hour at :00", "0 * * * *", false},
		{"daily at midnight", "0 0 * * *", false},
		{"weekdays at 9am", "0 9 * * 1-5", false},
		{"specific time", "30 14 1 * *", false},
		{"too few fields", "* * *", true},
		{"too many fields", "* * * * * *", true},
		{"invalid value", "60 * * * *", true},
		{"invalid step", "*/0 * * * *", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCronExpr(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCronExpr(%q): error = %v, wantErr = %v", tt.expr, err, tt.wantErr)
			}
		})
	}
}

func TestParseField(t *testing.T) {
	tests := []struct {
		field   string
		min     int
		max     int
		want    []int
		wantErr bool
	}{
		{"*", 0, 59, seq(0, 59, 1), false},
		{"*/15", 0, 59, []int{0, 15, 30, 45}, false},
		{"5", 0, 59, []int{5}, false},
		{"1-5", 0, 59, []int{1, 2, 3, 4, 5}, false},
		{"1,3,5", 0, 59, []int{1, 3, 5}, false},
		{"1-10/3", 0, 59, []int{1, 4, 7, 10}, false},
		{"0-6", 0, 6, seq(0, 6, 1), false},
		{"60", 0, 59, nil, true},
		{"*/0", 0, 59, nil, true},
		{"5-2", 0, 59, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got, err := parseField(tt.field, tt.min, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseField(%q): error = %v, wantErr = %v", tt.field, err, tt.wantErr)
				return
			}
			if err == nil && !intSliceEqual(got, tt.want) {
				t.Errorf("parseField(%q) = %v, want %v", tt.field, got, tt.want)
			}
		})
	}
}

func TestNextRun(t *testing.T) {
	loc := time.UTC

	tests := []struct {
		name string
		expr string
		from time.Time
		want time.Time
	}{
		{
			name: "every 5 minutes from :02",
			expr: "*/5 * * * *",
			from: time.Date(2026, 5, 3, 10, 2, 0, 0, loc),
			want: time.Date(2026, 5, 3, 10, 5, 0, 0, loc),
		},
		{
			name: "every 5 minutes from :05 exactly",
			expr: "*/5 * * * *",
			from: time.Date(2026, 5, 3, 10, 5, 0, 0, loc),
			want: time.Date(2026, 5, 3, 10, 10, 0, 0, loc),
		},
		{
			name: "hourly at :30 from :31",
			expr: "30 * * * *",
			from: time.Date(2026, 5, 3, 10, 31, 0, 0, loc),
			want: time.Date(2026, 5, 3, 11, 30, 0, 0, loc),
		},
		{
			name: "daily at midnight from afternoon",
			expr: "0 0 * * *",
			from: time.Date(2026, 5, 3, 14, 0, 0, 0, loc),
			want: time.Date(2026, 5, 4, 0, 0, 0, 0, loc),
		},
		{
			name: "specific day and time",
			expr: "0 9 15 * *",
			from: time.Date(2026, 5, 3, 10, 0, 0, 0, loc),
			want: time.Date(2026, 5, 15, 9, 0, 0, 0, loc),
		},
		{
			name: "specific month wraps to next year",
			expr: "0 0 1 1 *",
			from: time.Date(2026, 5, 3, 10, 0, 0, 0, loc),
			want: time.Date(2027, 1, 1, 0, 0, 0, 0, loc),
		},
		{
			name: "every minute",
			expr: "* * * * *",
			from: time.Date(2026, 5, 3, 10, 30, 45, 0, loc),
			want: time.Date(2026, 5, 3, 10, 31, 0, 0, loc),
		},
		{
			name: "weekday only from Friday evening",
			expr: "0 9 * * 1-5",
			// May 1, 2026 is a Friday
			from: time.Date(2026, 5, 1, 18, 0, 0, 0, loc),
			// Next weekday (Mon-Fri) at 9am: Monday May 4
			want: time.Date(2026, 5, 4, 9, 0, 0, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NextRunAfter(tt.expr, tt.from)
			if err != nil {
				t.Fatalf("NextRunAfter(%q, %v): unexpected error: %v", tt.expr, tt.from, err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("NextRunAfter(%q, %v) = %v, want %v", tt.expr, tt.from, got, tt.want)
			}
		})
	}
}

func TestNextRunAfter_ErrorOnBadExpr(t *testing.T) {
	_, err := NextRunAfter("bad", time.Now())
	if err == nil {
		t.Fatal("expected error for bad expression")
	}
}

func TestFindCronExpr(t *testing.T) {
	lines := []string{
		"# regular comment",
		"0 * * * * /usr/bin/something",
		"*/15 * * * * /usr/local/bin/multi-kb run --config /home/user/.multi-kb/config.yaml >> /home/user/.multi-kb/logs/cron.log 2>&1 # multi-kb scheduled run",
		"30 2 * * * /usr/bin/backup",
	}

	got := findCronExpr(lines)
	want := "*/15 * * * *"
	if got != want {
		t.Errorf("findCronExpr() = %q, want %q", got, want)
	}
}

func TestFindCronExpr_NoMatch(t *testing.T) {
	lines := []string{
		"0 * * * * /usr/bin/something",
	}

	got := findCronExpr(lines)
	if got != "" {
		t.Errorf("findCronExpr() = %q, want empty string", got)
	}
}

func TestFindCronExpr_EmptyInput(t *testing.T) {
	got := findCronExpr(nil)
	if got != "" {
		t.Errorf("findCronExpr(nil) = %q, want empty string", got)
	}
}

func TestContains(t *testing.T) {
	if !contains([]int{1, 3, 5, 7}, 5) {
		t.Error("expected contains(5) to be true")
	}
	if contains([]int{1, 3, 5, 7}, 4) {
		t.Error("expected contains(4) to be false")
	}
	if contains(nil, 1) {
		t.Error("expected contains on nil to be false")
	}
}

func TestSortInts(t *testing.T) {
	a := []int{5, 3, 1, 4, 2}
	sortInts(a)
	want := []int{1, 2, 3, 4, 5}
	if !intSliceEqual(a, want) {
		t.Errorf("sortInts() = %v, want %v", a, want)
	}
}

// --- helpers ---

func seq(start, end, step int) []int {
	var s []int
	for i := start; i <= end; i += step {
		s = append(s, i)
	}
	return s
}

func intSliceEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
