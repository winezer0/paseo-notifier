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

// newRotatingWriter 创建新的日志轮转写入器
// 如果无法打开日志文件则返回 nil
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

// open 创建或打开日志文件以追加写入
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

// Write 实现 io.Writer，检查日志轮转后写入文件
func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if info, err := w.file.Stat(); err == nil && info.Size() >= w.maxSize {
		w.rotate()
	}
	return w.file.Write(p)
}

// rotate 将当前日志文件重命名为 .1 并创建新文件
func (w *rotatingWriter) rotate() {
	w.file.Close()

	backup := w.path + ".1"
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
		slog.Error("log rotation remove backup failed", "path", backup, "err", err)
	}
	if err := os.Rename(w.path, backup); err != nil {
		slog.Error("log rotation rename failed", "src", w.path, "dst", backup, "err", err)
	}

	if err := w.open(); err != nil {
		slog.Error("log rotation failed, unable to create new file", "path", w.path, "err", err)
	}
}

// Close 实现 io.Closer，关闭底层日志文件
func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}
