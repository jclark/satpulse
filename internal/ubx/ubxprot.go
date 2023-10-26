package ubx

import (
	"log/slog"
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
	"github.com/jclark/gps4ptp/internal/ubx/cfg"
)

type Protocol struct {
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
	default:
		if ph != nil {
			ph.UBX(m, tRead)
		}
	}
	return nil
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
		bin.Poll(bin.CfgPrtID),
		bin.Poll(bin.CfgGNSSID),
		bin.Poll(bin.CfgRateID),
		bin.Poll(bin.CfgNav5ID),
	}
	tpIdx := 0
	switch prot.productCategory() {
	case "FTS":
		tpIdx = 1
		fallthrough
	case "TIM":
		packets = append(packets, bin.Poll(bin.CfgTmode2ID))
	case "HPG":
		packets = append(packets, bin.Poll(bin.CfgTmode3ID))
	}
	packets = append(packets, bin.PollCfgTp5(tpIdx))
	return packets
}

func (prot *Protocol) EnableTimeMsgs() [][]byte {
	if prot.productCategory() == "FTS" {
		return [][]byte{bin.SetRate(bin.TimTosID, 1)}
	} else {
		return [][]byte{
			bin.SetRate(bin.NavTimeGPSID, 1),
			bin.SetRate(bin.TimTPID, 1),
		}
	}
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
