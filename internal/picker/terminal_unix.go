//go:build !windows

package picker

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// escLookahead: a lone Esc may be the first fragment of an arrow key, so wait
// briefly for the rest before treating it as cancel.
const escLookahead = true

// waitInput reports whether f has bytes to read within timeout.
// Never switch stdin to O_NONBLOCK instead: on a tty, stdin and stdout usually
// share one open file description, so frame writes would fail part-way (EAGAIN).
func waitInput(f *os.File, timeout time.Duration) (bool, error) {
	fd := int(f.Fd())
	if fd >= unix.FD_SETSIZE {
		return true, nil // cannot select on it; fall back to a blocking read
	}
	for {
		var set unix.FdSet
		set.Set(fd)
		tv := unix.NsecToTimeval(timeout.Nanoseconds())
		n, err := unix.Select(fd+1, &set, nil, nil, &tv)
		// Go's runtime preemption (SIGURG) interrupts select often. Retry, or an
		// interrupted wait would look like an Esc timeout and cancel the picker.
		if errors.Is(err, unix.EINTR) {
			continue
		}
		return n > 0, err
	}
}

func notifyResize(c chan<- os.Signal)    { signal.Notify(c, syscall.SIGWINCH) }
func notifyTerminate(c chan<- os.Signal) { signal.Notify(c, os.Interrupt, syscall.SIGTERM) }
