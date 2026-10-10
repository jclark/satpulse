//go:build baremetal

package serialenum

import "errors"

// List always fails: there are no serial ports to enumerate on bare metal.
func List() ([]Port, error) {
	return nil, errors.ErrUnsupported
}
