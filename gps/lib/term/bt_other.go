//go:build !linux

package term

import (
	"fmt"
	"os"
)

// OpenBT is not supported on this platform.
func OpenBT(addr BDAddr) (*os.File, error) {
	return nil, fmt.Errorf("bluetooth not supported on this platform")
}
