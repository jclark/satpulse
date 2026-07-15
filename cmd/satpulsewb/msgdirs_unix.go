//go:build !darwin && !windows

package main

// systemDirs returns the installed message-file libraries. A local
// install (make install) precedes a packaged one, so it shadows it.
func systemDirs() []string {
	return []string{"/usr/local/share/satpulse/gpsmsg", "/usr/share/satpulse/gpsmsg"}
}
