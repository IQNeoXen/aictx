package fzf

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mattn/go-isatty"
)

// Available returns true if fzf is on PATH.
func Available() bool {
	_, err := exec.LookPath("fzf")
	return err == nil
}

// IsTerminal returns true if stdout is a terminal.
func IsTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// Pick shows an fzf picker with the given items and returns the selected one.
// current is highlighted in the header. Returns empty string if cancelled.
func Pick(items []string, current string) (string, error) {
	input := strings.Join(items, "\n")

	args := []string{"--ansi", "--no-preview"}
	if current != "" {
		args = append(args, "--header", fmt.Sprintf("current: %s", current))
	}

	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stderr = os.Stderr

	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		// fzf returns 130 on Ctrl-C / Esc
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 130 {
			return "", nil
		}
		return "", err
	}

	return strings.TrimSpace(out.String()), nil
}
