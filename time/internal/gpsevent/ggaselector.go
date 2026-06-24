package gpsevent

import (
	"strings"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/scan"
)

// GGASelector chooses original receiver GGA over synthesized GGA for the same UTC.
type GGASelector struct {
	ch              chan scan.Packet
	lastOriginalUTC string
}

// NewGGASelector creates a selected-GGA selector with a capacity-1 output channel.
func NewGGASelector() *GGASelector {
	return &GGASelector{ch: make(chan scan.Packet, 1)}
}

// Packets returns the selected GGA packet channel.
func (s *GGASelector) Packets() <-chan scan.Packet {
	return s.ch
}

// OriginalGGA validates and publishes an original receiver GGA packet.
func (s *GGASelector) OriginalGGA(pkt scan.Packet) bool {
	if !validOriginalGGA(pkt) {
		return false
	}
	s.publish(pkt)
	if utc, ok := ggaUTC(pkt.Data); ok {
		s.lastOriginalUTC = utc
	}
	return true
}

// GGASentence publishes a synthesized GGA unless it matches the last original UTC.
func (s *GGASelector) GGASentence(gga nmeamsg.GGASentence) {
	b, err := nmeamsg.SerializeMsg(gga)
	if err != nil {
		return
	}
	pkt := scan.Packet{
		Format:        gpsreg.NMEAPacketFormat,
		Data:          string(b),
		ChecksumValid: true,
	}
	if utc, ok := ggaUTC(pkt.Data); ok && utc == s.lastOriginalUTC {
		return
	}
	s.publish(pkt)
}

func (s *GGASelector) publish(pkt scan.Packet) {
	select {
	case s.ch <- pkt:
		return
	default:
	}
	select {
	case <-s.ch:
	default:
	}
	select {
	case s.ch <- pkt:
	default:
	}
}

func validOriginalGGA(pkt scan.Packet) bool {
	if !pkt.HasTag(gpsreg.TagNMEA) || !pkt.ChecksumValid {
		return false
	}
	flags := nmeamsg.CheckSyntax(pkt.Data)
	if !flags.IsValidGNSSTalkerNMEA() {
		return false
	}
	i := strings.IndexByte(pkt.Data, '*')
	if i < 0 {
		return false
	}
	msg, err := nmeamsg.ParseGNSSTalkerPayload(pkt.Data[1:i], flags)
	if err != nil {
		return false
	}
	_, ok := msg.(nmeamsg.GGASentence)
	return ok
}

func ggaUTC(data string) (string, bool) {
	if len(data) < 13 || data[0] != '$' || data[3:7] != "GGA," {
		return "", false
	}
	for i := 7; i < 13; i++ {
		if data[i] < '0' || data[i] > '9' {
			return "", false
		}
	}
	return data[7:13], true
}
