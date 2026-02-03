package gpscmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/jclark/satpulse/internal/asbin"
	"github.com/jclark/satpulse/internal/casbin"
	"github.com/jclark/satpulse/internal/gpsreg"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/ubxbin"
)

// responsePrinter handles displaying responses from the receiver.
type responsePrinter struct {
	w       io.Writer
	lineBuf []byte
}

func newResponsePrinter(w io.Writer) *responsePrinter {
	return &responsePrinter{w: w}
}

// handlePacket processes a packet for display.
// For unrecognized packets, it accumulates printable chars and prints on EOL.
// For recognized packets, it dispatches to protocol-specific formatters.
func (rp *responsePrinter) handlePacket(pkt scan.Packet) {
	if pkt.Format == nil {
		rp.handleUnrecognized([]byte(pkt.Data))
		return
	}
	rp.flushLine()
	if s := rp.formatPacket(pkt); s != "" {
		io.WriteString(rp.w, s)
	}
}

func (rp *responsePrinter) handleUnrecognized(data []byte) {
	for _, b := range data {
		if b == '\n' {
			rp.flushLine()
		} else if b == '\r' {
			// skip CR, will print on LF
		} else if isPrintable(b) {
			rp.lineBuf = append(rp.lineBuf, b)
		} else {
			// non-printable char, clear buffer
			rp.lineBuf = rp.lineBuf[:0]
		}
	}
}

// formatPacket returns the string to print for a recognized packet.
// Dispatches to protocol-specific formatters.
func (rp *responsePrinter) formatPacket(pkt scan.Packet) string {
	switch pkt.Tag() {
	case gpsreg.TagUBX:
		return rp.formatUBX(pkt)
	case gpsreg.TagCASICBin:
		return rp.formatCASBIN(pkt)
	case gpsreg.TagAllystarBin:
		return rp.formatASBIN(pkt)
	case gpsreg.TagNMEA:
		return rp.formatNMEA(pkt)
	default:
		return rp.formatText(pkt)
	}
}

func (rp *responsePrinter) formatUBX(pkt scan.Packet) string {
	msg, err := ubxbin.ParseMsg(pkt.Data)
	if err != nil {
		return ""
	}
	switch m := msg.(type) {
	case *ubxbin.AckAck:
		return fmt.Sprintf("UBX-ACK-ACK: %s\n", m.MsgID)
	case *ubxbin.AckNak:
		return fmt.Sprintf("UBX-ACK-NAK: %s\n", m.MsgID)
	}
	return ""
}

func (rp *responsePrinter) formatCASBIN(pkt scan.Packet) string {
	msg, err := casbin.ParseMsg(pkt.Data)
	if err != nil {
		return ""
	}
	switch m := msg.(type) {
	case *casbin.AckAck:
		return fmt.Sprintf("CASBIN-ACK-ACK: %s\n", casbin.MakeMsgID(m.ClsID, m.MsgID))
	case *casbin.AckNak:
		return fmt.Sprintf("CASBIN-ACK-NAK: %s\n", casbin.MakeMsgID(m.ClsID, m.MsgID))
	}
	return ""
}

func (rp *responsePrinter) formatASBIN(pkt scan.Packet) string {
	msg, err := asbin.ParseMsg(pkt.Data)
	if err != nil {
		return ""
	}
	switch m := msg.(type) {
	case *asbin.AckAck:
		return fmt.Sprintf("ASBIN-ACK-ACK: %s\n", asbin.MakeMsgID(m.MsgClass, m.MsgID))
	case *asbin.AckNak:
		return fmt.Sprintf("ASBIN-ACK-NAK: %s\n", asbin.MakeMsgID(m.MsgClass, m.MsgID))
	}
	return ""
}

func (rp *responsePrinter) formatNMEA(pkt scan.Packet) string {
	// Skip standard GNSS talker sentences but show TXT and proprietary
	if nmea.CheckSyntax(pkt.Data).IsValidGNSSTalkerNMEA() && pkt.Data[3:6] != "TXT" {
		return ""
	}
	return rp.formatText(pkt)
}

func (rp *responsePrinter) formatText(pkt scan.Packet) string {
	// Strip trailing EOL, check all chars printable
	s := strings.TrimRight(pkt.Data, "\r\n")
	if s == "" {
		return ""
	}
	for i := range len(s) {
		if !isPrintable(s[i]) {
			return ""
		}
	}
	return s + "\n"
}

func (rp *responsePrinter) flushLine() {
	if len(rp.lineBuf) > 0 {
		fmt.Fprintf(rp.w, "%s\n", rp.lineBuf)
		rp.lineBuf = rp.lineBuf[:0]
	}
}

// Flush outputs any buffered data.
func (rp *responsePrinter) Flush() {
	rp.flushLine()
}

// isPrintable returns true if b is a printable ASCII char (0x20-0x7E) or tab.
func isPrintable(b byte) bool {
	return (b >= 0x20 && b <= 0x7E) || b == '\t'
}
