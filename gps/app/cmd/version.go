package cmd

import "strings"

// version is set at build time via -ldflags -X.  It must be a valid
// HTTP token (RFC 9110): alphanumerics plus !#$%&'*+-.^_`|~, no
// spaces.  This lets it be used directly as the product-version in
// HTTP Server headers (e.g. Ntrip).  The Makefile's prerelease form
// "<v>-pre.<date>.<hash>[.dirty]" satisfies this.
var version string
var buildDate string

// Version returns the version and build date strings set at build time.
func Version() (string, string) {
	return version, buildDate
}

// ShortenVersion returns the build version trimmed to at most maxLen characters.
func ShortenVersion(maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	s := version
	for len(s) > maxLen {
		i := strings.LastIndexAny(s, "-.")
		if i <= 0 {
			break
		}
		s = s[:i]
	}
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

// VersionInfo returns a human-readable version string.
func VersionInfo() string {
	s := ""
	if version != "" {
		s = "version " + version
	}
	if buildDate != "" {
		if s != "" {
			s += ", "
		}
		s += "build date " + buildDate
	}
	if s == "" {
		return "no version information available"
	}
	return "satpulse" + " " + s
}
