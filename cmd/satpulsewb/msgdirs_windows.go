package main

import (
	"os"
	"path/filepath"
)

// systemDirs returns the installed message-file library. Windows has no
// shared data hierarchy, so the library ships beside the executable,
// wherever the installer or the unpacked archive put it.
func systemDirs() []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(filepath.Dir(exe), "gpsmsg")}
}
