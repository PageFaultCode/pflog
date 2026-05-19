package pflog

import (
	"compress/gzip"
	"fmt"
	"io"
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
// chronological order.
//
// When compress is true, all backup files except the most recently rotated one
// are gzip-compressed (app.log.20060101-120000.gz). This mirrors logrotate's
// behaviour: the newest backup stays plain for quick inspection; older ones are
// compressed on the next rotation cycle.
//
// If maxBackups is non-zero, the oldest backup files (compressed or plain)
// beyond that count are deleted automatically.
type RotatingWriter struct {
	filename   string
	maxSize    int64
	maxBackups int
	compress   bool
	file       *os.File
	size       int64
	mu         sync.Mutex
}

// newRotatingWriter opens (or creates) filename in append mode and returns a
// RotatingWriter. maxSizeBytes == 0 disables rotation.
func newRotatingWriter(filename string, maxSizeBytes int64, maxBackups int, compress bool) (*RotatingWriter, error) {
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
		compress:   compress,
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
// a fresh log file, optionally compresses old backups, and prunes when
// maxBackups is set.
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

	// Compress old backups (all except the one just created) before pruning,
	// so maxBackups counts compressed files too.
	if rw.compress {
		rw.compressOldBackups()
	}

	if rw.maxBackups > 0 {
		rw.pruneBackups()
	}
	return nil
}

// compressOldBackups gzip-compresses every plain backup file except the newest
// one (which was just created by rotate and should stay readable for quick
// inspection). Already-compressed files are left alone.
func (rw *RotatingWriter) compressOldBackups() {
	plain := rw.findBackups(false)
	sort.Strings(plain) // oldest first; timestamp suffix sorts lexicographically

	// Leave the newest plain backup uncompressed.
	if len(plain) <= 1 {
		return
	}
	toCompress := plain[:len(plain)-1]
	for _, path := range toCompress {
		if err := compressFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "pflog: compress %s: %v\n", path, err)
		}
	}
}

// pruneBackups removes the oldest backup files (plain and .gz counted together)
// so that at most maxBackups remain.
func (rw *RotatingWriter) pruneBackups() {
	all := append(rw.findBackups(false), rw.findBackups(true)...)
	sort.Strings(all) // oldest first

	for len(all) > rw.maxBackups {
		if err := os.Remove(all[0]); err != nil {
			fmt.Fprintf(os.Stderr, "pflog: prune %s: %v\n", all[0], err)
		}
		all = all[1:]
	}
}

// findBackups returns absolute paths for backup files in the same directory as
// filename. If compressed is true, only .gz files are returned; otherwise only
// plain (non-.gz) files are returned.
func (rw *RotatingWriter) findBackups(compressed bool) []string {
	dir := filepath.Dir(rw.filename)
	prefix := filepath.Base(rw.filename) + "."

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		isGz := strings.HasSuffix(name, ".gz")
		if isGz == compressed {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out
}

// compressFile gzip-compresses src to src+".gz" and removes src on success.
func compressFile(src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	gzPath := src + ".gz"
	out, err := os.Create(gzPath)
	if err != nil {
		return err
	}

	gz, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		out.Close()
		os.Remove(gzPath) //nolint:errcheck
		return err
	}

	_, copyErr := io.Copy(gz, in)
	gzCloseErr := gz.Close()
	outCloseErr := out.Close()

	if copyErr != nil {
		os.Remove(gzPath) //nolint:errcheck
		return copyErr
	}
	if gzCloseErr != nil {
		os.Remove(gzPath) //nolint:errcheck
		return gzCloseErr
	}
	if outCloseErr != nil {
		os.Remove(gzPath) //nolint:errcheck
		return outCloseErr
	}

	return os.Remove(src)
}
