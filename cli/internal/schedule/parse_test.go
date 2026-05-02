package schedule

import (
	"testing"
	"time"
)

func TestNextRunAfter(t *testing.T) {
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
		{
			name: "every 15 minutes",
			expr: "*/15 * * * *",
			from: time.Date(2026, 5, 3, 10, 7, 0, 0, loc),
			want: time.Date(2026, 5, 3, 10, 15, 0, 0, loc),
		},
		{
			name: "range list",
			expr: "0,30 9-17 * * *",
			from: time.Date(2026, 5, 3, 12, 31, 0, 0, loc),
			want: time.Date(2026, 5, 3, 13, 0, 0, 0, loc),
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

func TestNextRunAfter_InvalidExpr(t *testing.T) {
	badExprs := []string{
		"bad",
		"* * *",
		"* * * * * *",
		"",
	}
	for _, expr := range badExprs {
		t.Run(expr, func(t *testing.T) {
			_, err := NextRunAfter(expr, time.Now())
			if err == nil {
				t.Errorf("expected error for expression %q", expr)
			}
		})
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
