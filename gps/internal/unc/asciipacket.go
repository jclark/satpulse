package unc



import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/internal/nov"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
)

// TagAscii is the identifier for Unicore ASCII packets
const TagAscii gpsprot.Tag = "UNCA"

// AsciiPacketFormat is the Unicore ASCII packet format
// Packets are recognized as valid if they meet all the following criteria:
//
// 1. Starts with '#' character
// 2. Contains at least one ';' character (header/data separator)
// 3. Ends with CR/LF (\r\n)
// 4. Before the CR/LF, the packet ends with one of:
//    a) '*' followed by exactly 2 lowercase hex digits (8-bit XOR checksum) (this handles the output of MODE command)
//    b) '*' followed by exactly 8 lowercase hex digits (32-bit CRC) (this is the normal case)
// 5. Contains only printable ASCII characters (0x20-0x7E) before the terminating CR/LF
// 6. First field in header after message name starts with numeric character (CPU idle %)
var AsciiPacketFormat gpsprot.PacketFormat = nov.MakePacketFormat(TagAscii, isDigit, true, normalizeAsciiName)

// normalizeAsciiName maps a Unicore ASCII wire name to its canonical suffix-less
// name (e.g. "OBSVMA" -> "OBSVM"), leaving unknown names (and MODE) unchanged.
func normalizeAsciiName(name string) string {
	if id := uncmsg.AsciiMsgID(name); id != 0 {
		return id.String()
	}
	return name
}

// isDigit checks if a byte is a numeric character (for Unicore CPU idle %)
func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
