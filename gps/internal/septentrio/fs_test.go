//go:build !baremetal

package septentrio

// hasFS reports whether the platform can open the capture files
// under testdata. Bare metal cannot, and the tests that need them skip.
const hasFS = true
