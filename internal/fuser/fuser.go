//go:build linux

// Package fuser finds processes using files or directories by examining /proc filesystem.
// It provides functionality similar to the Unix fuser utility.
// This is intended to be user as a backstop to TTY locking, to guard against
// our using a TTY that is being used by another process that does not do TTY locking.
package fuser

import (
	"os"
	"path/filepath"
	"strconv"
)

const procDir = "/proc"

// ListPIDs returns a slice of process IDs that have the given path open.
// The path is cleaned and converted to absolute form before matching.
// Returns an error if the path cannot be resolved or /proc cannot be read.
func ListPIDs(targetPath string) ([]int, error) {
	path, err := filepath.Abs(targetPath)
	if err != nil {
		return nil, err
	}
	path = filepath.Clean(path)

	entries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, err
	}

	var pids []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		if pidHasPath(pid, path) {
			pids = append(pids, pid)
		}
	}

	return pids, nil
}

// pidHasPath checks if a PID has the target path open
func pidHasPath(pid int, targetPath string) bool {
	fdDir := filepath.Join(procDir, strconv.Itoa(pid), "fd")

	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		fdPath := filepath.Join(fdDir, entry.Name())

		linkTarget, err := os.Readlink(fdPath)
		if err != nil {
			continue
		}

		// Clean link target for comparison
		linkTarget = filepath.Clean(linkTarget)

		if linkTarget == targetPath {
			return true
		}
	}

	return false
}
