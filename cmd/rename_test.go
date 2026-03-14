package cmd

import (
	"bytes"
	"testing"
)

// executeCmd drives the root command with the given args and returns the error
// returned by RunE. stdout/stderr are suppressed to keep test output clean.
func executeCmd(args ...string) error {
	rootCmd.SetArgs(args)
	rootCmd.SetOut(&bytes.Buffer{})
	rootCmd.SetErr(&bytes.Buffer{})
	return rootCmd.Execute()
}

func TestRenameCmdSameNames(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := executeCmd("rename", "foo", "foo")
	if err == nil {
		t.Fatal("expected error for same old/new name, got nil")
	}
	const want = "old and new names are the same"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}
