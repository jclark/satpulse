//go:build !linux

package phc

import (
	"errors"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
)

var errPHCNotSupported = errors.New("PHC not supported on this platform")

const (
	PinFuncNone PinFunc = iota
	PinFuncExtts
	PinFuncPerout
)

// Clock represents a PTP Hardware Clock on non-Linux systems.
type Clock struct{}

// Open opens a PHC device (stub for non-Linux).
func Open(path string) (*Clock, error) {
	return nil, errPHCNotSupported
}

// ClockPath returns the device path for a PHC index.
func ClockPath(phcIndex int) string {
	return ""
}

// IfPhcIndex returns the PHC index for an interface.
func IfPhcIndex(ifname string) (phcIndex int, err error) {
	return 0, errPHCNotSupported
}

func (clk *Clock) Path() string {
	panic(errPHCNotSupported)
}

func (clk *Clock) ExttsAvailable(timeout time.Duration) bool {
	panic(errPHCNotSupported)
}

func (clk *Clock) ReadExtts() (ptime.Time, uint32, error) {
	panic(errPHCNotSupported)
}

func (clk *Clock) Close() error {
	panic(errPHCNotSupported)
}

func (clk *Clock) PinSetFunc(pinIndex uint32, pinFunc PinFunc, chanIndex uint32) error {
	panic(errPHCNotSupported)
}

func (clk *Clock) PinCount() int {
	panic(errPHCNotSupported)
}

func (clk *Clock) ExttsEnable(chanIndex uint32, enabled bool) (edges int, err error) {
	panic(errPHCNotSupported)
}

func (clk *Clock) ExttsChanCount() int {
	panic(errPHCNotSupported)
}

func (clk *Clock) SysOffset(nSamples int) (MultiSample, error) {
	panic(errPHCNotSupported)
}

func (clk *Clock) SysOffsetExtended(nSamples int) (MultiSample, error) {
	panic(errPHCNotSupported)
}

func (clk *Clock) SysOffsetPrecise(_ int) (MultiSample, error) {
	panic(errPHCNotSupported)
}

func (clk *Clock) AdjTime(d time.Duration) error {
	panic(errPHCNotSupported)
}

func (clk *Clock) FreqOffset() (float64, error) {
	panic(errPHCNotSupported)
}

func (clk *Clock) SetFreqOffset(fo float64) error {
	panic(errPHCNotSupported)
}

func (clk *Clock) MaxFreqOffset() float64 {
	panic(errPHCNotSupported)
}
