package gpsdecode

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func mustHexDecode(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func TestDecodeUBX(t *testing.T) {
	// Create a NAV-TIMEGPS message via serialization
	msg := ubxbin.NavTimeGPS{
		NavITOW: ubxbin.NavITOW{ITOW: 123456000},
		FTOW:    -500,
		Week:    2376,
		LeapS:   18,
		Valid:   0x07,
		TAcc:    50,
	}
	packet, err := ubxbin.Serialize(&msg)
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}
	pf, result, err := Decode(gpsreg.PacketFormats, packet, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Tag() != gpsreg.TagUBX {
		t.Errorf("expected tag %s, got %s", gpsreg.TagUBX, pf.Tag())
	}
	if result.Payload == nil {
		t.Fatal("expected payload field")
	}
	decoded, ok := result.Payload.(*ubxbin.NavTimeGPS)
	if !ok {
		t.Fatalf("expected *NavTimeGPS, got %T", result.Payload)
	}
	if decoded.ITOW != msg.ITOW {
		t.Errorf("ITOW mismatch: got %d, want %d", decoded.ITOW, msg.ITOW)
	}
	if decoded.Week != msg.Week {
		t.Errorf("Week mismatch: got %d, want %d", decoded.Week, msg.Week)
	}
}

func TestDecodeUBXCfgValget(t *testing.T) {
	// CFG-VALGET response with config items
	msg := ubxbin.CfgValget{
		CfgValgetFixed: ubxbin.CfgValgetFixed{
			Version:  ubxbin.CfgValgetVersionResponse,
			Layer:    ubxbin.CfgValgetLayerRAM,
			Position: 0,
		},
		// CFG-RATE-MEAS key (0x30210001) with value 100 (1 byte size indicator in key)
		CfgData: []byte{0x01, 0x00, 0x21, 0x30, 0x64, 0x00},
	}
	packet, err := ubxbin.Serialize(&msg)
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}
	// out=false means response, which has items (key-value pairs)
	_, result, err := Decode(gpsreg.PacketFormats, packet, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Payload == nil {
		t.Error("expected payload field")
	}
	if result.CfgData == nil {
		t.Error("expected cfgData field for CFG-VALGET response")
	}
}

func TestDecodeUBXCfgValgetRequest(t *testing.T) {
	// CFG-VALGET request has keys only
	msg := ubxbin.CfgValget{
		CfgValgetFixed: ubxbin.CfgValgetFixed{
			Version:  ubxbin.CfgValgetVersionRequest,
			Layer:    ubxbin.CfgValgetLayerRAM,
			Position: 0,
		},
		// Just a key, no value
		CfgData: []byte{0x01, 0x00, 0x21, 0x30},
	}
	packet, err := ubxbin.Serialize(&msg)
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}
	// out=true means request, which has keys only
	_, result, err := Decode(gpsreg.PacketFormats, packet, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CfgData == nil {
		t.Error("expected cfgData field for CFG-VALGET request")
	}
}

func TestDecodeUBXUnknown(t *testing.T) {
	// Unknown message should return ErrUnknownMsg
	// Construct a minimal valid UBX packet with unknown class/id
	// Sync: B5 62, Class: FF, ID: FF, Length: 0, Checksum: FE F9
	packet := []byte{0xB5, 0x62, 0xFF, 0xFF, 0x00, 0x00, 0xFE, 0xF9}
	_, _, err := Decode(gpsreg.PacketFormats, packet, false)
	if err != ErrUnknownMsg {
		t.Errorf("expected ErrUnknownMsg, got %v", err)
	}
}

func TestDecodeUNCB(t *testing.T) {
	// PPSSTATUSB packet from UM980
	packet := mustHexDecode(
		"aa44b55d28233c0000a0480968e334200000000000121d000300000048090000" +
			"80df3420fcffffffa0b259fe2000e803150000000000000069666600000000" +
			"2bbcd2100100000000acecb02c000000000000000091a800a5",
	)
	pf, result, err := Decode(gpsreg.PacketFormats, packet, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Tag() != gpsreg.TagUnicoreBin {
		t.Errorf("expected tag %s, got %s", gpsreg.TagUnicoreBin, pf.Tag())
	}
	if result.Header == nil {
		t.Error("expected header field for UNCB")
	}
	if result.Payload == nil {
		t.Error("expected payload field for UNCB")
	}
}

func TestDecodeErrUnknownFormat(t *testing.T) {
	// Random bytes that don't match any format
	_, _, err := Decode(gpsreg.PacketFormats, []byte("random data"), false)
	if err != ErrUnknownFormat {
		t.Errorf("expected ErrUnknownFormat, got %v", err)
	}
}

func TestDecodeErrInvalidPacket(t *testing.T) {
	// Starts like UBX but too short / invalid structure
	// This has the sync bytes but length field says more data needed
	packet := []byte{0xB5, 0x62, 0x01, 0x20, 0x10, 0x00} // says 16 bytes payload but none present
	_, _, err := Decode(gpsreg.PacketFormats, packet, false)
	if err != ErrInvalidPacket {
		t.Errorf("expected ErrInvalidPacket, got %v", err)
	}
}

func TestDecodeChecksumError(t *testing.T) {
	msg := ubxbin.NavTimeGPS{
		NavITOW: ubxbin.NavITOW{ITOW: 123456000},
		FTOW:    -500,
		Week:    2376,
		LeapS:   18,
		Valid:   0x07,
		TAcc:    50,
	}
	packet, err := ubxbin.Serialize(&msg)
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}
	// Corrupt last byte (part of checksum)
	packet[len(packet)-1] ^= 0xFF

	pf, _, err := Decode(gpsreg.PacketFormats, packet, false)
	if pf == nil {
		t.Error("expected pf to be returned even on checksum error")
	}
	var csErr *ChecksumError
	if !errors.As(err, &csErr) {
		t.Fatalf("expected ChecksumError, got %v", err)
	}
	if len(csErr.InPacket) == 0 || len(csErr.Computed) == 0 {
		t.Error("expected both checksum fields to be populated")
	}
}
