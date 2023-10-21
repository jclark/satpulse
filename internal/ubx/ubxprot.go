package ubx

import (
	"log/slog"
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
	"github.com/jclark/gps4ptp/internal/ubx/cfg"
)

type Protocol struct {
	curPort *bin.PortID
	ver     *Version
	acks    []*Ack
	ubxMsg  [nPort]uint16
}

type Ack struct {
	msgID bin.MsgID
	OK    bool
	TRead time.Time
}

func (prot *Protocol) ProcessPacket(data string, tRead time.Time, cfg *RawConfig, h gpsmsg.Handler, ph ProtHandler, lg *slog.Logger) error {
	m, err := bin.ParseMsg(data)
	if err != nil {
		return err
	}
	if cfg.AddMsg(m) {
		return nil
	}
	if Dispatch(m, tRead, h) {
		return nil
	}
	switch mt := m.(type) {
	case *bin.AckAck:
		prot.ack(mt.MsgID, true, tRead)
	case *bin.AckNak:
		prot.ack(mt.MsgID, false, tRead)
	case *bin.MonVer:
		prot.ver = monVer(mt)
		if cfg != nil {
			cfg.SetVersion(prot.ver)
		}
	case *bin.MonMsgPP:
		prot.msgPP(mt, lg)
	default:
		if ph != nil {
			ph.UBX(m, tRead)
		}
	}
	return nil
}

func (prot *Protocol) CurPortID() *bin.PortID {
	return prot.curPort
}

func (prot *Protocol) PollPort() []byte {
	return bin.Poll(bin.MonMsgPPID)
}

func (prot *Protocol) msgPP(mt *bin.MonMsgPP, lg *slog.Logger) {
	lg.Debug("got UBX-MON-MSGPP message")
	if prot.curPort != nil {
		return
	}
	incIndex := -1
	// The logic here should work for the first MSGPP message if only one port is connected.
	// It should work for the second MSGPP message if only one port is _in use_.
	for i, msgCount := range mt.Msg {
		curCount := msgCount[0]
		prevCount := prot.ubxMsg[i]
		prot.ubxMsg[i] = curCount
		if curCount > prevCount {
			if incIndex < 0 {
				incIndex = i
			} else {
				incIndex = -1
			}
		}
	}
	if incIndex >= 0 {
		port := bin.PortID(incIndex)
		lg.Debug("discovered current port", "port", port)
		prot.curPort = &port
	}
}

func (prot *Protocol) ack(msgID bin.MsgID, ok bool, t time.Time) {
	prot.acks = append(prot.acks, &Ack{msgID, ok, t})
}

func (prot *Protocol) Version() *Version {
	return prot.ver
}

func (prot *Protocol) FindAck(packet []byte, tSent time.Time) (ack *Ack) {
	return prot.FindAckByMsgId(bin.PacketMsgId(packet), tSent)
}

func (prot *Protocol) FindAckByMsgId(msgID bin.MsgID, tSent time.Time) (ack *Ack) {
	stale := 0
	for i, a := range prot.acks {
		if !a.TRead.After(tSent) {
			stale = i + 1
		} else if a.msgID == msgID {
			ack = a
			break
		}
	}
	if stale > 0 {
		prot.acks = prot.acks[stale:]
	}
	return
}

func (prot *Protocol) PollVersion() []byte {
	return bin.Poll(bin.MonVerID)
}

func (prot *Protocol) PollLeapSecond() []byte {
	return bin.Poll(bin.NavTimeLSID)
}

func (prot *Protocol) TPTimegridGPS() []byte {
	cfgMap := map[string]map[string]any{
		"TP": {
			"TIMEGRID_TP1": "GPS",
		},
	}
	u := bin.CfgValset{
		CfgValsetFixed: bin.CfgValsetFixed{
			Layers: bin.CfgValsetLayerRAM,
		},
		CfgData: cfg.GetSchema().MustMarshal(cfgMap),
	}
	bytes, err := bin.Serialize(&u)
	if err != nil {
		panic(err)
	}
	return bytes
}

func (prot *Protocol) GetterMsgs() [][]byte {
	packets := [][]byte{
		bin.Poll(bin.CfgGNSSID),
		bin.Poll(bin.CfgRateID),
		bin.Poll(bin.CfgTp5ID),
		bin.Poll(bin.CfgNav5ID),
	}
	switch prot.productCategory() {
	case "TIM", "FTS":
		packets = append(packets, bin.Poll(bin.CfgTmode2ID))
	case "HPG":
		packets = append(packets, bin.Poll(bin.CfgTmode3ID))
	}
	return packets
}

func (prot *Protocol) PollSurvey() []byte {
	switch prot.productCategory() {
	case "TIM", "FTS":
		return bin.Poll(bin.TimSvinID)
	case "HPG":
		return bin.Poll(bin.NavSvinID)
	}
	return nil
}

func (prot *Protocol) productCategory() string {
	if prot.ver != nil && prot.ver.FW != nil {
		return prot.ver.FW.ProductCategory
	}
	return ""
}
