//go:build !tinygo

package stream

// hasBcast reports whether bcast can run. Under TinyGo it cannot, because
// it uses reflect.Select, and the tests that run a Bcast skip.
const hasBcast = true
