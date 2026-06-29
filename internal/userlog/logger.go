// Package userlog provides plain, human-readable CLI logging.
package userlog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
)

// Logger prints short progress lines for humans, not structured slog fields.
type Logger struct {
	w       io.Writer
	verbose bool
}

// New creates a logger. Verbose enables technical detail lines (WebSocket traffic, etc.).
func New(w io.Writer, verbose bool) *Logger {
	return &Logger{w: w, verbose: verbose}
}

// Step prints a main progress line.
func (l *Logger) Step(msg string) {
	l.write("", msg)
}

// Note prints a secondary line (indented).
func (l *Logger) Note(msg string) {
	l.write("  ", msg)
}

// Verbose prints a detail line only when verbose mode is on.
func (l *Logger) Verbose(msg string) {
	if l.verbose {
		l.write("  ", msg)
	}
}

// Warn prints a warning.
func (l *Logger) Warn(msg string) {
	l.write("! ", msg)
}

// Error prints an error line.
func (l *Logger) Error(msg string) {
	l.write("Error: ", msg)
}

func (l *Logger) write(prefix, msg string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(l.w, "%s %s%s\n", ts, prefix, msg)
}

// Slog returns a slog.Logger that writes through this human-friendly handler.
func (l *Logger) Slog() *slog.Logger {
	return slog.New(&handler{l: l})
}

type handler struct {
	l *Logger
}

func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	if level == slog.LevelDebug {
		return h.l.verbose
	}
	return level >= slog.LevelInfo
}

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	msg := humanizeRecord(r)
	switch {
	case r.Level >= slog.LevelError:
		h.l.Error(msg)
	case r.Level >= slog.LevelWarn:
		h.l.Warn(msg)
	case r.Level == slog.LevelDebug:
		h.l.Verbose(msg)
	default:
		h.l.Step(msg)
	}
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h // attrs are folded into Handle via Record
}

func (h *handler) WithGroup(name string) slog.Handler {
	return h
}

func humanizeRecord(r slog.Record) string {
	msg := r.Message
	var parts []string

	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "tv":
			return true
		case "endpoint":
			if s, ok := a.Value.Any().(string); ok {
				switch s {
				case "com.samsung.art-app":
					parts = append(parts, "art mode")
				case "samsung.remote.control":
					parts = append(parts, "remote control")
				default:
					parts = append(parts, s)
				}
			}
		case "host":
			if ip, ok := a.Value.Any().(string); ok && ip != "" {
				parts = append(parts, ip)
			}
		case "error":
			if err, ok := a.Value.Any().(error); ok {
				parts = append(parts, err.Error())
			} else if s := a.Value.String(); s != "" {
				parts = append(parts, s)
			}
		case "file":
			if s, ok := a.Value.Any().(string); ok {
				parts = append(parts, s)
			}
		case "bytes":
			if n, ok := a.Value.Any().(int); ok {
				parts = append(parts, fmt.Sprintf("%d bytes", n))
			} else if n, ok := a.Value.Any().(int64); ok {
				parts = append(parts, fmt.Sprintf("%d bytes", n))
			}
		case "payload":
			if s, ok := a.Value.Any().(string); ok {
				parts = append(parts, truncate(s, 120))
			}
		case "url":
			if s, ok := a.Value.Any().(string); ok {
				parts = append(parts, truncate(s, 80))
			}
		default:
			if s := a.Value.String(); s != "" && s != "<nil>" {
				parts = append(parts, fmt.Sprintf("%s=%s", a.Key, s))
			}
		}
		return true
	})

	if len(parts) == 0 {
		return msg
	}
	if msg == "" {
		return strings.Join(parts, " · ")
	}
	return msg + " (" + strings.Join(parts, " · ") + ")"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
