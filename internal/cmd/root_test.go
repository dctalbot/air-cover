package cmd

import (
	"os"
	"testing"
)

func TestExecute(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"aircover", "--help"}

	Execute()
}

func TestExecuteError(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"aircover", "--unknown-flag"}

	originalRootOsExit := rootOsExit
	defer func() { rootOsExit = originalRootOsExit }()

	exited := false
	rootOsExit = func(code int) {
		exited = true
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
	}

	Execute()

	if !exited {
		t.Errorf("expected rootOsExit to be called")
	}
}
