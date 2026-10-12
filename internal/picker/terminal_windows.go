//go:build windows

package picker

import (
	"os"
	"os/signal"
	"time"
)

// The Windows console delivers a whole key sequence per read, so a lone Esc is
// a real Esc keypress (same behavior as the original picker).
const escLookahead = false

// ponytail: always "ready", so reads block and resize is not detected while
// idle. Add console-event polling if Windows users need live resize.
func waitInput(f *os.File, timeout time.Duration) (bool, error) { return true, nil }

func notifyResize(c chan<- os.Signal)    {}
func notifyTerminate(c chan<- os.Signal) { signal.Notify(c, os.Interrupt) }
