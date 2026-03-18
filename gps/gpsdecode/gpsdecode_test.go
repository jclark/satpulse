package gpsdecode

import (
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/lib/novmsg"
	"github.com/jclark/satpulse/gps/lib/qtmmsg"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
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
	pf, result, err := Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), packet, false)
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
	_, result, err := Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), packet, false)
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
	_, result, err := Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), packet, true)
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
	_, _, err := Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), packet, false)
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
	pf, result, err := Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), packet, false)
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

func TestDecodeErrInvalidPacket(t *testing.T) {
	// Random bytes that don't match any format
	_, _, err := Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), []byte("random data"), false)
	if err != ErrInvalidPacket {
		t.Errorf("expected ErrInvalidPacket for random data, got %v", err)
	}
	// Starts like UBX but too short / invalid structure
	packet := []byte{0xB5, 0x62, 0x01, 0x20, 0x10, 0x00} // says 16 bytes payload but none present
	_, _, err = Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), packet, false)
	if err != ErrInvalidPacket {
		t.Errorf("expected ErrInvalidPacket for truncated UBX, got %v", err)
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

	pf, _, err := Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), packet, false)
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

func TestDecodeUnicoreAscii(t *testing.T) {
	packet := []byte("#PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;3,2376,540336000,-4,-27676000,0x03E80020,0x00000015,0,0x00666669,0x2B000000,0x0110D2BC,0x00000000,0x2CB0ECAC,0x00000000,0x00000000*0bbaac1a\r\n")
	pf, result, err := Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), packet, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Tag() != gpsreg.TagUnicoreAscii {
		t.Errorf("expected tag %s, got %s", gpsreg.TagUnicoreAscii, pf.Tag())
	}
	if result.Header == nil {
		t.Error("expected header field")
	}
	if result.Payload == nil {
		t.Error("expected payload field")
	}
	if _, ok := result.Payload.(*uncmsg.PPSStatus); !ok {
		t.Errorf("expected *uncmsg.PPSStatus, got %T", result.Payload)
	}
}

func TestDecodeUnicoreAsciiUnknown(t *testing.T) {
	// Build a packet with an unrecognized message name
	packet := []byte("#XYZUNKNOWNA,93,GPS,FINE,2376,540337000,0,0,18,29;foo*")
	// Compute CRC32 checksum
	crc := novmsg.CRC32(packet[1 : len(packet)-1])
	packet = append(packet, []byte(fmt.Sprintf("%08x\r\n", crc))...)
	_, _, err := Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), packet, false)
	if err != ErrUnknownMsg {
		t.Errorf("expected ErrUnknownMsg, got %v", err)
	}
}

func TestDecodeNovAtelAscii(t *testing.T) {
	packet := []byte("#IONUTCA,COM3,0,99.7,FINESTEERING,2382,7850.000,00000000,0000,784;2.514570951461792e-08,2.235174179077148e-08,-1.192092895507813e-07,-1.192092895507813e-07,1.331200000000000e+05,3.276800000000000e+04,-2.621440000000000e+05,4.587520000000000e+05,2382,147456,9.3132257461548000e-10,6.217248938e-15,2441,7,18,18,0*73788199\r\n")
	pf, result, err := Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), packet, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Tag() != gpsreg.TagNovAtelAscii {
		t.Errorf("expected tag %s, got %s", gpsreg.TagNovAtelAscii, pf.Tag())
	}
	if result.Header == nil {
		t.Error("expected header field")
	}
	if result.Payload == nil {
		t.Error("expected payload field")
	}
}

// makeNMEASentence builds a valid NMEA sentence from a payload string.
func makeNMEASentence(payload string) []byte {
	cs := nmeamsg.Checksum([]byte(payload))
	return []byte(fmt.Sprintf("$%s*%02X\r\n", payload, cs))
}

func TestDecodeNMEAPQTM(t *testing.T) {
	payload := "PQTMPVT,1,31075000,20221225,083737.000,,3,09,18,31.12738291,117.26372910,34.212,5.267,3.212,2.928,0.238,4.346,34.12,2.16,4.38"
	packet := makeNMEASentence(payload)
	pf, result, err := Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), packet, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Tag() != gpsreg.TagNMEA {
		t.Errorf("expected tag %s, got %s", gpsreg.TagNMEA, pf.Tag())
	}
	if result.Payload == nil {
		t.Fatal("expected payload field")
	}
	if _, ok := result.Payload.(*qtmmsg.PVT); !ok {
		t.Errorf("expected *qtmmsg.PVT, got %T", result.Payload)
	}
}

func TestDecodeNMEAUnknown(t *testing.T) {
	packet := makeNMEASentence("GPGGA,123456.00,,,,,0,00,99.99,,,,,,")
	_, _, err := Decode(gpsreg.CreatePacketFormats(gpsreg.VendorUnknown), packet, false)
	if err != ErrUnknownMsg {
		t.Errorf("expected ErrUnknownMsg, got %v", err)
	}
}
