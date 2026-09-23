//go:build !windows

package picker

import (
	"os"
	"testing"
	"time"
)

func TestWaitInput(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	start := time.Now()
	if ready, err := waitInput(r, 30*time.Millisecond); err != nil || ready {
		t.Fatalf("empty pipe: ready=%v err=%v, want timeout", ready, err)
	}
	if elapsed := time.Since(start); elapsed < 25*time.Millisecond {
		t.Fatalf("returned after %v, want it to wait for the timeout", elapsed)
	}

	w.Write([]byte{'j'})
	if ready, err := waitInput(r, time.Second); err != nil || !ready {
		t.Fatalf("pending byte: ready=%v err=%v, want ready", ready, err)
	}
}
