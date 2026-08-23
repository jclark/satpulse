package scan_test

import (
	"io"
	"math/rand"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/internal/ubx"
	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/lib/novmsg"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
	"github.com/jclark/satpulse/gps/scan"
)

// packetSyncBytes contains the first sync byte of each packet format.
// Used to generate invalid packets that won't be mistaken for valid ones.
var packetSyncBytes = []byte{
	'$',                  // NMEA
	novmsg.AbbrevSync,    // NovAtel abbreviated ASCII
	ubxbin.Sync1,         // UBX
	rtcmbin.PreambleByte, // RTCM
	casbin.Sync1,         // CASIC
	uncmsg.Sync1,         // Unicore
	novmsg.Sync1,         // NovAtel
	asbin.Sync1,          // Allystar
	sdbpbin.Sync1,        // SDBP (Techtotop/Taidou)
}

// randomUBXPacket generates a random UBX packet for testing purposes.
// It returns a string representation of the packet.
// It only needs to be superficially valid:
// start with the right sync bytes, and have the right length.
func randomUBXPacket() string {
	packet := []byte{0xB5, 0x62}

	messageClass := byte(rand.Intn(256))
	messageID := byte(rand.Intn(256))
	packet = append(packet, messageClass, messageID)

	payloadLength := rand.Intn(300)
	packet = append(packet, byte(payloadLength), byte(payloadLength>>8))

	for range payloadLength {
		packet = append(packet, byte(rand.Intn(256)))
	}

	ckA, ckB := ubxbin.Checksum(packet[2:])
	packet = append(packet, ckA, ckB)

	return string(packet)
}

func TestGoodUBX(t *testing.T) {
	const numPackets = 50000
	const bufferSize = 64

	var packets []string

	// Generate random packets and keep track of them
	for range numPackets {
		packet := randomUBXPacket()
		packets = append(packets, packet)
	}

	// Join all packets into a single big string
	data := strings.Join(packets, "")

	// Create a reader and scanner
	r := strings.NewReader(data)
	s := scan.New(r, bufferSize, gpsreg.CreatePacketFormats(nil))

	// Scan the data and verify the packets
	for i, expectedPacket := range packets {
		f, err := s.Scan()
		if err != nil {
			t.Fatalf("Error scanning packet %d: %v", i, err)
		}
		if f.Data != expectedPacket {
			t.Fatalf("Packet %d: expected data %q, got %q", i, expectedPacket, f.Data)
		}
		if !f.ChecksumValid {
			t.Fatalf("Packet %d: checksum not valid", i)
		}
	}

	// Ensure no extra data is left
	pkt, err := s.Scan()
	if err != io.EOF || pkt.Format != nil || pkt.Data != "" {
		t.Fatalf("Expected EOF after all packets, but got: %v", err)
	}
}

func TestUBXRescan(t *testing.T) {
	const bufferSize = 64

	// Generate two random UBX packets
	packet1 := randomUBXPacket()
	packet2 := randomUBXPacket()

	// Insert an invalid byte (0xB5) between the two packets
	data := packet1 + "\xB5" + packet2

	// Create a reader and scanner
	r := strings.NewReader(data)
	s := scan.New(r, bufferSize, gpsreg.CreatePacketFormats(nil))

	// Scan the first packet
	pkt, err := s.Scan()
	if err != nil {
		t.Fatalf("Error scanning first packet: %v", err)
	}
	if !pkt.HasTag(ubx.Tag) {
		t.Fatalf("First packet not recognized as UBX")
	}
	if pkt.Data != packet1 {
		t.Fatalf("First packet data mismatch: expected %q, got %q", packet1, pkt.Data)
	}

	// Scan the invalid packet
	pkt, err = s.Scan()
	if err != nil {
		t.Fatalf("Error scanning invalid packet: %v", err)
	}
	if pkt.Format != nil {
		t.Fatalf("Invalid packet not recognized as invalid")
	}
	if pkt.Data != "\xB5" {
		t.Fatalf("Invalid packet data mismatch: expected %q, got %q", "\xB5", pkt.Data)
	}

	// Scan the second packet
	pkt, err = s.Scan()
	if err != nil {
		t.Fatalf("Error scanning second packet: %v", err)
	}
	if !pkt.HasTag(ubx.Tag) {
		t.Fatalf("Second packet not recognized as UBX")
	}
	if pkt.Data != packet2 {
		t.Fatalf("Second packet data mismatch: expected %q, got %q", packet2, pkt.Data)
	}

	// Ensure no extra data is left
	pkt, err = s.Scan()
	if err != io.EOF || pkt.Format != nil || pkt.Data != "" {
		t.Fatalf("Expected EOF after all packets, but got: %v", err)
	}
}

func TestUBXWithInterspersedInvalidPackets(t *testing.T) {
	const numPackets = 10000
	const bufferSize = 64

	// Generate packets, alternating between UBX and invalid
	var packets []string
	for i := range numPackets {
		if i%2 == 0 {
			// Generate a UBX packet
			packets = append(packets, randomUBXPacket())
		} else {
			// Generate an invalid packet
			packets = append(packets, randomInvalidPacket())
		}
	}

	// Combine all packets into a single data stream
	data := strings.Join(packets, "")

	// Create a reader and scanner
	r := strings.NewReader(data)
	s := scan.New(r, bufferSize, gpsreg.CreatePacketFormats(nil))

	// Top-level loop to scan packets
	var concatenatedInvalid string
	packetIndex := 0
	for {
		pkt, err := s.Scan()
		if err == io.EOF {
			if pkt.Data == "" {
				break
			}
		} else if err != nil {
			t.Fatalf("Error scanning packet: %v", err)
		}

		if packetIndex%2 == 0 {
			// UBX packet
			if !pkt.HasTag(ubx.Tag) {
				t.Fatalf("Packet %d: expected UBX, but got %v", packetIndex, pkt.Format.Tag())
			}
			if pkt.Data != packets[packetIndex] {
				t.Fatalf("Packet %d: UBX data mismatch: expected %q, got %q", packetIndex, packets[packetIndex], pkt.Data)
			}
			if concatenatedInvalid != "" {
				t.Fatalf("Unexpected concatenated invalid data: %q", concatenatedInvalid)
			}
			packetIndex++ // Move to the next packet
		} else {
			// Invalid packet
			if pkt.Format != nil {
				t.Fatalf("Packet %d: expected Invalid, but got %v", packetIndex, pkt.Format.Tag())
			}
			concatenatedInvalid += pkt.Data
			if concatenatedInvalid == packets[packetIndex] {
				// Once the concatenated invalid data matches the expected invalid packet, reset it
				concatenatedInvalid = ""
				packetIndex++ // Move to the next packet
			}
		}
	}

	// Ensure no extra data is left
	if packetIndex != len(packets) {
		t.Fatalf("Expected to process %d packets, but processed %d", len(packets), packetIndex)
	}
}

// randomInvalidPacket generates a random invalid packet that does not start with valid packet start characters.
func randomInvalidPacket() string {
	var packet []byte
	length := rand.Intn(50) + 1 // Random length between 1 and 50

	for len(packet) < length {
		b := byte(rand.Intn(256))
		if !isSyncByte(b) {
			packet = append(packet, b)
		}
	}

	return string(packet)
}

func isSyncByte(b byte) bool {
	for _, sync := range packetSyncBytes {
		if b == sync {
			return true
		}
	}
	return false
}
