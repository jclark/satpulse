package mon

import (
	"context"

	"github.com/jclark/gps2phc/internal/pmc"
	"golang.org/x/exp/slog"
)

// The updating goroutine must wait for a response to each request before sending another request.
func PTP4LWorker(ctx context.Context, client *pmc.Client, reqCh <-chan GrandmasterUpdateRequest, lg *slog.Logger) {
	lg.Debug("PTP4LWorker", "state", "started")
	for {
		req, ok := <-reqCh
		if !ok {
			break
		}
		req.resp <- sendRecv(ctx, client, req.props, lg)
	}
	lg.Debug("PTP4LWorker", "state", "exiting")
}

func sendRecv(ctx context.Context, client *pmc.Client, props GrandmasterProps, lg *slog.Logger) *GrandmasterProps {
	msg := pmc.NewMgmtSetMsg(props.Settings())
	if ctx.Err() != nil {
		return nil
	}
	seqid, err := client.Send(msg)
	if err != nil {
		lg.Debug("pmcSend", "err", err)
		return nil
	}
	if ctx.Err() != nil {
		return nil
	}
	respMsg, err := client.Recv(seqid)
	if err != nil {
		lg.Debug("pmcRecv", "err", err)
		return nil
	}
	switch respMsg := respMsg.(type) {
	case *pmc.MgmtErrorStatusMsg:
		lg.Debug("pmcSendRecv", "mgmtErr", respMsg.V.MgmtErrorID.String())
		return nil
	case *pmc.GrandmasterSettingsMsg:
		lg.Debug("gmUpdated", "clockClass", props.ClockClass, "clockAccuracy", props.ClockAccuracy,
			"utcOffset", props.UTCOffset, "leapTonight", props.LeapTonight)
		return &props
	}
	lg.Debug("pmcSendRecv", "unexpectedResponse", respMsg)
	return nil
}
