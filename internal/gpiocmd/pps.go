//go:build linux

package gpiocmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/pps"
	"github.com/jclark/satpulse/gps/lib/gpiomem"
)

// pollPPS polls the GPIO that v names, or else the one GPIO set up as a PPS
// input, with satpulsed's poller and prints every edge it catches, rejected
// ones included, until the timeout or ctx is done.
func pollPPS(ctx context.Context, lg *slog.Logger, v flagVars, out io.Writer) error {
	name, err := pollName(v.name)
	if err != nil {
		return err
	}
	if v.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, v.timeout)
		defer cancel()
	}
	pin, err := gpiomem.Open(name)
	if err != nil {
		return err
	}
	defer pin.Close()
	lg.Info("GPIO PPS polling", "gpio", name, "device", pin.Device())
	n, err := detectEdges(ctx, lg, pin, name, v, out)
	if err != nil {
		return err
	}
	if n == 0 {
		return noDataError{msg: "no PPS edges detected"}
	}
	return nil
}

// pollName returns the SoC's name for the GPIO to poll: the GPIO called name,
// or, if name is empty, the one GPIO that a pps-gpio node uses.
func pollName(name string) (string, error) {
	if name != "" {
		gpio, err := gpiomem.Lookup(name)
		return gpio.Name, err
	}
	ctrls, err := gpiomem.List()
	if err != nil {
		return "", err
	}
	return onlyPPSInput(ctrls)
}

// onlyPPSInput returns the name of the one GPIO of ctrls that pps-gpio nodes
// use.
func onlyPPSInput(ctrls []gpiomem.Controller) (string, error) {
	var names []string
	for _, c := range ctrls {
		for _, p := range c.PPS {
			if !slices.Contains(names, p.Name) {
				names = append(names, p.Name)
			}
		}
	}
	if len(names) == 0 {
		return "", errors.New("no GPIO is set up as a PPS input; give the name of the GPIO to poll")
	}
	if len(names) > 1 {
		return "", fmt.Errorf("several GPIOs are set up as PPS inputs (%s); give the name of the one to poll", strings.Join(names, ", "))
	}
	return names[0], nil
}

// detectEdges prints the edges DetectGPIO catches on pin until it returns,
// which cancellation of parent or a failure to print makes it do, and
// returns how many it printed.
func detectEdges(parent context.Context, lg *slog.Logger, pin *gpiomem.Pin, name string, v flagVars, out io.Writer) (int, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	edges := make(chan pps.CandidateEdge)
	errCh := make(chan error, 1)
	stats := new(pps.PollStats)
	if !lg.Enabled(ctx, slog.LevelInfo) {
		stats = nil
	}
	go func() {
		err := pps.DetectGPIO(ctx, lg, pin, v.cfg, edges, stats)
		stats.Log(lg)
		errCh <- err
	}()
	count := 0
	var outputErr error
	for {
		select {
		case ce := <-edges:
			if outputErr != nil {
				continue
			}
			if err := printEdge(out, name, ce, v.jsonl); err != nil {
				outputErr = err
				cancel()
				continue
			}
			count++
		case err := <-errCh:
			if outputErr != nil {
				return count, fmt.Errorf("writing PPS edge: %w", outputErr)
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return count, nil
			}
			return count, err
		}
	}
}

// gpioEvent is the JSON Lines form of an edge, with the fields of
// satpulsetool serial's PPS edges.
type gpioEvent struct {
	GPIO        string           `json:"gpio"`
	T           string           `json:"t"`
	Uncertainty [2]float64       `json:"uncertainty,omitzero"`
	PollWidths  [2]float64       `json:"pollWidths,omitzero"`
	Reject      pps.RejectReason `json:"reject,omitempty"`
}

// printEdge prints ce, caught on the GPIO called gpio, as its time to the
// microsecond or as a gpioEvent.
func printEdge(out io.Writer, gpio string, ce pps.CandidateEdge, jsonl bool) error {
	t := ce.Timestamp.UTC()
	if !jsonl {
		_, err := fmt.Fprintln(out, t.Round(time.Microsecond).Format("15:04:05.000000"))
		return err
	}
	return json.NewEncoder(out).Encode(&gpioEvent{
		GPIO:        gpio,
		T:           t.Format(time.RFC3339Nano),
		Uncertainty: [2]float64{ce.Uncertainty[0].Seconds(), ce.Uncertainty[1].Seconds()},
		PollWidths:  [2]float64{ce.PollWidths[0].Seconds(), ce.PollWidths[1].Seconds()},
		Reject:      ce.Reject,
	})
}
