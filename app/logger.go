package app

import (
	"os"
	"path/filepath"

	"github.com/gookit/slog"
	"github.com/gookit/slog/handler"
)

func NewLogger(opts Options) (*slog.Logger, error) {
	opts = normalizeOptions(opts)
	if err := os.MkdirAll(filepath.Dir(opts.LogPath), 0755); err != nil {
		return fallbackLogger(opts.LogPath, err), nil
	}
	h, err := handler.NewFileHandler(opts.LogPath, handler.WithLogLevels(slog.AllLevels))
	if err != nil {
		return fallbackLogger(opts.LogPath, err), nil
	}
	return slog.NewWithHandlers(h), nil
}

func fallbackLogger(path string, cause error) *slog.Logger {
	logger := slog.NewWithHandlers(handler.NewSimpleHandler(os.Stderr, slog.TraceLevel))
	logger.Errorf("could not open log file %q: %v", path, cause)
	return logger
}
