package mon

import (
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

// PTP4LWorker processes GrandmasterUpdateRequests and sends them to ptp4l.
// It should be run in a goroutine.
// It returns when reqCh is closed.
// The updating goroutine must wait for a response to each request before sending another request.
func PTP4LWorker(client *pmc.Client, reqCh <-chan GrandmasterUpdateRequest, lg *slog.Logger) {
	// This doesn't take a context because a request is sent after the context is canceled,
	// to set ptp4l to a non-synced state. 
	lg.Debug("the PTP management goroutine has started")
	sockOK := true
	for {
		req, ok := <-reqCh
		if !ok {
			break
		}
		props := req.props
		tStart := time.Now()
		client.T.Conn.SetDeadline(tStart.Add(ptp4lTimeout))
		err := sendRecv(client, props)
		if err != nil {
			if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ECONNREFUSED) {
				if sockOK {
					lg.Info("the ptp4l management socket is not ready", "path", client.T.RemoteAddr)
					sockOK = false
				}
			} else if errors.Is(err, os.ErrDeadlineExceeded) && sockOK {
				lg.Info("timeout on ptp4l management socket", "path", client.T.RemoteAddr)
			} else {
				lg.Warn("error while updating the PTP grandmaster using PTP management protocol", "err", err)
			}
		} else {
			if !sockOK {
				lg.Info("the ptp4l management socket has become ready", "path", client.T.RemoteAddr)
				sockOK = true
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

func sendRecv(client *pmc.Client, props GrandmasterProps) error {
	msg := pmc.NewMgmtSetMsg(props.Settings())
	seqid, err := client.Send(msg)
	if err != nil {
		return err
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
