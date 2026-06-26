// Package schedule controls when frame-it refreshes wallpaper during the day.
package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Window is a daily active period measured from local midnight.
type Window struct {
	Start time.Duration // inclusive
	End   time.Duration // exclusive
	Loc   *time.Location
}

// Options configures the wallpaper refresh loop.
type Options struct {
	Interval   time.Duration
	Window     Window
	RunOnStart bool
}

// ParseClock parses "7:00", "07:30", "9pm", or "21:00" into a duration from midnight.
func ParseClock(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty time")
	}

	ampm := 0 // 0=24h, 1=am, 2=pm
	switch {
	case strings.HasSuffix(s, "am"):
		ampm = 1
		s = strings.TrimSuffix(s, "am")
	case strings.HasSuffix(s, "pm"):
		ampm = 2
		s = strings.TrimSuffix(s, "pm")
	}

	parts := strings.Split(s, ":")
	var hour, minute int
	switch len(parts) {
	case 1:
		h, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || h < 0 || h > 23 {
			return 0, fmt.Errorf("invalid hour in %q", s)
		}
		hour = h
	case 2:
		h, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || h < 0 || h > 23 {
			return 0, fmt.Errorf("invalid hour in %q", s)
		}
		m, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || m < 0 || m > 59 {
			return 0, fmt.Errorf("invalid minute in %q", s)
		}
		hour, minute = h, m
	default:
		return 0, fmt.Errorf("invalid time %q (use HH:MM or H:MMam/pm)", s)
	}

	switch ampm {
	case 1:
		if hour == 12 {
			hour = 0
		}
	case 2:
		if hour != 12 {
			hour += 12
		}
	}

	return time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute, nil
}

// NewWindow builds a daily window in loc (local timezone when nil).
func NewWindow(start, end string, loc *time.Location) (Window, error) {
	startDur, err := ParseClock(start)
	if err != nil {
		return Window{}, fmt.Errorf("start time: %w", err)
	}
	endDur, err := ParseClock(end)
	if err != nil {
		return Window{}, fmt.Errorf("end time: %w", err)
	}
	if startDur >= endDur {
		return Window{}, fmt.Errorf("start (%s) must be before end (%s)", start, end)
	}
	if loc == nil {
		loc = time.Local
	}
	return Window{Start: startDur, End: endDur, Loc: loc}, nil
}

func (w Window) location() *time.Location {
	if w.Loc != nil {
		return w.Loc
	}
	return time.Local
}

// InWindow reports whether now falls within [start, end).
func InWindow(now time.Time, w Window) bool {
	now = now.In(w.location())
	today := dayOffset(now)
	return today >= w.Start && today < w.End
}

func dayOffset(t time.Time) time.Duration {
	return time.Duration(t.Hour())*time.Hour +
		time.Duration(t.Minute())*time.Minute +
		time.Duration(t.Second())*time.Second +
		time.Duration(t.Nanosecond())
}

func dateInLoc(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// NextWindowStart returns the next time the active window opens.
func NextWindowStart(now time.Time, w Window) time.Time {
	loc := w.location()
	now = now.In(loc)
	todayStart := dateInLoc(now, loc).Add(w.Start)
	if dayOffset(now) < w.Start {
		return todayStart
	}
	return todayStart.Add(24 * time.Hour)
}

// NextWake returns when the scheduler should run again.
// When inside the window it is min(now+interval, today's window end); otherwise NextWindowStart.
func NextWake(now time.Time, w Window, interval time.Duration) time.Time {
	if interval <= 0 {
		interval = 15 * time.Minute
	}

	loc := w.location()
	now = now.In(loc)

	if !InWindow(now, w) {
		return NextWindowStart(now, w)
	}

	next := now.Add(interval)
	windowEnd := dateInLoc(now, loc).Add(w.End)
	if !next.Before(windowEnd) {
		return NextWindowStart(now, w)
	}
	return next
}

// FormatClock renders a duration-from-midnight as HH:MM.
func FormatClock(d time.Duration) string {
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	return fmt.Sprintf("%02d:%02d", h, m)
}
