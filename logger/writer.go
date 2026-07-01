package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const maxLogSize int64 = 10 * 1024 * 1024

type rotatingWriter struct {
	path    string
	maxSize int64
	file    *os.File
	mu      sync.Mutex
}

func newRotatingWriter(path string) *rotatingWriter {
	w := &rotatingWriter{
		path:    path,
		maxSize: maxLogSize,
	}
	if err := w.open(); err != nil {
		slog.Warn("failed to open log file, falling back to stdout", "path", path, "err", err)
		return nil
	}
	return w
}

func (w *rotatingWriter) open() error {
	dir := filepath.Dir(w.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	w.file = f
	return nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if info, err := w.file.Stat(); err == nil && info.Size() >= w.maxSize {
		w.rotate()
	}
	return w.file.Write(p)
}

func (w *rotatingWriter) rotate() {
	w.file.Close()

	backup := w.path + ".1"
	os.Remove(backup)
	os.Rename(w.path, backup)

	if err := w.open(); err != nil {
		slog.Error("log rotation failed, unable to create new file", "path", w.path, "err", err)
	}
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}
