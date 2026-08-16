// Package pflog defines all of the pflog package
package pflog

import (
	"os"
	"os/signal"
)

// EnableSignalDump wires DumpBuffer to the given signals (e.g. SIGUSR1) so
// the current backlog can be inspected on demand without restarting the
// process or waiting for a trigger-level entry — the two situations that
// otherwise look identical from outside (a healthy-but-quiet process and a
// wedged one). Repeated signals dump repeatedly; there is no one-shot
// behavior and no state left corrupted by a dump.
//
// Unlike a C signal handler, the Go runtime delivers signals.Notify on an
// ordinary goroutine rather than a restricted async-signal-safe context, so
// DumpBuffer's own locking and I/O are safe to run directly on receipt —
// no flag-and-defer-to-the-main-loop indirection is needed here.
//
// Call the returned stop function to unregister the handler and let the
// signals resume their default disposition (relevant mainly in tests, or
// if a caller wants to rewire the same signal elsewhere later).
func (l *Log) EnableSignalDump(sigs ...os.Signal) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, sigs...)

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-sigCh:
				l.DumpBuffer()
			case <-done:
				return
			}
		}
	}()

	return func() {
		signal.Stop(sigCh)
		close(done)
	}
}
