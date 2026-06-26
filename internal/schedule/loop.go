package schedule

import (
	"context"
	"fmt"
	"time"

	"github.com/jkrauska/frame-it/internal/userlog"
)

// Tick runs one unit of scheduled work (e.g. refresh wallpaper).
type Tick func(ctx context.Context) error

// Run executes tick on interval while inside the daily window; overnight it waits without changing the TV.
func Run(ctx context.Context, opts Options, tick Tick, log *userlog.Logger) error {
	if opts.Interval <= 0 {
		opts.Interval = 15 * time.Minute
	}
	if tick == nil {
		return fmt.Errorf("schedule tick is nil")
	}

	loc := opts.Window.location()
	log.Step(fmt.Sprintf(
		"Scheduler running — refresh every %s between %s and %s (%s)",
		opts.Interval,
		FormatClock(opts.Window.Start),
		FormatClock(opts.Window.End),
		loc,
	))

	skipNext := !opts.RunOnStart
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		now := time.Now().In(loc)
		if !InWindow(now, opts.Window) {
			skipNext = false
			next := NextWindowStart(now, opts.Window)
			log.Note(fmt.Sprintf(
				"Outside active hours — keeping current image until %s",
				next.Format("Mon 15:04"),
			))
			if err := sleepUntil(ctx, next); err != nil {
				return err
			}
			continue
		}

		if skipNext {
			skipNext = false
			log.Note("Skipping initial refresh (--no-run-on-start)")
		} else {
			log.Step("Refreshing wallpaper…")
			if err := tick(ctx); err != nil {
				log.Warn(fmt.Sprintf("Refresh failed: %v", err))
			}
		}

		next := NextWake(now, opts.Window, opts.Interval)
		if InWindow(next, opts.Window) {
			log.Note(fmt.Sprintf("Next refresh at %s", next.Format("15:04")))
		} else {
			log.Note(fmt.Sprintf("Holding overnight — next refresh at %s", next.Format("Mon 15:04")))
		}
		if err := sleepUntil(ctx, next); err != nil {
			return err
		}
	}
}

func sleepUntil(ctx context.Context, t time.Time) error {
	delay := time.Until(t)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
