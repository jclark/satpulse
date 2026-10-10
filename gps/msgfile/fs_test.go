//go:build !baremetal

package msgfile

// hasFS reports whether the platform has a file system. Bare metal does
// not, and the tests that write or read a message file skip.
const hasFS = true
