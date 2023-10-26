package ubx

import (
	"log/slog"
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
	"github.com/jclark/gps4ptp/internal/ubx/cfg"
)

// XXX this probably isn't the right name any more
type Protocol struct {
	ver  *Version
	acks []*Ack
	cfg  RawConfig
	step int
}

type Ack struct {
	msgID bin.MsgID
	OK    bool
	TRead time.Time
}

var _ gpsmsg.Configurator = &Protocol{}

func (prot *Protocol) ProcessPacket(data string, tRead time.Time, h gpsmsg.Handler, ph ProtHandler, lg *slog.Logger) error {
	m, err := bin.ParseMsg(data)
	if err != nil {
		return err
	}
	if prot.cfg.AddMsg(m) {
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
	default:
		if ph != nil {
			ph.UBX(m, tRead)
		}
	}
	return nil
}

func (prot *Protocol) Config() *gpsmsg.Config {
	return prot.cfg.Config(prot.ver)
}

var configSteps = []func(*Protocol) gpsmsg.ConfigRequest{
	(*Protocol).pollPrt,
	(*Protocol).pollGNSS,
	(*Protocol).pollTp5,
	(*Protocol).pollTmode,
	(*Protocol).pollRate,
	(*Protocol).pollNav5,
	(*Protocol).pollSurvey,
	(*Protocol).pollLeapSecond,
	(*Protocol).enableTimePulseMsg,
	(*Protocol).enableTimeGNSSMsg,
}

func (prot *Protocol) NextRequest() gpsmsg.ConfigRequest {
	for prot.step < len(configSteps) {
		req := configSteps[prot.step](prot)
		prot.step++
		if req != nil {
			return req
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

// XXX need to move this into something at the gpsmsg level
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

func (prot *Protocol) ProbePacket() []byte {
	return bin.Poll(bin.MonVerID)
}

func (prot *Protocol) ProbeOK() bool {
	return prot.ver != nil
}

// XXX this is old code; needs to be generalized into support for new style UBX config
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

func (prot *Protocol) pollPrt() gpsmsg.ConfigRequest {
	return pollRequest{bin.CfgPrtID}
}

func (prot *Protocol) pollGNSS() gpsmsg.ConfigRequest {
	return pollRequest{bin.CfgGNSSID}
}

func (prot *Protocol) pollRate() gpsmsg.ConfigRequest {
	return pollRequest{bin.CfgRateID}
}

func (prot *Protocol) pollNav5() gpsmsg.ConfigRequest {
	return pollRequest{bin.CfgNav5ID}
}

func (prot *Protocol) pollTmode() gpsmsg.ConfigRequest {
	switch prot.productCategory() {
	case "FTS", "TIM":
		return pollRequest{bin.CfgTmode2ID}
	case "HPG":
		return pollRequest{bin.CfgTmode3ID}
	}
	return nil
}

func (prot *Protocol) pollTp5() gpsmsg.ConfigRequest {
	tpIdx := 0
	if prot.productCategory() == "FTS" {
		tpIdx = 1
	}
	return pollTp5Request{
		pollRequest: pollRequest{bin.CfgTp5ID},
		tpIdx:       tpIdx,
	}
}

func (prot *Protocol) enableTimePulseMsg() gpsmsg.ConfigRequest {
	if prot.productCategory() == "FTS" {
		return prot.enableMsgRequest(bin.TimTosID)
	} else {
		return prot.enableMsgRequest(bin.TimTPID)
	}
}

func (prot *Protocol) enableTimeGNSSMsg() gpsmsg.ConfigRequest {
	if prot.productCategory() == "FTS" {
		return nil
	}
	return prot.enableMsgRequest(bin.NavTimeGPSID)
}

// XXX not clear what to do about waiting for response NAV-TIMELS response
// we don't have to wait for the response (unlike with CFG messages)
func (prot *Protocol) pollLeapSecond() gpsmsg.ConfigRequest {
	return pollRequest{bin.NavTimeLSID}
}

// XXX same problem as with pollLeapSecond
func (prot *Protocol) pollSurvey() gpsmsg.ConfigRequest {
	switch prot.productCategory() {
	case "TIM", "FTS":
		return pollRequest{bin.TimSvinID}
	case "HPG":
		return pollRequest{bin.NavSvinID}
	}
	return nil
}

type pollRequest struct {
	msgID bin.MsgID
}

func (r pollRequest) Packet() []byte {
	return bin.Poll(r.msgID)
}

func (r pollRequest) ID() string { return r.msgID.String() }

func (r pollRequest) Ack(_ bool) {}

func (r pollRequest) Ackable() bool {
	return r.msgID.Ackable()
}

type pollTp5Request struct {
	pollRequest
	tpIdx int
}

func (r pollTp5Request) Packet() []byte {
	return bin.PollCfgTp5(r.tpIdx)
}

func (prot *Protocol) enableMsgRequest(msgID bin.MsgID) gpsmsg.ConfigRequest {
	return enableMsgRequest{&prot.cfg, msgID}
}

type enableMsgRequest struct {
	raw   *RawConfig
	msgID bin.MsgID
}

func (r enableMsgRequest) Packet() []byte {
	return bin.SetCfgMsg(r.msgID, 1)
}

func (r enableMsgRequest) Ack(ok bool) {
	if ok {
		r.raw.SetMsgRate(r.msgID, 1)
	}
}

func (r enableMsgRequest) Ackable() bool { return true }

func (r enableMsgRequest) ID() string { return bin.CfgMsgID.String() }

func (prot *Protocol) productCategory() string {
	if prot.ver != nil && prot.ver.FW != nil {
		return prot.ver.FW.ProductCategory
	}
	return ""
}
