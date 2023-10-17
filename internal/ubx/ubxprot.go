package ubx

import (
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
	"github.com/jclark/gps4ptp/internal/ubx/cfg"
)

type Protocol struct {
	ver  *Version
	acks []*Ack
}

type Ack struct {
	msgID bin.MsgID
	OK    bool
	TRead time.Time
}

func (prot *Protocol) ProcessPacket(data string, tRead time.Time, cfg *Config, h gpsmsg.Handler, ph ProtHandler) error {
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
		if cfg != nil && prot.ver.Prot != nil {
			cfg.SetProtVer(*prot.ver.Prot)
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

func (prot *Protocol) PollSurveyIn() []byte {
	svinID := bin.TimSvinID
	if prot.productCategory() == "HPG" {
		svinID = bin.NavSvinID
	}
	return bin.Poll(svinID)
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
	}
	tmodeID := bin.CfgTmode2ID
	if prot.productCategory() == "HPG" {
		tmodeID = bin.CfgTmode3ID
	}
	packets = append(packets, bin.Poll(tmodeID))
	return packets
}

func (prot *Protocol) productCategory() string {
	if prot.ver != nil && prot.ver.FW != nil {
		return prot.ver.FW.ProductCategory
	}
	return ""
}
