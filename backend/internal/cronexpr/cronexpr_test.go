package cronexpr

import (
	"errors"
	"testing"
	"time"
)

func mustParse(t *testing.T, spec string) Schedule {
	t.Helper()
	s, err := Parse(spec)
	if err != nil {
		t.Fatalf("Parse(%q): unexpected error %v", spec, err)
	}
	return s
}

func TestParseErrors(t *testing.T) {
	cases := []string{
		"",                       // empty
		"* * * *",                // too few fields
		"* * * * * *",            // too many fields
		"60 * * * *",             // minute out of range
		"* 24 * * *",             // hour out of range
		"* * 0 * *",              // dom below range
		"* * * 13 *",             // month out of range
		"* * * * 7",              // dow out of range
		"abc * * * *",            // non-numeric
		"* * 1-31-2 * *",         // malformed range
		"* * -5 * *",             // negative
		"*/0 * * * *",            // zero step
		"* * * *",                // still too few
		"5-1 * * * *",            // inverted range
		"1,2,,3 * * * *",         // empty list member
	}
	for _, spec := range cases {
		if _, err := Parse(spec); !errors.Is(err, ErrInvalidSpec) {
			t.Errorf("Parse(%q): expected ErrInvalidSpec, got %v", spec, err)
		}
	}
}

func TestParseValidSpecs(t *testing.T) {
	cases := []string{
		"* * * * *",
		"0 0 * * *",
		"*/15 * * * *",
		"0 9-17 * * 1-5",
		"30 8 1,15 * *",
	}
	for _, spec := range cases {
		if _, err := Parse(spec); err != nil {
			t.Errorf("Parse(%q): unexpected error %v", spec, err)
		}
	}
	if _, err := Parse("0 0 * * MON"); !errors.Is(err, ErrInvalidSpec) {
		t.Errorf("dow names should be rejected, got %v", err)
	}
}

func TestNextEveryMinute(t *testing.T) {
	s := mustParse(t, "* * * * *")
	after := time.Date(2026, 9, 5, 10, 30, 59, 0, time.UTC)
	want := time.Date(2026, 9, 5, 10, 31, 0, 0, time.UTC)
	if got := s.Next(after); !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNextDaily(t *testing.T) {
	s := mustParse(t, "0 9 * * *")
	after := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	want := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	if got := s.Next(after); !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
	// same minute boundary: strictly after
	after = time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	want = time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)
	if got := s.Next(after); !got.Equal(want) {
		t.Errorf("Next at exact fire time = %v, want next day %v", got, want)
	}
}

func TestNextStepMinutes(t *testing.T) {
	s := mustParse(t, "*/15 * * * *")
	after := time.Date(2026, 9, 5, 10, 16, 0, 0, time.UTC)
	want := time.Date(2026, 9, 5, 10, 30, 0, 0, time.UTC)
	if got := s.Next(after); !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNextMonthBoundary(t *testing.T) {
	s := mustParse(t, "0 0 1 * *")
	after := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if got := s.Next(after); !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNextDayOfWeek(t *testing.T) {
	// 2026-09-05 is a Saturday; next Monday is 2026-09-07.
	s := mustParse(t, "0 8 * * 1")
	after := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	want := time.Date(2026, 9, 7, 8, 0, 0, 0, time.UTC)
	if got := s.Next(after); !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNextListAndRange(t *testing.T) {
	s := mustParse(t, "30 8 1,15 * *")
	after := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	want := time.Date(2026, 9, 15, 8, 30, 0, 0, time.UTC)
	if got := s.Next(after); !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}

	s = mustParse(t, "0 9-17 * * *")
	after = time.Date(2026, 9, 5, 18, 0, 0, 0, time.UTC)
	want = time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	if got := s.Next(after); !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNextDomDowRestrictedEither(t *testing.T) {
	// Both dom and dow restricted: fire on the 1st OR Mondays.
	// 2026-09-05 is Saturday; 2026-09-07 is Monday.
	s := mustParse(t, "0 0 1 * 1")
	after := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	want := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	if got := s.Next(after); !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNextImpossibleDate(t *testing.T) {
	s := mustParse(t, "0 0 31 2 *") // Feb 31st never exists
	if got := s.Next(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); !got.IsZero() {
		t.Errorf("Next for impossible date = %v, want zero time", got)
	}
}

func TestNextRespectsLocation(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*3600)
	s := mustParse(t, "0 9 * * *")
	after := time.Date(2026, 9, 5, 2, 0, 0, 0, loc) // 09:00 local just passed? no: 2:00 local
	want := time.Date(2026, 9, 5, 9, 0, 0, 0, loc)
	if got := s.Next(after); !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}
