package gpscmd

import (
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ascii"
	"github.com/jclark/satpulse/gps/msgfile"
	"github.com/jclark/satpulse/gps/scan"
)

// responseHandler handles displaying responses from the receiver,
// using a Correlator to correlate responses to sent messages.
type responseHandler struct {
	w       io.Writer
	lg      *slog.Logger
	cor     *msgfile.Correlator
	lineBuf []byte
	lineEOL string
}

func newResponseHandler(w io.Writer, lg *slog.Logger) *responseHandler {
	return &responseHandler{
		w:   w,
		lg:  lg,
		cor: msgfile.NewCorrelator(),
	}
}

func (rh *responseHandler) notifySent(rm msgfile.RawMsg) {
	rh.cor.NotifyMsgSent(rm)
}

func (rh *responseHandler) readyToSend(rm msgfile.RawMsg) bool {
	return rh.cor.ReadyToSend(rm)
}

func (rh *responseHandler) canAcceptMore() bool {
	return rh.cor.CanAcceptMore()
}

// handlePacket processes a received packet for display.
// For unrecognized packets, line buffering re-slices raw bytes into
// printable lines which are then fed into CorrelatePacket.
// For recognized packets, CorrelatePacket is called directly.
func (rh *responseHandler) handlePacket(pkt scan.Packet) {
	if pkt.Format == nil {
		rh.bufferLines([]byte(pkt.Data))
		return
	}
	rh.flushLine()
	cor := rh.cor.CorrelatePacket(pkt.Tag(), pkt.Data)
	rh.lg.Debug("correlate packet", "tag", pkt.Tag(), "ack", cor.Ack, "relevance", cor.Relevance)
	if s := rh.formatCorrelation(cor, pkt); s != "" {
		io.WriteString(rh.w, s)
	}
}

func (rh *responseHandler) bufferLines(data []byte) {
	for _, b := range data {
		if b == '\r' || b == '\n' {
			rh.lineEOL += string(b)
			if b == '\n' {
				rh.flushLine()
			}
		} else if isPrintable(b) {
			rh.lineBuf = append(rh.lineBuf, b)
		} else {
			rh.lineBuf = rh.lineBuf[:0]
			rh.lineEOL = ""
		}
	}
}

func (rh *responseHandler) formatCorrelation(cor msgfile.Correlation, pkt scan.Packet) string {
	var b strings.Builder
	if cor.Ack != msgfile.AckNone && cor.InResponseTo != nil {
		b.WriteString(formatAck(cor))
	}
	if cor.Relevance >= msgfile.LevelMaybeResponse {
		b.WriteString(formatPacket(pkt))
	}
	return b.String()
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

func formatAck(cor msgfile.Correlation) string {
	mid := cor.InResponseTo.MsgID()
	prefix := formatMsgID(mid)
	switch cor.Ack {
	case msgfile.AckAck:
		return prefix + ": OK\n"
	case msgfile.AckNak:
		if cor.NakError != "" {
			return fmt.Sprintf("%s: receiver rejected message: %s\n", prefix, cor.NakError)
		}
		return prefix + ": receiver rejected message: NAK\n"
	case msgfile.AckOther:
		return prefix + ": processing...\n"
	}
	return ""
}

func formatMsgID(mid msgfile.MsgID) string {
	if mid.Count <= 1 {
		return mid.Tag
	}
	sep := "/"
	if mid.Tag == "" {
		sep = ""
	}
	return fmt.Sprintf("%s%s%d", mid.Tag, sep, mid.Index+1)
}

func formatText(pkt scan.Packet) string {
	s := strings.TrimRight(pkt.Data, "\r\n")
	if s == "" {
		return ""
	}
	// A multi-line reply packet (e.g. a Septentrio $R reply) is CRLF-separated;
	// pass internal line breaks through as newlines so it displays as its lines
	// rather than falling back to the hex path.
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\r' || c == '\n' {
			if c == '\r' && i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			b.WriteByte('\n')
			continue
		}
		if !isPrintable(c) {
			return ""
		}
		b.WriteByte(c)
	}
	return b.String() + "\n"
}

func (rh *responseHandler) flushLine() {
	if len(rh.lineBuf) == 0 {
		rh.lineEOL = ""
		return
	}
	line := string(rh.lineBuf)
	eol := rh.lineEOL
	rh.lineBuf = rh.lineBuf[:0]
	rh.lineEOL = ""
	cor := rh.cor.CorrelatePacket(gpsprot.EmptyTag, line+eol)
	rh.lg.Debug("correlate line", "ack", cor.Ack, "relevance", cor.Relevance)
	if cor.Relevance >= msgfile.LevelMaybeResponse {
		fmt.Fprintf(rh.w, "%s\n", line)
	}
}

// Flush outputs any buffered data.
func (rh *responseHandler) Flush() {
	rh.flushLine()
}

func (rh *responseHandler) reportMissing() {
	missingAck, missingData := rh.cor.Missing()
	for _, rm := range missingAck {
		fmt.Fprintf(rh.w, "%s: no response received\n", formatMsgID(rm.MsgID()))
	}
	for _, rm := range missingData {
		fmt.Fprintf(rh.w, "%s: no data response received\n", formatMsgID(rm.MsgID()))
	}
}

// isPrintable returns true if b is a printable ASCII char (0x20-0x7E) or tab.
func isPrintable(b byte) bool {
	return ascii.IsPrint(b) || b == '\t'
}
