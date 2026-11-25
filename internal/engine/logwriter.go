package engine

import (
	"os"
	"sync"
)

const defaultMaxLogBytes = 2 * 1024 * 1024 // 2 MB cap per child process log

// cappedFileWriter wraps an *os.File and truncates/rewinds it whenever written
// bytes exceed maxBytes, preventing unbounded disk growth in long-running sessions.
type cappedFileWriter struct {
	mu       sync.Mutex
	file     *os.File
	written  int64
	maxBytes int64
}

func newCappedFileWriter(path string, maxBytes int64) (*cappedFileWriter, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxLogBytes
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &cappedFileWriter{
		file:     f,
		maxBytes: maxBytes,
	}, nil
}

func (w *cappedFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return len(p), nil
	}
	if int64(len(p)) > w.maxBytes {
		p = p[len(p)-int(w.maxBytes):]
	}
	if w.written+int64(len(p)) > w.maxBytes {
		_ = w.file.Truncate(0)
		_, _ = w.file.Seek(0, 0)
		w.written = 0
	}
	n, err := w.file.Write(p)
	w.written += int64(n)
	return n, err
}

func (w *cappedFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
