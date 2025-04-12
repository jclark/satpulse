package gpsreg

import (
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
func CreatePacketProcessors() map[gpsprot.Tag]gpsprot.PacketProcessor {
	return map[gpsprot.Tag]gpsprot.PacketProcessor{
		ubx.Tag:  ubx.NewPacketProcessor(),
		nmea.Tag: nmea.NewPacketProcessor(),
		rtcm.Tag: rtcm.NewPacketProcessor(),
	}
}
