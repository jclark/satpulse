//go:build !baremetal

package ubxsim

// hasFS reports whether the platform has a file system. Bare metal does
// not, and the tests that load a personality file skip.
const hasFS = true
