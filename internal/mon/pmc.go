package mon

import (
	"context"
	"sync"

	"github.com/jclark/gps2phc/internal/pmc"
	"golang.org/x/exp/slog"
)

func StartPMC(ctx context.Context, wg *sync.WaitGroup, lg *slog.Logger, cfg *pmc.ClientConfig) chan<- UpdateRequest {
	return nil
}
