// Package pflog defines all of the pflog package
package pflog

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type RegressionTestSuite struct {
	suite.Suite
}

// A completely full ring buffer must still dump its backlog on trigger;
// firstEntry == nextEntry used to be mistaken for an empty buffer.
func (suite *RegressionTestSuite) TestFullBufferDumpsOnTrigger() {
	log := New()
	log.SetCompactDuplicates(false)

	suite.Nil(log.SetBacklogDepth(3))

	var buf bytes.Buffer
	_ = log.AddOutputTarget(&buf)

	log.Trace("one")
	log.Trace("two")
	log.Trace("three") // buffer now exactly full
	log.Log(Fatal, "boom")

	output := buf.String()
	suite.Contains(output, "two")
	suite.Contains(output, "three")
	suite.Contains(output, "boom")
}

// Overflowing the buffer past full must keep the newest entries.
func (suite *RegressionTestSuite) TestOverfullBufferKeepsNewest() {
	log := New()
	log.SetCompactDuplicates(false)

	suite.Nil(log.SetBacklogDepth(3))

	var buf bytes.Buffer
	_ = log.AddOutputTarget(&buf)

	for i := 1; i <= 5; i++ {
		log.Tracef("entry%d", i)
	}
	log.Log(Fatal, "boom")

	output := buf.String()
	suite.NotContains(output, "entry2")
	suite.Contains(output, "entry4")
	suite.Contains(output, "entry5")
	suite.Contains(output, "boom")
}

// JSON and YAML formatters used to panic on a nil tag map.
func (suite *RegressionTestSuite) TestFormattersWithTags() {
	entry := NewEntry(Error, time.Now(), "tagged message", []*Tag{CreateTag("key", "value")})

	jf := &JSONFormatter{}
	jf.SetTimestampFormat(time.RFC3339)
	suite.NotPanics(func() {
		output := string(jf.Format(entry))
		suite.Contains(output, "tagged message")
		suite.Contains(output, "value")
	})

	yf := &YAMLFormatter{}
	yf.SetTimestampFormat(time.RFC3339)
	suite.NotPanics(func() {
		output := string(yf.Format(entry))
		suite.Contains(output, "tagged message")
		suite.Contains(output, "value")
	})
}

// A backlog depth of zero used to be accepted and then panic on first flush.
func (suite *RegressionTestSuite) TestZeroBacklogDepthRejected() {
	log := New()
	suite.NotNil(log.SetBacklogDepth(0))
}

// A config file omitting the backlog setting must fall back to the default
// depth instead of configuring a zero-length buffer.
func (suite *RegressionTestSuite) TestConfigDefaultBacklog() {
	var configuration Configuration
	configuration.Settings.Level = LogLevelInformation
	configuration.Settings.TriggerLevel = LogLevelFatal

	suite.Nil(configuration.LoadConfiguration())
	suite.Equal(DefaultBacklogDepth, configuration.GetLogger().backlogDepth)

	suite.NotPanics(func() {
		configuration.GetLogger().Log(Fatal, "boom")
	})
}

// CreateFormatter must return a fresh instance per call so per-target
// timestamp formats don't overwrite each other.
func (suite *RegressionTestSuite) TestCreateFormatterReturnsNewInstance() {
	first, err := CreateFormatter(formatterTypeID)
	suite.Nil(err)
	second, err := CreateFormatter(formatterTypeID)
	suite.Nil(err)
	suite.NotSame(first, second)

	first.SetTimestampFormat(time.RFC3339)
	second.SetTimestampFormat(time.Kitchen)

	entry := NewEntry(Error, time.Now(), "message", nil)
	suite.NotEqual(string(first.Format(entry)), "")
	suite.NotEqual(string(first.Format(entry)), string(second.Format(entry)))
}

// Concurrent logging, tagging, and cloning must be race-free (run with -race).
func (suite *RegressionTestSuite) TestConcurrentUse() {
	log := New()

	var buf bytes.Buffer
	_ = log.AddOutputTarget(&buf)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				log.Errorf("worker %d message %d", n, j)
				log.AddTag(fmt.Sprintf("tag%d", n), j)
				_ = log.Clone()
				_ = log.GetCompactDuplicates()
			}
		}(i)
	}
	wg.Wait()
}

// Rotations within the same second must not clobber earlier backups.
func (suite *RegressionTestSuite) TestRotationBackupCollision() {
	dir := suite.T().TempDir()
	logPath := filepath.Join(dir, "app.log")

	rw, err := newRotatingWriter(logPath, 32, 0, false)
	suite.Nil(err)

	line := []byte(strings.Repeat("x", 24) + "\n")
	for i := 0; i < 4; i++ {
		_, writeErr := rw.Write(line)
		suite.Nil(writeErr)
	}
	suite.Nil(rw.file.Close())

	backups := rw.findBackups(false)
	suite.Len(backups, 3)
}

// Unrelated files sharing the log's name prefix must never be pruned.
func (suite *RegressionTestSuite) TestPruneIgnoresUnrelatedFiles() {
	dir := suite.T().TempDir()
	logPath := filepath.Join(dir, "app.log")
	unrelated := filepath.Join(dir, "app.log.notes")
	suite.Nil(os.WriteFile(unrelated, []byte("keep me"), 0o600))

	rw, err := newRotatingWriter(logPath, 32, 1, false)
	suite.Nil(err)

	line := []byte(strings.Repeat("x", 24) + "\n")
	for i := 0; i < 4; i++ {
		_, writeErr := rw.Write(line)
		suite.Nil(writeErr)
	}
	suite.Nil(rw.file.Close())

	_, statErr := os.Stat(unrelated)
	suite.Nil(statErr)
	suite.Len(rw.findBackups(false), 1)
}

// syncBuffer guards bytes.Buffer with a mutex — needed only in these signal
// tests, where the dump runs on EnableSignalDump's own goroutine while the
// test concurrently polls the buffer via suite.Eventually. bytes.Buffer
// itself is not safe for that; the race is in the test harness, not in
// DumpBuffer/EnableSignalDump (dumpBufferRange's writes are already
// serialized by logLock on the production side).
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// A real SIGUSR1 must dump whatever is currently buffered, without needing
// a trigger-level entry — the on-demand path added for radar#60-equivalent
// "is this process healthy or wedged" investigations. Entries below `level`
// are the ones that only ever surface via a dump (Log() writes immediately,
// separately from buffering, for anything at or above level but below
// triggerLevel) — so this uses a Trace entry under an Information floor to
// exercise the buffer-only path the signal dump exists for.
func (suite *RegressionTestSuite) TestSignalDumpsBuffer() {
	log := New()
	log.SetCompactDuplicates(false)
	suite.Nil(log.SetLevel(Information))
	suite.Nil(log.SetTriggerLevel(Fatal)) // nothing here should self-trigger

	buf := &syncBuffer{}
	_ = log.AddOutputTarget(buf)

	stop := log.EnableSignalDump(syscall.SIGUSR1)
	defer stop()

	log.Trace("before signal")
	suite.Empty(buf.String(), "an entry below level must not write until dumped")

	suite.Nil(syscall.Kill(syscall.Getpid(), syscall.SIGUSR1))
	suite.Eventually(func() bool {
		return strings.Contains(buf.String(), "before signal")
	}, time.Second, 10*time.Millisecond)
}

// Repeated signals must dump repeatedly, not just once (no one-shot
// behavior, no state left corrupted by a prior dump).
func (suite *RegressionTestSuite) TestSignalDumpRepeats() {
	log := New()
	log.SetCompactDuplicates(false)
	suite.Nil(log.SetLevel(Trace))
	suite.Nil(log.SetTriggerLevel(Fatal))

	buf := &syncBuffer{}
	_ = log.AddOutputTarget(buf)

	stop := log.EnableSignalDump(syscall.SIGUSR1)
	defer stop()

	log.Trace("first batch")
	suite.Nil(syscall.Kill(syscall.Getpid(), syscall.SIGUSR1))
	suite.Eventually(func() bool {
		return strings.Contains(buf.String(), "first batch")
	}, time.Second, 10*time.Millisecond)

	log.Trace("second batch")
	suite.Nil(syscall.Kill(syscall.Getpid(), syscall.SIGUSR1))
	suite.Eventually(func() bool {
		return strings.Contains(buf.String(), "second batch")
	}, time.Second, 10*time.Millisecond)
}

// Trigger-level dumping must keep working unmodified once a signal handler
// is also registered — the two dump paths share dumpBuffer but must not
// interfere with each other.
func (suite *RegressionTestSuite) TestSignalDumpDoesNotAffectTriggerDump() {
	log := New()
	log.SetCompactDuplicates(false)
	suite.Nil(log.SetLevel(Trace))
	suite.Nil(log.SetTriggerLevel(Error))

	var buf bytes.Buffer
	_ = log.AddOutputTarget(&buf)

	stop := log.EnableSignalDump(syscall.SIGUSR1)
	defer stop()

	log.Trace("leads up to the error")
	log.Log(Error, "boom") // trigger-level dump, unrelated to the signal

	output := buf.String()
	suite.Contains(output, "leads up to the error")
	suite.Contains(output, "boom")
}

func TestRegressionTestSuite(t *testing.T) {
	suite.Run(t, new(RegressionTestSuite))
}
