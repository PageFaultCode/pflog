package pflog

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// logFileMode is the permission mode for log files and backups; logs can
// contain sensitive data so keep them owner-only.
const logFileMode = 0o600

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
	f, err := os.OpenFile(filepath.Clean(filename), os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFileMode)
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
// maxBackups is set. The log file is always reopened, even when an earlier
// step fails, so a failed rotation never leaves the writer wedged on a
// closed handle.
func (rw *RotatingWriter) rotate() error {
	var errs []error

	if err := rw.file.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close: %w", err))
	}

	renamed := false
	if err := os.Rename(rw.filename, rw.backupName()); err != nil {
		errs = append(errs, fmt.Errorf("rename: %w", err))
	} else {
		renamed = true
	}

	f, err := os.OpenFile(filepath.Clean(rw.filename), os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFileMode)
	if err != nil {
		errs = append(errs, fmt.Errorf("open: %w", err))
		return errors.Join(errs...)
	}
	rw.file = f
	if renamed {
		rw.size = 0
	}
	// if the rename failed we reopened the old oversized file; size is
	// unchanged and rotation will be retried on the next write

	// Compress old backups (all except the one just created) before pruning,
	// so maxBackups counts compressed files too.
	if rw.compress {
		rw.compressOldBackups()
	}

	if rw.maxBackups > 0 {
		rw.pruneBackups()
	}
	return errors.Join(errs...)
}

// backupName returns an unused backup filename. The timestamp only has
// one-second granularity, so a numeric suffix is added when rotations
// collide within the same second (a bare rename would clobber the
// earlier backup).
func (rw *RotatingWriter) backupName() string {
	base := rw.filename + "." + time.Now().UTC().Format(backupTimeFormat)
	backup := base
	for i := 1; ; i++ {
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			return backup
		}
		backup = fmt.Sprintf("%s-%d", base, i)
	}
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

const backupTimeFormat = "20060102-150405"

// findBackups returns absolute paths for backup files in the same directory as
// filename. If compressed is true, only .gz files are returned; otherwise only
// plain (non-.gz) files are returned. Only names carrying a rotation
// timestamp after the prefix are considered, so unrelated files that merely
// share the prefix (e.g. app.log.txt) are never compressed or pruned.
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
		if !hasBackupTimestamp(name[len(prefix):]) {
			continue
		}
		isGz := strings.HasSuffix(name, ".gz")
		if isGz == compressed {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out
}

// hasBackupTimestamp reports whether rest begins with a rotation timestamp
// in the backupTimeFormat layout (e.g. 20060102-150405).
func hasBackupTimestamp(rest string) bool {
	if len(rest) < len(backupTimeFormat) {
		return false
	}
	for i := 0; i < len(backupTimeFormat); i++ {
		c := rest[i]
		if i == 8 {
			if c != '-' {
				return false
			}
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// compressFile gzip-compresses src to src+".gz" and removes src on success.
func compressFile(src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	gzPath := src + ".gz"
	out, err := os.OpenFile(gzPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, logFileMode)
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
