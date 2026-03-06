package gpscmd

import (
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/msgfile"
	"github.com/jclark/satpulse/gps/scan"
)

// responseHandler handles displaying responses from the receiver,
// using a PacketAnalyzer to correlate responses to sent messages.
type responseHandler struct {
	w       io.Writer
	pa      *msgfile.PacketAnalyzer
	lineBuf []byte
}

func newResponseHandler(w io.Writer) *responseHandler {
	return &responseHandler{
		w:  w,
		pa: msgfile.NewPacketAnalyzer(),
	}
}

func (rh *responseHandler) notifySent(rm msgfile.RawMsg) {
	rh.pa.NotifySent(rm)
}

// handlePacket processes a received packet for display.
// For unrecognized packets, line buffering re-slices raw bytes into
// printable lines which are then fed into Analyze.
// For recognized packets, Analyze is called directly.
func (rh *responseHandler) handlePacket(pkt scan.Packet) {
	if pkt.Format == nil {
		rh.bufferLines([]byte(pkt.Data))
		return
	}
	rh.flushLine()
	result := rh.pa.Analyze(pkt.Tag(), pkt.Data)
	if s := rh.formatAnalysis(result, pkt); s != "" {
		io.WriteString(rh.w, s)
	}
}

func (rh *responseHandler) bufferLines(data []byte) {
	for _, b := range data {
		if b == '\n' {
			rh.flushLine()
		} else if b == '\r' {
			// skip CR, will print on LF
		} else if isPrintable(b) {
			rh.lineBuf = append(rh.lineBuf, b)
		} else {
			// non-printable char, clear buffer
			rh.lineBuf = rh.lineBuf[:0]
		}
	}
}

func (rh *responseHandler) formatAnalysis(r msgfile.PacketAnalysis, pkt scan.Packet) string {
	switch r.Kind {
	case msgfile.NotResponse:
		return ""
	case msgfile.AckResponse:
		return formatAck(r)
	default:
		return formatPacket(pkt)
	}
}

func formatPacket(pkt scan.Packet) string {
	if s := formatText(pkt); s != "" {
		return s
	}
	var name string
	if pkt.Format != nil {
		name = string(pkt.Format.Tag()) + "-" + pkt.Format.MsgID([]byte(pkt.Data))
	} else {
		name = "binary"
	}
	return name + " " + hex.EncodeToString([]byte(pkt.Data)) + "\n"
}

func formatAck(r msgfile.PacketAnalysis) string {
	mid := r.RelatedMsg.MsgID()
	prefix := formatMsgID(mid)
	if r.AckError == "" {
		return prefix + ": OK\n"
	}
	return fmt.Sprintf("%s: receiver rejected message: %s\n", prefix, r.AckError)
}

func formatMsgID(mid msgfile.MsgID) string {
	if mid.Count <= 1 {
		return mid.Tag
	}
	return fmt.Sprintf("%s/%d", mid.Tag, mid.Index+1)
}

func formatText(pkt scan.Packet) string {
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

func (rh *responseHandler) flushLine() {
	if len(rh.lineBuf) == 0 {
		return
	}
	line := string(rh.lineBuf)
	rh.lineBuf = rh.lineBuf[:0]
	result := rh.pa.Analyze(gpsprot.EmptyTag, line)
	if result.Kind != msgfile.NotResponse {
		fmt.Fprintf(rh.w, "%s\n", line)
	}
}

// Flush outputs any buffered data.
func (rh *responseHandler) Flush() {
	rh.flushLine()
}

// isPrintable returns true if b is a printable ASCII char (0x20-0x7E) or tab.
func isPrintable(b byte) bool {
	return (b >= 0x20 && b <= 0x7E) || b == '\t'
}
