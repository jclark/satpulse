package gpsreg

import (
	"strings"

	"github.com/jclark/satpulse/internal/as"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/rtcm"
	"github.com/jclark/satpulse/internal/ubx"
)

// PacketFormats contains all known packet formats
var PacketFormats = []gpsprot.PacketFormat{
	ubx.PacketFormat,
	nmea.PacketFormat,
	rtcm.PacketFormat,
}

// CreatePacketProcessors creates packet processors for all registered protocols
func CreatePacketProcessors(nmeaNumbering []gpsprot.NMEASVNumberingRange) map[gpsprot.Tag]gpsprot.PacketProcessor {
	nmeaPP := nmea.NewPacketProcessor()
	if nmeaNumbering != nil {
		nmeaPP.SetSVNumbering(nmeaNumbering)
	}
	return map[gpsprot.Tag]gpsprot.PacketProcessor{
		ubx.Tag:  ubx.NewPacketProcessor(),
		nmea.Tag: nmeaPP,
		rtcm.Tag: rtcm.NewPacketProcessor(),
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
