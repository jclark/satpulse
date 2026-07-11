package main

import "path/filepath"

// The only macOS install is the Homebrew tap, which puts the library
// under its prefix. The prefix depends on the architecture, so the
// prefix itself comes from msgdirs_darwin_{arm64,amd64}.go.
const (
	brewSiliconPrefix = "/opt/homebrew"
	brewIntelPrefix   = "/usr/local"
)

// systemDirs returns the installed message-file library.
func systemDirs() []string {
	return []string{filepath.Join(brewPrefix, "share", "satpulse", "gpsmsg")}
}
