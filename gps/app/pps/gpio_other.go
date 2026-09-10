//go:build !(linux && arm64)

package pps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// DetectGPIO reports that GPIO PPS polling is not supported on this
// platform.
func DetectGPIO(context.Context, *slog.Logger, int, GPIOConfig, chan<- CandidateEdge, *PollStats) error {
	return fmt.Errorf("GPIO PPS: %w", errors.ErrUnsupported)
}
