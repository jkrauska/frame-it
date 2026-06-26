package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jkrauska/frame-it/internal/schedule"
	"github.com/jkrauska/frame-it/internal/userlog"
)

func runSchedule(ctx context.Context, flags cliFlags, args []string, log *userlog.Logger) int {
	interval, err := time.ParseDuration(flags.scheduleInterval)
	if err != nil {
		log.Error(fmt.Sprintf("Invalid --interval: %v", err))
		return 1
	}

	loc := time.Local
	if flags.scheduleTZ != "" {
		loc, err = time.LoadLocation(flags.scheduleTZ)
		if err != nil {
			log.Error(fmt.Sprintf("Invalid --timezone: %v", err))
			return 1
		}
	}

	window, err := schedule.NewWindow(flags.scheduleStart, flags.scheduleEnd, loc)
	if err != nil {
		log.Error(err.Error())
		return 1
	}

	tick := func(ctx context.Context) error {
		if runWallpaper(ctx, flags, args, log) != 0 {
			return fmt.Errorf("wallpaper refresh failed")
		}
		return nil
	}

	if flags.scheduleOnce {
		now := time.Now().In(loc)
		if !schedule.InWindow(now, window) {
			next := schedule.NextWindowStart(now, window)
			log.Step(fmt.Sprintf("Outside active hours — next window opens %s", next.Format("Mon 15:04")))
			return 0
		}
		log.Step("Refreshing wallpaper…")
		if err := tick(ctx); err != nil {
			log.Error(err.Error())
			return 1
		}
		return 0
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	err = schedule.Run(ctx, schedule.Options{
		Interval:   interval,
		Window:     window,
		RunOnStart: flags.scheduleRunOnStart,
	}, tick, log)
	if err == context.Canceled {
		log.Step("Scheduler stopped")
		return 0
	}
	if err != nil {
		log.Error(err.Error())
		return 1
	}
	return 0
}
