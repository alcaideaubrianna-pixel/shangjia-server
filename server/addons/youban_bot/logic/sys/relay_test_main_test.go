package sys

import (
	"fmt"
	"os"
	"testing"
)

var relaySQLiteDirectory string

func TestMain(m *testing.M) {
	directory, err := os.MkdirTemp("", "youban-relay-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	relaySQLiteDirectory = directory
	code := m.Run()
	_ = os.RemoveAll(directory)
	os.Exit(code)
}
