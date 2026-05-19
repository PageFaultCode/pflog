package pflog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RotatingWriter is an io.Writer that writes to a named file and rotates it
// when the file would exceed maxSize bytes. Rotated files are renamed with a
// UTC timestamp suffix (e.g. app.log.20060102-150405) so they sort in
// chronological order. If maxBackups is non-zero, the oldest backups beyond
// that count are deleted automatically.
type RotatingWriter struct {
	filename   string
	maxSize    int64
	maxBackups int
	file       *os.File
	size       int64
	mu         sync.Mutex
}

// newRotatingWriter opens (or creates) filename in append mode and returns a
// RotatingWriter. maxSizeBytes == 0 disables rotation (writes pass straight
// through). Callers should treat the returned value as an io.Writer.
func newRotatingWriter(filename string, maxSizeBytes int64, maxBackups int) (*RotatingWriter, error) {
	f, err := os.OpenFile(filepath.Clean(filename), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &RotatingWriter{
		filename:   filename,
		maxSize:    maxSizeBytes,
		maxBackups: maxBackups,
		file:       f,
		size:       info.Size(),
	}, nil
}

// Write implements io.Writer. It rotates the backing file before writing if the
// write would push the file past maxSize. Rotation failures are reported to
// stderr but do not drop the log message.
func (rw *RotatingWriter) Write(p []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.maxSize > 0 && rw.size+int64(len(p)) > rw.maxSize {
		if err := rw.rotate(); err != nil {
			fmt.Fprintf(os.Stderr, "pflog: rotation failed for %s: %v\n", rw.filename, err)
		}
	}

	n, err := rw.file.Write(p)
	rw.size += int64(n)
	return n, err
}

// rotate closes the current file, moves it to a timestamped backup name, opens
// a fresh log file, and prunes old backups when maxBackups is set.
func (rw *RotatingWriter) rotate() error {
	if err := rw.file.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	backup := rw.filename + "." + time.Now().UTC().Format("20060102-150405")
	if err := os.Rename(rw.filename, backup); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	f, err := os.OpenFile(filepath.Clean(rw.filename), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	rw.file = f
	rw.size = 0

	if rw.maxBackups > 0 {
		rw.pruneBackups()
	}
	return nil
}

// pruneBackups removes the oldest timestamped backup files, retaining at most
// maxBackups of them.
func (rw *RotatingWriter) pruneBackups() {
	dir := filepath.Dir(rw.filename)
	prefix := filepath.Base(rw.filename) + "."

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var backups []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			backups = append(backups, filepath.Join(dir, e.Name()))
		}
	}

	sort.Strings(backups) // timestamp suffix sorts lexicographically oldest-first
	for len(backups) > rw.maxBackups {
		os.Remove(backups[0]) //nolint:errcheck
		backups = backups[1:]
	}
}
