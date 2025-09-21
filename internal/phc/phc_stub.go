//go:build !linux

package phc

import (
	"time"

	"github.com/jclark/satpulse/internal/ptime"
)

const (
	PinFuncNone PinFunc = iota
	PinFuncExtts
	PinFuncPerout
	PinFuncPhysync
)

// Clock represents a PTP Hardware Clock on non-Linux systems.
type Clock struct{}

// Open opens a PHC device (stub for non-Linux).
func Open(path string) (*Clock, error) {
	return nil, ErrNotSupported
}

// ClockPath returns the device path for a PHC index.
func ClockPath(phcIndex int) string {
	return ""
}

// IfPhcIndex returns the PHC index for an interface.
func IfPhcIndex(ifname string) (phcIndex int, err error) {
	return 0, ErrNotSupported
}

func (clk *Clock) Path() string {
	panic(ErrNotSupported)
}

func (clk *Clock) ExttsAvailable(timeout time.Duration) bool {
	panic(ErrNotSupported)
}

func (clk *Clock) ReadExtts() (ptime.Time, uint32, error) {
	panic(ErrNotSupported)
}

func (clk *Clock) Close() error {
	panic(ErrNotSupported)
}

func (clk *Clock) PinSetFunc(pinIndex uint32, pinFunc PinFunc, chanIndex uint32) error {
	panic(ErrNotSupported)
}

func (clk *Clock) PinGetFunc(pinIndex uint32) (*PinDesc, error) {
	panic(ErrNotSupported)
}

func (clk *Clock) PinCount() int {
	panic(ErrNotSupported)
}

func (clk *Clock) ExttsEnable(chanIndex uint32, enabled bool) (edges int, err error) {
	panic(ErrNotSupported)
}

func (clk *Clock) ExttsChanCount() int {
	panic(ErrNotSupported)
}

func (clk *Clock) PeroutChanCount() int {
	panic(ErrNotSupported)
}

func (clk *Clock) PeroutEnable(chanIndex uint32, period, width, startOffset time.Duration) error {
	panic(ErrNotSupported)
}

func (clk *Clock) SysOffset(nSamples int) (MultiSample, error) {
	panic(ErrNotSupported)
}

func (clk *Clock) SysOffsetExtended(nSamples int) (MultiSample, error) {
	panic(ErrNotSupported)
}

func (clk *Clock) SysOffsetPrecise(_ int) (MultiSample, error) {
	panic(ErrNotSupported)
}

func (clk *Clock) AdjTime(d time.Duration) error {
	panic(ErrNotSupported)
}

func (clk *Clock) FreqOffset() (float64, error) {
	panic(ErrNotSupported)
}

func (clk *Clock) SetFreqOffset(fo float64) error {
	panic(ErrNotSupported)
}

func (clk *Clock) MaxFreqOffset() float64 {
	panic(ErrNotSupported)
}
