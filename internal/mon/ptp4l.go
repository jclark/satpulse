package mon

import (
	"context"
	"errors"
	"fmt"

	"github.com/jclark/gps4ptp/internal/pmc"
	"golang.org/x/exp/slog"
	"golang.org/x/sys/unix"
)

// The updating goroutine must wait for a response to each request before sending another request.
func PTP4LWorker(ctx context.Context, client *pmc.Client, reqCh <-chan GrandmasterUpdateRequest, lg *slog.Logger) {
	lg.Debug("PTP4LWorker", "state", "started")
	socketOK := true
	for {
		req, ok := <-reqCh
		if !ok {
			break
		}
		props := req.props
		err := sendRecv(ctx, client, props)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				if socketOK {
					lg.Info("ptpMgmt", "socketPathDoesNotExistYet", client.T.RemoteAddr)
					socketOK = false
				}
			} else if ctx.Err() == nil {
				lg.Info("ptpMgmt", "err", err)
			}
			req.resp <- nil
		} else {
			if !socketOK {
				lg.Info("ptpMgmt", "socketPathNowOK", client.T.RemoteAddr)
				socketOK = true
			}
			lg.Info("gmUpdated", "clockClass", props.ClockClass, "clockAccuracy", props.ClockAccuracy,
				"utcOffset", props.UTCOffset, "leapTonight", props.LeapTonight)
			req.resp <- &props
		}
	}
	lg.Debug("PTP4LWorker", "state", "exiting")
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
