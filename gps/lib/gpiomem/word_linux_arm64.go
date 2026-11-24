//go:build linux && arm64

package gpiomem

import (
	"fmt"
	"unsafe"
)

// word returns a pointer to the 32-bit word at offset in b, so that High can
// read a register as the single, aligned 32-bit load the hardware needs. It
// panics unless the word lies within b and is aligned, so the pointer can
// reach only the word it names. b is mapped memory outside the Go heap, so
// the pointer keeps nothing alive: it is valid only until b is unmapped.
func word(b []byte, offset int) *uint32 {
	if offset < 0 || offset > len(b)-4 {
		panic(fmt.Sprintf("gpiomem: word at %#x outside %#x-byte mapping", offset, len(b)))
	}
	p := unsafe.Pointer(&b[offset])
	if uintptr(p)%4 != 0 {
		panic(fmt.Sprintf("gpiomem: word at %#x not aligned", offset))
	}
	return (*uint32)(p)
}
