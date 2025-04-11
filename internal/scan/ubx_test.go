package scan

import (
	"io"
	"math/rand"
	"strings"
	"testing"

	"github.com/jclark/satpulse/internal/ubx"
)

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

	for i := 0; i < payloadLength; i++ {
		packet = append(packet, byte(rand.Intn(256)))
	}

	ckA, ckB := ubxChecksum(packet[2:])
	packet = append(packet, ckA, ckB)

	return string(packet)
}

func ubxChecksum(data []byte) (ckA, ckB byte) {
	for _, b := range data {
		ckA = (ckA + b) & 0xFF
		ckB = (ckB + ckA) & 0xFF
	}
	return
}

func TestGoodUBX(t *testing.T) {
	const numPackets = 50000
	const bufferSize = 64

	var packets []string

	// Generate random packets and keep track of them
	for i := 0; i < numPackets; i++ {
		packet := randomUBXPacket()
		packets = append(packets, packet)
	}

	// Join all packets into a single big string
	data := strings.Join(packets, "")

	// Create a reader and scanner
	r := strings.NewReader(data)
	s := New(r, bufferSize)

	// Scan the data and verify the packets
	for i, expectedPacket := range packets {
		f, err := s.Scan()
		if err != nil {
			t.Fatalf("Error scanning packet %d: %v", i, err)
		}
		if f.Data != expectedPacket {
			t.Fatalf("Packet %d: expected data %q, got %q", i, expectedPacket, f.Data)
		}
	}

	// Ensure no extra data is left
	f, err := s.Scan()
	if err != io.EOF || f.Kind != Invalid || f.Data != "" {
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
	s := New(r, bufferSize)

	// Scan the first packet
	f, err := s.Scan()
	if err != nil {
		t.Fatalf("Error scanning first packet: %v", err)
	}
	if f.Kind != ubx.PacketKind {
		t.Fatalf("First packet not recognized as UBX")
	}
	if f.Data != packet1 {
		t.Fatalf("First packet data mismatch: expected %q, got %q", packet1, f.Data)
	}

	// Scan the invalid packet
	f, err = s.Scan()
	if err != nil {
		t.Fatalf("Error scanning invalid packet: %v", err)
	}
	if f.Kind != Invalid {
		t.Fatalf("Invalid packet not recognized as invalid")
	}
	if f.Data != "\xB5" {
		t.Fatalf("Invalid packet data mismatch: expected %q, got %q", "\xB5", f.Data)
	}

	// Scan the second packet
	f, err = s.Scan()
	if err != nil {
		t.Fatalf("Error scanning second packet: %v", err)
	}
	if f.Kind != ubx.PacketKind {
		t.Fatalf("Second packet not recognized as UBX")
	}
	if f.Data != packet2 {
		t.Fatalf("Second packet data mismatch: expected %q, got %q", packet2, f.Data)
	}

	// Ensure no extra data is left
	f, err = s.Scan()
	if err != io.EOF || f.Kind != Invalid || f.Data != "" {
		t.Fatalf("Expected EOF after all packets, but got: %v", err)
	}
}

func TestUBXWithInterspersedInvalidPackets(t *testing.T) {
	const numPackets = 10000
	const bufferSize = 64

	// Generate packets, alternating between UBX and invalid
	var packets []string
	for i := 0; i < numPackets; i++ {
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
	s := New(r, bufferSize)

	// Top-level loop to scan packets
	var concatenatedInvalid string
	packetIndex := 0
	for {
		f, err := s.Scan()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Error scanning packet: %v", err)
		}

		if packetIndex%2 == 0 {
			// UBX packet
			if f.Kind != ubx.PacketKind {
				t.Fatalf("Packet %d: expected UBX, but got %v", packetIndex, f.Kind)
			}
			if f.Data != packets[packetIndex] {
				t.Fatalf("Packet %d: UBX data mismatch: expected %q, got %q", packetIndex, packets[packetIndex], f.Data)
			}
			if concatenatedInvalid != "" {
				t.Fatalf("Unexpected concatenated invalid data: %q", concatenatedInvalid)
			}
			packetIndex++ // Move to the next packet
		} else {
			// Invalid packet
			if f.Kind != Invalid {
				t.Fatalf("Packet %d: expected Invalid, but got %v", packetIndex, f.Kind)
			}
			concatenatedInvalid += f.Data
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
		// Exclude valid start bytes for NMEA, UBX, and RTCM packets
		if b != '$' && b != 0xB5 && b != 0xD3 {
			packet = append(packet, b)
		}
	}

	return string(packet)
}
