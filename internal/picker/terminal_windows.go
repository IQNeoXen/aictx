//go:build windows

package picker

import (
	"os"
	"os/signal"
)

func prepareInput(f *os.File) error                 { return nil }
func restoreInput(f *os.File)                       {}
func readInput(f *os.File, buf []byte) (int, error) { return f.Read(buf) }
func isWouldBlock(err error) bool                   { return false }
func notifyResize(c chan<- os.Signal)               {}
func notifyTerminate(c chan<- os.Signal)            { signal.Notify(c, os.Interrupt) }
