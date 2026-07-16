package ubxsim

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/internal/rtcm"
	"github.com/jclark/satpulse/gps/internal/ubx"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
	"github.com/jclark/satpulse/gps/scan"
)

// A Personality is the recorded identity, configuration key inventory
// and nav message bank of one real receiver, captured with the capture
// kit in the capture directory.
type Personality struct {
	MonVer   []byte  // raw MON-VER packet, replayed verbatim
	MonGnss  []byte  // raw MON-GNSS packet, replayed verbatim (nil if not captured)
	Defaults ucv.Map // the Default configuration layer (also the key inventory)
	Epochs   [][]Pkt // per-epoch nav message bank, in recording order
	Skipped  int     // recorded packets with no MSGOUT key, not replayable
}

// Pkt is one recorded packet, gated at replay time by the MSGOUT key it
// maps to and by the port's OUTPROT key for its protocol.
type Pkt struct {
	KeyM ucv.KeyM    // port-neutral MSGOUT key
	Tag  gpsprot.Tag // protocol, selecting the port's OUTPROT key
	Data []byte
}

// LoadPersonality loads a personality file: a raw UBX stream (as
// produced by satpulsetool pack from the capture kit's packet log)
// holding the receiver's MON-VER and MON-GNSS responses and its
// Default-layer CFG-VALGET dump. It loads no nav message bank: without
// one the simulator is silent, like a receiver with no antenna
// connected, but still answers configuration traffic. LoadReplay adds
// the bank.
func LoadPersonality(path string) (*Personality, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	p := &Personality{Defaults: make(ucv.Map)}
	sc := scan.New(f, 4096, []gpsprot.PacketFormat{ubx.PacketFormat})
	for {
		pkt, err := sc.Scan()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %v", path, err)
		}
		if pkt.Format != ubx.PacketFormat || !pkt.ChecksumValid {
			return nil, fmt.Errorf("%s: not a clean UBX stream", path)
		}
		if err := p.addPersonalityMsg(pkt.Data); err != nil {
			return nil, fmt.Errorf("%s: %v", path, err)
		}
	}
	if p.MonVer == nil {
		return nil, fmt.Errorf("%s: no MON-VER", path)
	}
	if len(p.Defaults) == 0 {
		return nil, fmt.Errorf("%s: no Default-layer CFG-VALGET responses", path)
	}
	return p, nil
}

// addPersonalityMsg processes one UBX packet of a personality file.
// The identity messages and the Default-layer dump are loaded;
// anything else (the capture's ACK traffic, other layers) is ignored.
func (p *Personality) addPersonalityMsg(data string) error {
	switch ubxbin.PacketMsgId(data) {
	case ubxbin.MonVerID:
		p.MonVer = []byte(data)
	case ubxbin.MonGnssID:
		p.MonGnss = []byte(data)
	case ubxbin.CfgValgetID:
		m, err := ubxbin.ParseMsg(data)
		if err != nil {
			return err
		}
		vg := m.(*ubxbin.CfgValget)
		if vg.Layer != ubxbin.CfgValgetLayerDefault {
			return nil
		}
		items, err := ucv.UnmarshalItems(vg.CfgData)
		if err != nil {
			return err
		}
		p.Defaults.AddItems(items)
	}
	return nil
}

// LoadReplay loads the personality's nav message bank from a replay
// packet log.
func (p *Personality) LoadReplay(replayPath string) error {
	var last time.Time
	if err := readLog(replayPath, func(e *gpsio.PacketLogEntry) error {
		return p.loadReplayEntry(e, &last)
	}); err != nil {
		return err
	}
	if len(p.Epochs) == 0 {
		return fmt.Errorf("%s: no replayable packets", replayPath)
	}
	return nil
}

func readLog(path string, f func(*gpsio.PacketLogEntry) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	sc.Buffer(nil, 1<<20)
	line := 0
	for sc.Scan() {
		line++
		var e gpsio.PacketLogEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return fmt.Errorf("%s:%d: %v", path, line, err)
		}
		if e.Out {
			continue
		}
		if err := f(&e); err != nil {
			return fmt.Errorf("%s:%d: %v", path, line, err)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("%s: %v", path, err)
	}
	return nil
}

// epochGap is the idle time that separates one recorded epoch burst from
// the next. Recordings are made at a 1 Hz nav rate, so anything well
// below a second and well above a burst works.
const epochGap = 300 * time.Millisecond

func (p *Personality) loadReplayEntry(e *gpsio.PacketLogEntry, last *time.Time) error {
	pkts, err := replayPkts(e)
	if err != nil {
		return err
	}
	if len(pkts) == 0 {
		p.Skipped++
		return nil
	}
	t := time.Time(e.T)
	if len(p.Epochs) == 0 || t.Sub(*last) > epochGap {
		p.Epochs = append(p.Epochs, nil)
	}
	*last = t
	i := len(p.Epochs) - 1
	p.Epochs[i] = append(p.Epochs[i], pkts...)
	return nil
}

// replayPkts maps a recorded packet to its bank entries: the packet
// itself gated by its MSGOUT key, plus, for an RTCM MSM7 message, a
// derived MSM4 variant gated by the corresponding MSM4 key. A u-blox
// receiver cannot output MSM4 and MSM7 simultaneously, so the
// recording contains MSM7 only and the MSM4 content must be
// generated. Packets with no MSGOUT key map to nothing.
func replayPkts(e *gpsio.PacketLogEntry) ([]Pkt, error) {
	data := e.Data()
	switch e.Tag {
	case ubx.Tag:
		if km, ok := msgOutKey[ubxbin.PacketMsgId(data)]; ok {
			return []Pkt{{KeyM: km, Tag: ubx.Tag, Data: []byte(data)}}, nil
		}
	case nmea.Tag:
		if i := strings.IndexByte(data, ','); i == 6 && data[0] == '$' && data[1] != 'P' {
			if km, ok := nmeaOutKey[data[3:6]]; ok {
				return []Pkt{{KeyM: km, Tag: nmea.Tag, Data: []byte(data)}}, nil
			}
		}
	case rtcm.Tag:
		return rtcmReplayPkts(data)
	}
	return nil, nil
}

func rtcmReplayPkts(data string) ([]Pkt, error) {
	mt, sub, hasSub := rtcmbin.ExtractMsgTypeSubtype(data)
	if hasSub {
		km := ucv.KRtcm3xType40720
		if sub == 1 {
			km = ucv.KRtcm3xType40721
		} else if sub != 0 {
			return nil, nil
		}
		return []Pkt{{KeyM: km, Tag: rtcm.Tag, Data: []byte(data)}}, nil
	}
	km, ok := rtcmOutKey[mt]
	if !ok {
		return nil, nil
	}
	pkts := []Pkt{{KeyM: km, Tag: rtcm.Tag, Data: []byte(data)}}
	if km4, ok := rtcmOutKey[mt-3]; ok && mt.IsMSM() && mt%10 == 7 {
		out, err := rtcmbin.MSM7ConvertPacket(data, 4)
		if err != nil {
			return nil, fmt.Errorf("MSM7 to MSM4: %v", err)
		}
		pkts = append(pkts, Pkt{KeyM: km4, Tag: rtcm.Tag, Data: []byte(out)})
	}
	return pkts, nil
}

var msgOutKey = map[ubxbin.MsgID]ucv.KeyM{
	ubxbin.NavSatID:       ucv.KUbxNavSat,
	ubxbin.NavSvinID:      ucv.KUbxNavSvin,
	ubxbin.NavTimeGPSID:   ucv.KUbxNavTimegps,
	ubxbin.NavTimeGalID:   ucv.KUbxNavTimegal,
	ubxbin.NavTimeGLOID:   ucv.KUbxNavTimeglo,
	ubxbin.NavTimeBDSID:   ucv.KUbxNavTimebds,
	ubxbin.NavTimeNavICID: ucv.KUbxNavTimenavic,
	ubxbin.NavTimeUTCID:   ucv.KUbxNavTimeutc,
	ubxbin.NavTimeLSID:    ucv.KUbxNavTimels,
	ubxbin.NavPVTID:       ucv.KUbxNavPvt,
	ubxbin.NavHPPosLLHID:  ucv.KUbxNavHpposllh,
	ubxbin.NavDOPID:       ucv.KUbxNavDop,
	ubxbin.NavEOEID:       ucv.KUbxNavEoe,
	ubxbin.NavSigID:       ucv.KUbxNavSig,
	ubxbin.TimSvinID:      ucv.KUbxTimSvin,
	ubxbin.TimTPID:        ucv.KUbxTimTp,
}

var rtcmOutKey = map[rtcmbin.MsgType]ucv.KeyM{
	1005: ucv.KRtcm3xType1005,
	1074: ucv.KRtcm3xType1074,
	1077: ucv.KRtcm3xType1077,
	1084: ucv.KRtcm3xType1084,
	1087: ucv.KRtcm3xType1087,
	1094: ucv.KRtcm3xType1094,
	1097: ucv.KRtcm3xType1097,
	1124: ucv.KRtcm3xType1124,
	1127: ucv.KRtcm3xType1127,
	1230: ucv.KRtcm3xType1230,
}

var nmeaOutKey = map[string]ucv.KeyM{
	"GGA": ucv.KNmeaIdGga,
	"GLL": ucv.KNmeaIdGll,
	"GSA": ucv.KNmeaIdGsa,
	"GSV": ucv.KNmeaIdGsv,
	"RMC": ucv.KNmeaIdRmc,
	"VTG": ucv.KNmeaIdVtg,
	"ZDA": ucv.KNmeaIdZda,
	"GNS": ucv.KNmeaIdGns,
}
