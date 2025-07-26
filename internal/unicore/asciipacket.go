package unicore

// ASCII Packet Format Specification
//
// This file implements the gpsprot.PacketFormat interface for Unicore ASCII packets.
//
// VALIDITY GUARANTEES:
// The ASCII packet format scanner guarantees that any packet passed to ParseAsciiMessage
// will satisfy ALL of the following criteria:
//
// 1. Starts with '#' character
// 2. Contains at least one ';' character (header/data separator)
// 3. Ends with CR/LF (\r\n)
// 4. Before the CR/LF, the packet ends with one of:
//    a) The first semicolon in the packet (no checksum) (this handles the output of UNILOGLIST command)
//    b) '*' followed by exactly 2 lowercase hex digits (8-bit XOR checksum) (this handles the output of MODE command)
//    c) '*' followed by exactly 8 lowercase hex digits (32-bit CRC) (this is the normal case)
// 5. Contains only printable ASCII characters (0x20-0x7E) before the terminating CR/LF
//
// If ParseAsciiMessage receives a packet that does not satisfy these guarantees,
// it should panic as this indicates a bug in the packet format scanner.
//
// ASCII MESSAGE FORMAT:
// #MessageName,header_fields;data_fields*checksum\r\n
//
// Examples:
// #PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;3,2376,540336000,...*0bbaac1a\r\n
// #MODE,81,GPS,FINE,2230,547967000,0,0,18,518;MODE ROVER SURVEY,*1B\r\n (8-bit checksum)

import (
	"github.com/jclark/satpulse/internal/gpsprot"
)

// AsciiPacketFormat is the Unicore ASCII packet format
var AsciiPacketFormat gpsprot.PacketFormat = asciiPacketFormat{}

// asciiPacketFormat implements the gpsprot.PacketFormat interface for Unicore ASCII packets
type asciiPacketFormat struct{}

func (f asciiPacketFormat) Tag() gpsprot.Tag {
	return "UNCA" // Unicore ASCII
}

const (
	// asciiStateSync is the initial state looking for '#'
	asciiStateSync gpsprot.ScanState = iota + gpsprot.ScanStateSync
	// asciiStateStarted means we have seen '#'
	asciiStateStarted
	// asciiStateHadSemi means we found the first semicolon
	asciiStateHadSemi
	// asciiStateHadStar means we found '*' after semicolon
	asciiStateHadStar
	// asciiStateHadChecksum1 means we have 1 hex digit after '*'
	asciiStateHadChecksum1
	// asciiStateHadChecksum2 means we have 2 hex digits after '*'
	asciiStateHadChecksum2
	// asciiStateHadChecksum3-7 means we have 3-7 hex digits after '*'
	asciiStateHadChecksum3
	asciiStateHadChecksum4
	asciiStateHadChecksum5
	asciiStateHadChecksum6
	asciiStateHadChecksum7
	// asciiStateHadChecksum8 means we have 8 hex digits after '*'
	asciiStateHadChecksum8
	// asciiStateHadCR means we have CR
	asciiStateHadCR
	// asciiStateComplete means we have CR LF
	asciiStateComplete
)

func (f asciiPacketFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
	b := buf[nextScanIndex]

	switch state {
	case asciiStateSync:
		if b == '#' {
			return asciiStateStarted
		}
	case asciiStateStarted:
		if b == ';' {
			return asciiStateHadSemi
		}
		if !isPrintableAscii(b) {
			return asciiStateSync
		}
		return asciiStateStarted
	case asciiStateHadSemi:
		if b == '\r' {
			return asciiStateHadCR
		}
		if b == '*' {
			return asciiStateHadStar
		}
		if !isPrintableAscii(b) {
			return asciiStateSync
		}
		return asciiStateHadSemi
	case asciiStateHadStar:
		if isLowerHexDigit(b) {
			return asciiStateHadChecksum1
		}
		return asciiStateSync
	case asciiStateHadChecksum1:
		if isLowerHexDigit(b) {
			return asciiStateHadChecksum2
		}
		return asciiStateSync
	case asciiStateHadChecksum2:
		if b == '\r' {
			return asciiStateHadCR
		}
		if isLowerHexDigit(b) {
			return asciiStateHadChecksum3
		}
		return asciiStateSync
	case asciiStateHadChecksum3, asciiStateHadChecksum4, asciiStateHadChecksum5, asciiStateHadChecksum6, asciiStateHadChecksum7:
		if isLowerHexDigit(b) {
			return state + 1
		}
		return asciiStateSync
	case asciiStateHadChecksum8:
		if b == '\r' {
			return asciiStateHadCR
		}
		return asciiStateSync
	case asciiStateHadCR:
		if b == '\n' {
			return asciiStateComplete
		}
		return asciiStateSync
	}

	return asciiStateSync
}

func (f asciiPacketFormat) IsFinal(state gpsprot.ScanState) bool {
	return state == asciiStateComplete
}

func (f asciiPacketFormat) MsgID(pkt []byte) string {
	// Find the first comma or semicolon to extract message name
	for i := 1; i < len(pkt); i++ {
		if pkt[i] == ',' || pkt[i] == ';' {
			return string(pkt[1:i])
		}
	}
	return ""
}

func (f asciiPacketFormat) ExtractChecksum(pkt []byte) []byte {
	// Check if packet ends at semicolon (no checksum)
	if pkt[len(pkt)-3] == ';' {
		return []byte{}
	}
	
	// Check for 8-digit CRC32 checksum: *xxxxxxxx\r\n
	if len(pkt) >= 11 && pkt[len(pkt)-11] == '*' {
		h := pkt[len(pkt)-10 : len(pkt)-2]
		return []byte{
			hexByte(h, 0),
			hexByte(h, 2),
			hexByte(h, 4),
			hexByte(h, 6),
		}
	}
	
	// Check for 2-digit XOR checksum: *xx\r\n
	if len(pkt) >= 5 && pkt[len(pkt)-5] == '*' {
		h := pkt[len(pkt)-4 : len(pkt)-2]
		return []byte{hexByte(h, 0)}
	}
	
	return []byte{}
}

const hexDigits = "0123456789abcdef"

func (f asciiPacketFormat) ComputeChecksum(pkt []byte) []byte {
	// Check if packet ends at semicolon (no checksum)
	if pkt[len(pkt)-3] == ';' {
		return []byte{}
	}
	
	// Check for 8-digit CRC32 checksum: *xxxxxxxx\r\n
	if len(pkt) >= 11 && pkt[len(pkt)-11] == '*' {
		// 32-bit CRC: data from '#' to '*' (exclusive)
		data := pkt[1 : len(pkt)-11]
		crc := crc32(data)
		return []byte{
			byte((crc >> 24) & 0xff),
			byte((crc >> 16) & 0xff),
			byte((crc >> 8) & 0xff),
			byte(crc & 0xff),
		}
	}
	
	// Check for 2-digit XOR checksum: *xx\r\n
	if len(pkt) >= 5 && pkt[len(pkt)-5] == '*' {
		// 8-bit XOR: data from '#' to '*' (exclusive)
		data := pkt[1 : len(pkt)-5]
		var xor byte
		for _, b := range data {
			xor ^= b
		}
		return []byte{xor}
	}
	
	return []byte{}
}

func hexByte(h []byte, i int) byte {
	return (hexValue(h[i]) << 4) | hexValue(h[i+1])
}

func hexValue(b byte) byte {
	if b >= '0' && b <= '9' {
		return b - '0'
	}
	if b >= 'a' && b <= 'f' {
		return b - 'a' + 10
	}
	return 0
}

func (f asciiPacketFormat) RescanOnBadChecksum(_ bool, _ []byte) bool {
	return false
}

// Helper functions
func isPrintableAscii(b byte) bool {
	return b >= 0x20 && b <= 0x7E
}

func isLowerHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f')
}