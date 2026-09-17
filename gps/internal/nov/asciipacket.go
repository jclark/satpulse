package nov



import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ascii"
	"github.com/jclark/satpulse/gps/lib/novmsg"
)

// TagAscii is the identifier for NovAtel ASCII packets
const TagAscii gpsprot.Tag = "NOVA"

// AsciiPacketFormat is the NovAtel ASCII packet format
// Packets are recognized as valid if they meet all the following criteria:
//
// 1. Starts with '#' character
// 2. Contains at least one ';' character (header/data separator)
// 3. Ends with CR/LF (\r\n)
// 4. Before the CR/LF, the packet ends with '*' followed by exactly 8 lowercase hex digits (32-bit CRC)
// 5. Contains only printable ASCII characters (0x20-0x7E) before the terminating CR/LF
// 6. First field in header after message name starts with alphabetic character (port name)
var AsciiPacketFormat gpsprot.PacketFormat = MakePacketFormat(TagAscii, ascii.IsLetter, false, normalizeAsciiName)

// normalizeAsciiName maps a NovAtel ASCII wire name to its canonical suffix-less
// name (e.g. "BESTPOSA" -> "BESTPOS"), leaving unknown names unchanged.
func normalizeAsciiName(name string) string {
	if id := novmsg.AsciiMsgID(name); id != 0 {
		return id.String()
	}
	return name
}

// asciiPacketFormat implements the gpsprot.PacketFormat interface for ASCII packets
type asciiPacketFormat struct {
	tag                 gpsprot.Tag
	dataFieldValidStart func(byte) bool     // validates first character after comma
	allow2DigitChecksum bool                // whether to allow 2-digit XOR checksums
	nameNormalizer      func(string) string // maps the wire name to its canonical (suffix-less) name; nil is identity
}

// MakePacketFormat creates an ASCII packet format with the given tag and validation function.
// nameNormalizer maps the wire message name to its canonical form for MsgID (nil leaves it as-is).
func MakePacketFormat(tag gpsprot.Tag, dataFieldValidStart func(byte) bool, allow2DigitChecksum bool, nameNormalizer func(string) string) gpsprot.PacketFormat {
	return asciiPacketFormat{
		tag:                 tag,
		dataFieldValidStart: dataFieldValidStart,
		allow2DigitChecksum: allow2DigitChecksum,
		nameNormalizer:      nameNormalizer,
	}
}

func (f asciiPacketFormat) Tag() gpsprot.Tag {
	return f.tag
}

func (f asciiPacketFormat) IsBinary() bool {
	return false
}

const (
	// asciiStateSync is the initial state looking for '#'
	asciiStateSync gpsprot.ScanState = iota + gpsprot.ScanStateSync
	// asciiStateStarted means we have seen '#'
	asciiStateStarted
	// asciiStateHadComma means we found the first comma after message name
	asciiStateHadComma
	// asciiStateBeforeSemi means we validated the port field and are reading rest of header
	asciiStateBeforeSemi
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
		if b == ',' {
			return asciiStateHadComma
		}
		if ascii.IsPrint(b) {
			return asciiStateStarted
		}
	case asciiStateHadComma:
		// Use the validation function to check the first character after comma
		if f.dataFieldValidStart(b) {
			return asciiStateBeforeSemi
		}
	case asciiStateBeforeSemi:
		if b == ';' {
			return asciiStateHadSemi
		}
		if ascii.IsPrint(b) {
			return state
		}
	case asciiStateHadSemi:
		if b == '*' {
			return asciiStateHadStar
		}
		if ascii.IsPrint(b) {
			return state
		}
	case asciiStateHadStar:
		if ascii.IsHexDigit(b) {
			return asciiStateHadChecksum1
		}
	case asciiStateHadChecksum1:
		if ascii.IsHexDigit(b) {
			return asciiStateHadChecksum2
		}
	case asciiStateHadChecksum2:
		if b == '\r' && f.allow2DigitChecksum {
			return asciiStateHadCR
		}
		if ascii.IsHexDigit(b) {
			return asciiStateHadChecksum3
		}
	case asciiStateHadChecksum3, asciiStateHadChecksum4, asciiStateHadChecksum5, asciiStateHadChecksum6, asciiStateHadChecksum7:
		if ascii.IsHexDigit(b) {
			return state + 1
		}
	case asciiStateHadChecksum8:
		if b == '\r' {
			return asciiStateHadCR
		}
	case asciiStateHadCR:
		if b == '\n' {
			return asciiStateComplete
		}
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
			name := string(pkt[1:i])
			if f.nameNormalizer != nil {
				return f.nameNormalizer(name)
			}
			return name
		}
	}
	return ""
}

func (f asciiPacketFormat) ExtractChecksum(pkt []byte) []byte {
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

func (f asciiPacketFormat) ComputeChecksum(pkt []byte) []byte {
	// Check for 8-digit CRC32 checksum: *xxxxxxxx\r\n
	if len(pkt) >= 11 && pkt[len(pkt)-11] == '*' {
		// 32-bit CRC: data from '#' to '*' (exclusive)
		data := pkt[1 : len(pkt)-11]
		crc := novmsg.CRC32(data)
		return []byte{
			byte((crc >> 24) & 0xff),
			byte((crc >> 16) & 0xff),
			byte((crc >> 8) & 0xff),
			byte(crc & 0xff),
		}
	}

	// Check for 2-digit XOR checksum: *xx\r\n
	if len(pkt) >= 5 && pkt[len(pkt)-5] == '*' {
		// 8-bit XOR: data from '#' (inclusive) to '*' (exclusive)
		data := pkt[0 : len(pkt)-5]
		var xor byte
		for _, b := range data {
			xor ^= b
		}
		return []byte{xor}
	}

	return []byte{}
}

func (f asciiPacketFormat) RescanOnBadChecksum(_ bool, _ []byte) bool {
	return false
}

func hexByte(h []byte, i int) byte {
	hi, _ := ascii.HexVal(h[i])
	lo, _ := ascii.HexVal(h[i+1])
	return hi<<4 | lo
}