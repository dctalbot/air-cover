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

func TestInitConfig(t *testing.T) {
	// Test with no config file
	cfgFile = ""
	initConfig()

	// Test with a non-existent config file
	cfgFile = "nonexistent.yaml"
	initConfig()

	// Test with a valid config file
	f, err := os.CreateTemp("", "testconfig-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString("port: 8081\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfgFile = f.Name()
	initConfig()
}
