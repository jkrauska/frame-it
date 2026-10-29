package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/jkrauska/frame-it/internal/gallery"
	"github.com/jkrauska/frame-it/internal/userlog"
)

func runGallery(ctx context.Context, flags cliFlags, log *userlog.Logger) int {
	dir, _ := resolveArchive(flags)

	limit := flags.galleryLimit
	if limit <= 0 {
		limit = gallery.DefaultLimit
	}
	port := flags.galleryPort
	if port <= 0 {
		port = gallery.DefaultPort
	}
	bind := flags.galleryBind
	if bind == "" {
		bind = gallery.DefaultBind
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if flags.galleryOpen {
		openBrowser(gallery.LocalURL(port), log)
	}

	err := gallery.Serve(ctx, gallery.Options{Dir: dir, Limit: limit, Bind: bind, Port: port}, log)
	switch {
	case err == nil, errors.Is(err, context.Canceled):
		log.Step("Gallery stopped")
		return 0
	default:
		log.Error(err.Error())
		return 1
	}
}

// openBrowser launches the system browser at url; failures are only a warning
// since the address is printed anyway.
func openBrowser(url string, log *userlog.Logger) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Warn(fmt.Sprintf("Could not open a browser: %v", err))
	}
}
