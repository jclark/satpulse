//go:build linux

package fuser

import (
	"os"
	"slices"
	"testing"
)

func TestPIDs(t *testing.T) {
	// Open /dev/null so our PID should be found
	f, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("Failed to open /dev/null: %v", err)
	}
	defer f.Close()

	// Get our own PID
	ourPID := os.Getpid()

	// Find PIDs using /dev/null
	pids, err := ListPIDs("/dev/null")
	if err != nil {
		t.Fatalf("PIDs failed: %v", err)
	}

	// Our PID should be in the list
	if !slices.Contains(pids, ourPID) {
		t.Errorf("Expected our PID %d to be in the list, but got: %v", ourPID, pids)
	}
}
