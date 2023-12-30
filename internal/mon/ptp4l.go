package mon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jclark/satpulse/internal/pmc"
	"golang.org/x/sys/unix"
)

// Time to send and receive a PTP management message to/from ptp4l.
const ptp4lTimeout = time.Second / 10

// The updating goroutine must wait for a response to each request before sending another request.
func PTP4LWorker(ctx context.Context, client *pmc.Client, reqCh <-chan GrandmasterUpdateRequest, lg *slog.Logger) {
	lg.Debug("the PTP management goroutine has started")
	sockExists := true
	for {
		req, ok := <-reqCh
		if !ok {
			break
		}
		props := req.props
		tStart := time.Now()
		client.T.Conn.SetDeadline(tStart.Add(ptp4lTimeout))
		err := sendRecv(ctx, client, props)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				if sockExists {
					lg.Info("the ptp4l management socket does not exist (ptp4l not running)", "path", client.T.RemoteAddr)
					sockExists = false
				}
			} else if errors.Is(err, os.ErrDeadlineExceeded) && sockExists {
				lg.Info("timeout on ptp4l management socket", "path", client.T.RemoteAddr)
			} else if ctx.Err() == nil {
				lg.Warn("error while updating the PTP grandmaster using PTP management protocol", "err", err)
			}
		} else {
			if !sockExists {
				lg.Info("the ptp4l management socket now exists", "path", client.T.RemoteAddr)
				sockExists = true
			}
			lg.Info("successfully updated the grandmaster using the PTP managment protocol", "clockClass", props.ClockClass, "clockAccuracy", props.ClockAccuracy,
				"utcOffset", props.UTCOffset, "leapTonight", props.LeapTonight, "responseTime", time.Since(tStart))
			req.resp <- props
		}
		close(req.resp)
	}
	lg.Debug("the PTP4L worker is about to close the PTP management client")
	err := client.Close()
	if err != nil {
		lg.Warn("error while closing the PTP management client", "err", err)
	}
	lg.Debug("the PTP4L worker is about to exit")
}

func sendRecv(ctx context.Context, client *pmc.Client, props GrandmasterProps) error {
	msg := pmc.NewMgmtSetMsg(props.Settings())
	if ctx.Err() != nil {
		return ctx.Err()
	}
	seqid, err := client.Send(msg)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	respMsg, err := client.Recv(seqid)
	if err != nil {
		return err
	}
	switch respMsg := respMsg.(type) {
	case *pmc.MgmtErrorStatusMsg:
		return fmt.Errorf("PTP management error while updating grandmaster: %s", respMsg.V.MgmtErrorID.String())
	case *pmc.GrandmasterSettingsMsg:
		return nil
	}
	return fmt.Errorf("unexpected PTP management response type: %T", respMsg)
}
