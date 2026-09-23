//go:build !windows

package picker

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
)

func prepareInput(f *os.File) error                 { return syscall.SetNonblock(int(f.Fd()), true) }
func restoreInput(f *os.File)                       { _ = syscall.SetNonblock(int(f.Fd()), false) }
func readInput(f *os.File, buf []byte) (int, error) { return syscall.Read(int(f.Fd()), buf) }
func isWouldBlock(err error) bool {
	return errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EINTR)
}
func notifyResize(c chan<- os.Signal)    { signal.Notify(c, syscall.SIGWINCH) }
func notifyTerminate(c chan<- os.Signal) { signal.Notify(c, os.Interrupt, syscall.SIGTERM) }
