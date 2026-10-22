//go:build !baremetal

package ntrip

// hasNet reports whether the platform has a network stack to listen on.
// Bare metal does not, and the tests that serve a caster skip.
const hasNet = true
