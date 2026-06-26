package schedule

import (
	"testing"
	"time"
)

func mustWindow(t *testing.T, start, end string) Window {
	t.Helper()
	w, err := NewWindow(start, end, time.UTC)
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	return w
}

func at(h, m int) time.Time {
	return time.Date(2026, 6, 26, h, m, 0, 0, time.UTC)
}

func TestParseClock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want time.Duration
	}{
		{"7:00", 7 * time.Hour},
		{"07:30", 7*time.Hour + 30*time.Minute},
		{"9pm", 21 * time.Hour},
		{"9:00pm", 21 * time.Hour},
		{"12:00am", 0},
		{"12:00pm", 12 * time.Hour},
	}

	for _, tc := range tests {
		got, err := ParseClock(tc.in)
		if err != nil {
			t.Fatalf("ParseClock(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseClock(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestInWindow(t *testing.T) {
	w := mustWindow(t, "7:00", "21:00")

	if InWindow(at(6, 59), w) {
		t.Fatal("6:59 should be outside")
	}
	if !InWindow(at(7, 0), w) {
		t.Fatal("7:00 should be inside")
	}
	if !InWindow(at(20, 59), w) {
		t.Fatal("20:59 should be inside")
	}
	if InWindow(at(21, 0), w) {
		t.Fatal("21:00 should be outside")
	}
}

func TestNextWake(t *testing.T) {
	w := mustWindow(t, "7:00", "21:00")
	interval := 15 * time.Minute

	next := NextWake(at(10, 0), w, interval)
	if !next.Equal(at(10, 15)) {
		t.Fatalf("next = %v, want 10:15", next)
	}

	next = NextWake(at(20, 50), w, interval)
	if !next.Equal(at(21, 0).AddDate(0, 0, 1).Add(-14 * time.Hour)) {
		// 20:50 + 15m = 21:05 past window -> next day 7:00
		want := at(7, 0).AddDate(0, 0, 1)
		if !next.Equal(want) {
			t.Fatalf("next = %v, want %v", next, want)
		}
	}
}

func TestNextWindowStart(t *testing.T) {
	w := mustWindow(t, "7:00", "21:00")

	if !NextWindowStart(at(6, 0), w).Equal(at(7, 0)) {
		t.Fatal("before start should wake at 7:00 today")
	}
	if !NextWindowStart(at(22, 0), w).Equal(at(7, 0).AddDate(0, 0, 1)) {
		t.Fatal("after end should wake at 7:00 tomorrow")
	}
}

func TestNewWindowRejectsInvertedRange(t *testing.T) {
	_, err := NewWindow("21:00", "7:00", time.UTC)
	if err == nil {
		t.Fatal("expected error for inverted window")
	}
}
