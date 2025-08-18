package gpsreg

import (
	"fmt"
	"strings"

	"github.com/jclark/satpulse/internal/as"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/rtcm"
	"github.com/jclark/satpulse/internal/ubx"
	"github.com/jclark/satpulse/internal/unc"
)

// PacketFormats contains all known packet formats
var PacketFormats = []gpsprot.PacketFormat{
	ubx.PacketFormat,
	nmea.PacketFormat,
	rtcm.PacketFormat,
	unc.BinPacketFormat,
	unc.AsciiPacketFormat,
}

// CreatePacketProcessors creates packet processors for all registered protocols
func CreatePacketProcessors(nmeaNumbering []gpsprot.NMEASVNumberingRange) map[gpsprot.Tag]gpsprot.PacketProcessor {
	nmeaPP := nmea.NewPacketProcessor()
	if nmeaNumbering != nil {
		nmeaPP.SetSVNumbering(nmeaNumbering)
	}
	return map[gpsprot.Tag]gpsprot.PacketProcessor{
		ubx.Tag:       ubx.NewPacketProcessor(),
		nmea.Tag:      nmeaPP,
		rtcm.Tag:      rtcm.NewPacketProcessor(),
		unc.TagBinary: unc.NewBinPacketProcessor(),
		unc.TagAscii:  unc.NewAsciiPacketProcessor(),
	}
}

// CreateConfigProtocols creates all available configuration protocols
func CreateConfigProtocols() []gpsprot.ConfigProtocol {
	return []gpsprot.ConfigProtocol{
		ubx.NewConfigProtocol(),
	}
}

func FindNMEASVNumbering(manu string) []gpsprot.NMEASVNumberingRange {
	switch strings.ToLower(manu) {
	case "allystar", "as":
		return as.NewNMEASVNumbering()
	case "ublox", "u-blox", "ubx":
		return ubx.NewNMEASVNumbering()
	default:
		return nil
	}
}

// Protocol is like gpsprot.Tag but implements encoding.TextUnmarshaler
// and enforces that the tag is one of the known protocols.
type Protocol gpsprot.Tag

func (prot Protocol) Tag() gpsprot.Tag {
	return gpsprot.Tag(prot)
}

func (prot *Protocol) UnmarshalText(data []byte) error {
	s := string(data)
	switch strings.ToUpper(s) {
	case "UBX":
		*prot = Protocol(ubx.Tag)
	case "NMEA":
		*prot = Protocol(nmea.Tag)
	case "RTCM":
		*prot = Protocol(rtcm.Tag)
	case "":
		*prot = Protocol("")
	default:
		return fmt.Errorf("unknown protocol: %s", s)
	}
	return nil
}
