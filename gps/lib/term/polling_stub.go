//go:build !linux

package term

import (
	"fmt"
	"os"
)

// OpenPolling is not supported on this platform.
func OpenPolling(path string) (*os.File, DevKind, error) {
	return nil, DevUnknown, fmt.Errorf("%s: polling not supported on this platform", path)
}
