package stream

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/nmeasyn"
	"github.com/jclark/satpulse/gps/scan"
)

func TestGGASelectorPacketForwarded(t *testing.T) {
	s := NewGGASelector()
	pkt := ggaPacket("GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,")
	if !s.Packet(pkt) {
		t.Fatal("Packet returned false")
	}
	if got := oneSelectedGGA(t, s); got.Data != pkt.Data {
		t.Fatalf("selected GGA = %q, want %q", got.Data, pkt.Data)
	}
}

func TestGGASelectorPacketSuppressesSynthSameUTC(t *testing.T) {
	s := NewGGASelector()
	if !s.Packet(ggaPacket("GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,")) {
		t.Fatal("Packet returned false")
	}
	oneSelectedGGA(t, s)
	s.Msg(nmeamsg.MakeGGA("GN", time.Date(2026, 6, 24, 12, 35, 19, 0, time.UTC), &[2]float64{1, 2}, nil, nil, 1, pu8(8), pf64(0.9)), nmeasyn.PhaseEpoch)
	if pkt, ok := trySelectedGGA(s); ok {
		t.Fatalf("selected synthesized same-UTC GGA %q, want none", pkt.Data)
	}
}

func TestGGASelectorPacketEmptyUTCDoesNotSuppressSynth(t *testing.T) {
	s := NewGGASelector()
	if !s.Packet(ggaPacket("GPGGA,,,,,,0,00,99.99,,,,,,")) {
		t.Fatal("Packet returned false")
	}
	oneSelectedGGA(t, s)
	s.Msg(nmeamsg.MakeGGA("GN", time.Date(2026, 6, 24, 12, 35, 19, 0, time.UTC), &[2]float64{1, 2}, nil, nil, 1, pu8(8), pf64(0.9)), nmeasyn.PhaseEpoch)
	if got := oneSelectedGGA(t, s); got.Format != gpsreg.NMEAPacketFormat {
		t.Fatalf("Format = %v, want NMEA", got.Format)
	}
}

func TestGGASelectorSynthDifferentUTCForwarded(t *testing.T) {
	s := NewGGASelector()
	if !s.Packet(ggaPacket("GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,")) {
		t.Fatal("Packet returned false")
	}
	oneSelectedGGA(t, s)
	s.Msg(nmeamsg.MakeGGA("GN", time.Date(2026, 6, 24, 12, 35, 20, 0, time.UTC), &[2]float64{1, 2}, nil, nil, 1, pu8(8), pf64(0.9)), nmeasyn.PhaseEpoch)
	if got := oneSelectedGGA(t, s); !hasGGAUTC(got.Data, "123520") {
		t.Fatalf("selected GGA = %q, want UTC 123520", got.Data)
	}
}

func TestGGASelectorSynthPacketValid(t *testing.T) {
	s := NewGGASelector()
	s.Msg(nmeamsg.MakeGGA("GN", time.Date(2026, 6, 24, 12, 35, 19, 0, time.UTC), &[2]float64{13.731826167, 100.644802333}, nil, nil, 1, pu8(8), pf64(1.2)), nmeasyn.PhaseEpoch)
	pkt := oneSelectedGGA(t, s)
	if pkt.Format != gpsreg.NMEAPacketFormat || !pkt.ChecksumValid {
		t.Fatalf("packet format/checksum = %v/%v, want NMEA/true", pkt.Format, pkt.ChecksumValid)
	}
	b := []byte(pkt.Data)
	if !bytes.Equal(pkt.Format.ComputeChecksum(b), pkt.Format.ExtractChecksum(b)) {
		t.Fatalf("bad checksum in synthesized packet %q", pkt.Data)
	}
	if !nmeamsg.CheckSyntax(pkt.Data).IsValidGNSSTalkerNMEA() {
		t.Fatalf("synthesized packet is not valid GNSS NMEA: %q", pkt.Data)
	}
}

func TestGGASelectorIgnoresImmediatePhase(t *testing.T) {
	s := NewGGASelector()
	s.Msg(nmeamsg.MakeGGA("GN", time.Date(2026, 6, 24, 12, 35, 19, 0, time.UTC), &[2]float64{1, 2}, nil, nil, 1, pu8(8), pf64(1.2)), nmeasyn.PhaseImmediate)
	if pkt, ok := trySelectedGGA(s); ok {
		t.Fatalf("selected immediate-phase GGA %q, want none", pkt.Data)
	}
}

func TestGGASelectorIgnoresNonGGA(t *testing.T) {
	s := NewGGASelector()
	s.Msg(parseSentence(t, "GPRMC,123519,A,4807.038,N,01131.000,E,0,,240626,,,A,V"), nmeasyn.PhaseEpoch)
	if pkt, ok := trySelectedGGA(s); ok {
		t.Fatalf("selected non-GGA sentence %q, want none", pkt.Data)
	}
}

func TestGGASelectorRejectsInvalidPacket(t *testing.T) {
	tests := []scan.Packet{
		{Format: gpsreg.NMEAPacketFormat, Data: ggaPacket("GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,").Data},
		nmeaPacket("GPRMC,123519,A,4807.038,N,01131.000,E,0,,240626,,,A,V"),
		nmeaPacket("GPGGA,123519,4807.038,N,01131.000,E,1"),
	}
	for _, pkt := range tests {
		s := NewGGASelector()
		if s.Packet(pkt) {
			t.Fatalf("Packet(%q) returned true, want false", pkt.Data)
		}
		if pkt, ok := trySelectedGGA(s); ok {
			t.Fatalf("selected invalid packet %q", pkt.Data)
		}
	}
}

func TestGGASelectorLatestWins(t *testing.T) {
	s := NewGGASelector()
	oldPkt := ggaPacket("GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,")
	newPkt := ggaPacket("GPGGA,123520,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,")
	if !s.Packet(oldPkt) || !s.Packet(newPkt) {
		t.Fatal("Packet returned false")
	}
	if got := oneSelectedGGA(t, s); got.Data != newPkt.Data {
		t.Fatalf("selected GGA = %q, want newest %q", got.Data, newPkt.Data)
	}
}

func ggaPacket(payload string) scan.Packet {
	return nmeaPacket(payload)
}

func nmeaPacket(payload string) scan.Packet {
	return scan.Packet{
		Format:        gpsreg.NMEAPacketFormat,
		Data:          fmt.Sprintf("$%s*%02X\r\n", payload, nmeamsg.Checksum([]byte(payload))),
		ChecksumValid: true,
	}
}

func parseSentence(t *testing.T, payload string) nmeamsg.GNSSTalkerIDMsg {
	t.Helper()
	wire := fmt.Sprintf("$%s*%02X\r\n", payload, nmeamsg.Checksum([]byte(payload)))
	msg, err := nmeamsg.ParseGNSSTalkerPayload(payload, nmeamsg.CheckSyntax(wire))
	if err != nil {
		t.Fatalf("ParseGNSSTalkerPayload(%q): %v", payload, err)
	}
	return msg
}

func pu8(v uint8) *uint8      { return &v }
func pf64(v float64) *float64 { return &v }

func oneSelectedGGA(t *testing.T, s *GGASelector) scan.Packet {
	t.Helper()
	select {
	case pkt := <-s.Packets():
		return pkt
	default:
		t.Fatal("no selected GGA")
	}
	return scan.Packet{}
}

func trySelectedGGA(s *GGASelector) (scan.Packet, bool) {
	select {
	case pkt := <-s.Packets():
		return pkt, true
	default:
		return scan.Packet{}, false
	}
}

func hasGGAUTC(data, utc string) bool {
	got, ok := ggaUTC(data)
	return ok && got == utc
}
