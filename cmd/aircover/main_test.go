package main

import (
	"os"
	"testing"
)

func TestMainFunc(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"aircover", "--help"}

	// Should not panic or exit since we just print help
	main()
}
