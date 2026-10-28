package logger

import (
	"io"
	"log/slog"
	"os"
)

// New returns a JSON logger writing to stdout. Platforms (Render, Datadog, ELK)
// collect stdout; JSON makes every field searchable.
func New(level string) *slog.Logger {
	return NewWithWriter(os.Stdout, level)
}

func NewWithWriter(w io.Writer, level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl}))
}
