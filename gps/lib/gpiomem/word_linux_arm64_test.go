//go:build linux && arm64

package gpiomem

import (
	"encoding/binary"
	"testing"

	"golang.org/x/sys/unix"
)

func TestWord(t *testing.T) {
	b, err := unix.Mmap(-1, 0, 0x1000, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_ANON|unix.MAP_PRIVATE)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Munmap(b)
	binary.NativeEndian.PutUint32(b[0xffc:], 0x12345678)
	tests := []struct {
		name        string
		offset      int
		expect      uint32
		expectPanic bool
	}{
		{name: "last word", offset: 0xffc, expect: 0x12345678},
		{name: "first word", offset: 0},
		{name: "past the end", offset: 0xffd, expectPanic: true},
		{name: "after the end", offset: 0x1000, expectPanic: true},
		{name: "negative", offset: -4, expectPanic: true},
		{name: "misaligned", offset: 2, expectPanic: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); (r != nil) != tc.expectPanic {
					t.Errorf("panic %v, want panic %v", r, tc.expectPanic)
				}
			}()
			if got := *word(b, tc.offset); got != tc.expect {
				t.Errorf("got %#x, want %#x", got, tc.expect)
			}
		})
	}
}
