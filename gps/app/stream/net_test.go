//go:build !baremetal

package stream

// hasNet reports whether the platform has a network stack to listen on.
// Bare metal does not, and the tests that need a socket skip.
const hasNet = true
